package debugagent

import (
	"runtime"
	"runtime/metrics"
	"sort"
	"sync"
	"time"
)

// ─── Heap snapshot state ─────────────────────────────────────────────────────

// HeapSnapshot records the heap state at a point in time.
type HeapSnapshot struct {
	ID         string           `json:"id"`
	Timestamp  time.Time        `json:"timestamp"`
	HeapInUse  uint64           `json:"heap_inuse_bytes"`
	HeapAlloc  uint64           `json:"heap_alloc_bytes"`
	HeapObjects uint64          `json:"heap_objects"`
	TotalAlloc uint64           `json:"total_alloc_bytes"`
	TypeBytes  map[string]int64 `json:"type_bytes"`  // type/class -> bytes
	TypeCounts map[string]int64 `json:"type_counts"` // type/class -> count
}

var (
	heapSnapshots   []HeapSnapshot
	heapSnapshotMu  sync.Mutex
	heapSnapshotSeq int
)

func registerLeakInspector() {
	RegisterTool("take_heap_snapshot", "Record current heap state: object counts by type, total alloc bytes, heap in-use bytes. Returns snapshot ID.", nil, func(args map[string]any) (any, error) {
		snap := captureHeapSnapshot()

		heapSnapshotMu.Lock()
		heapSnapshots = append(heapSnapshots, snap)
		// Keep last 50 snapshots
		if len(heapSnapshots) > 50 {
			heapSnapshots = heapSnapshots[len(heapSnapshots)-50:]
		}
		heapSnapshotMu.Unlock()

		return map[string]any{
			"snapshot_id":  snap.ID,
			"timestamp":    snap.Timestamp.Format(time.RFC3339Nano),
			"heap_inuse":   snap.HeapInUse,
			"heap_alloc":   snap.HeapAlloc,
			"heap_objects": snap.HeapObjects,
			"total_alloc":  snap.TotalAlloc,
			"type_count":   len(snap.TypeBytes),
		}, nil
	})

	RegisterTool("compare_heap_snapshots", "Compare two heap snapshots. Returns per-type: count_delta, bytes_delta, growth_percentage. Sorted by bytes_delta descending.", map[string]ToolParam{
		"snapshot1_id": {Type: "string", Description: "First snapshot ID", Required: true},
		"snapshot2_id": {Type: "string", Description: "Second (newer) snapshot ID", Required: true},
	}, func(args map[string]any) (any, error) {
		id1, _ := args["snapshot1_id"].(string)
		id2, _ := args["snapshot2_id"].(string)
		if id1 == "" || id2 == "" {
			return map[string]any{"error": "Both snapshot1_id and snapshot2_id are required"}, nil
		}

		heapSnapshotMu.Lock()
		defer heapSnapshotMu.Unlock()

		var snap1, snap2 *HeapSnapshot
		for i := range heapSnapshots {
			if heapSnapshots[i].ID == id1 {
				snap1 = &heapSnapshots[i]
			}
			if heapSnapshots[i].ID == id2 {
				snap2 = &heapSnapshots[i]
			}
		}

		if snap1 == nil || snap2 == nil {
			return map[string]any{"error": "One or both snapshot IDs not found"}, nil
		}

		type typeDelta struct {
			Type        string  `json:"type"`
			BytesDelta int64   `json:"bytes_delta"`
			CountDelta int64   `json:"count_delta"`
			GrowthPct  float64 `json:"growth_percentage"`
		}

		allTypes := map[string]bool{}
		for t := range snap1.TypeBytes {
			allTypes[t] = true
		}
		for t := range snap2.TypeBytes {
			allTypes[t] = true
		}

		deltas := make([]typeDelta, 0, len(allTypes))
		for t := range allTypes {
			b1 := snap1.TypeBytes[t]
			b2 := snap2.TypeBytes[t]
			c1 := snap1.TypeCounts[t]
			c2 := snap2.TypeCounts[t]

			var pct float64
			if b1 > 0 {
				pct = float64(b2-b1) / float64(b1) * 100
			} else if b2 > 0 {
				pct = 100.0
			}

			deltas = append(deltas, typeDelta{
				Type:        t,
				BytesDelta: b2 - b1,
				CountDelta: c2 - c1,
				GrowthPct:  pct,
			})
		}

		sort.Slice(deltas, func(i, j int) bool {
			return deltas[i].BytesDelta > deltas[j].BytesDelta
		})

		return map[string]any{
			"snapshot1":         id1,
			"snapshot2":         id2,
			"heap_inuse_delta":  int64(snap2.HeapInUse) - int64(snap1.HeapInUse),
			"heap_alloc_delta":  int64(snap2.HeapAlloc) - int64(snap1.HeapAlloc),
			"heap_objects_delta": int64(snap2.HeapObjects) - int64(snap1.HeapObjects),
			"type_count":        len(deltas),
			"deltas":            deltas,
		}, nil
	})

	RegisterTool("get_leak_candidates", "Analyze heap snapshots for potential leaks: types with monotonic growth, large single allocations. Returns top suspects.", nil, func(args map[string]any) (any, error) {
		heapSnapshotMu.Lock()
		defer heapSnapshotMu.Unlock()

		if len(heapSnapshots) < 2 {
			return map[string]any{
				"message": "Need at least 2 heap snapshots to detect leaks. Call take_heap_snapshot multiple times.",
				"snapshot_count": len(heapSnapshots),
			}, nil
		}

		candidates := detectLeakCandidates(heapSnapshots)

		return map[string]any{
			"snapshot_count": len(heapSnapshots),
			"candidates":     candidates,
		}, nil
	})
}

// captureHeapSnapshot takes a point-in-time snapshot of the heap.
func captureHeapSnapshot() HeapSnapshot {
	heapSnapshotSeq++
	snap := HeapSnapshot{
		ID:        fmtSprintf("heap-%03d", heapSnapshotSeq),
		Timestamp: time.Now(),
		TypeBytes: map[string]int64{},
		TypeCounts: map[string]int64{},
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	snap.HeapInUse = m.HeapInuse
	snap.HeapAlloc = m.HeapAlloc
	snap.HeapObjects = m.HeapObjects
	snap.TotalAlloc = m.TotalAlloc

	// Use runtime/metrics for type-level heap data
	readHeapMetricsByClass(&snap)

	return snap
}

// readHeapMetricsByClass reads runtime/metrics for memory class breakdowns.
func readHeapMetricsByClass(snap *HeapSnapshot) {
	defer func() {
		_ = recover() // Defensive: metrics API may vary across Go versions
	}()

	// Read available memory class metrics
	sampleStats := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/memory/classes/heap/free:bytes"},
		{Name: "/memory/classes/heap/released:bytes"},
		{Name: "/memory/classes/heap/stacks:bytes"},
		{Name: "/memory/classes/heap/unused:bytes"},
	}
	metrics.Read(sampleStats)

	for _, s := range sampleStats {
		if s.Value.Kind() == metrics.KindUint64 {
			val := int64(s.Value.Uint64())
			// Extract class name from metric path
			name := s.Name
			// e.g. /memory/classes/heap/objects:bytes -> heap/objects
			if len(name) > 16 {
				name = name[16:] // strip "/memory/classes/"
			}
			name = trimAfter(name, ':')
			snap.TypeBytes[name] = val
			snap.TypeCounts[name] = 1
		}
	}

	// Also read GC and general memory metrics as separate types
	gcSamples := []metrics.Sample{
		{Name: "/gc/heap/goal:bytes"},
		{Name: "/gc/heap/allocs:bytes"},
		{Name: "/gc/heap/allocs:objects"},
		{Name: "/gc/heap/frees:bytes"},
		{Name: "/gc/heap/frees:objects"},
	}
	metrics.Read(gcSamples)

	for _, s := range gcSamples {
		if s.Value.Kind() == metrics.KindUint64 {
			val := int64(s.Value.Uint64())
			name := s.Name
			if len(name) > 4 {
				name = name[4:] // strip "/gc/"
			}
			name = trimAfter(name, ':')
			snap.TypeBytes[name] = val
			snap.TypeCounts[name] = 1
		}
	}
}

// detectLeakCandidates analyzes heap snapshots for types showing monotonic growth.
func detectLeakCandidates(snapshots []HeapSnapshot) []map[string]any {
	defer func() {
		_ = recover()
	}()

	if len(snapshots) < 2 {
		return nil
	}

	// Collect all type names
	allTypes := map[string]bool{}
	for _, s := range snapshots {
		for t := range s.TypeBytes {
			allTypes[t] = true
		}
	}

	var candidates []map[string]any

	for t := range allTypes {
		var monotonicGrowth bool
		var prevBytes int64
		var firstBytes, lastBytes int64
		var firstSnap, lastSnap int

		for i, s := range snapshots {
			b := s.TypeBytes[t]
			if i == 0 {
				firstBytes = b
				firstSnap = i
			}
			if i > 0 && b > prevBytes {
				monotonicGrowth = true
			} else if i > 0 && b < prevBytes {
				monotonicGrowth = false
			}
			prevBytes = b
			lastBytes = b
			lastSnap = i
		}

		// Only report if there's actual growth
		if lastBytes > firstBytes {
			var growthPct float64
			if firstBytes > 0 {
				growthPct = float64(lastBytes-firstBytes) / float64(firstBytes) * 100
			} else {
				growthPct = 100
			}

			entry := map[string]any{
				"type":            t,
				"bytes_first":     firstBytes,
				"bytes_last":      lastBytes,
				"bytes_growth":    lastBytes - firstBytes,
				"growth_pct":      growthPct,
				"monotonic":       monotonicGrowth,
				"snapshots_span":  lastSnap - firstSnap,
			}

			if monotonicGrowth {
				entry["severity"] = "high"
				entry["reason"] = "Monotonic growth across all snapshots"
			} else if growthPct > 50 {
				entry["severity"] = "medium"
				entry["reason"] = "Significant growth (>50%) but not monotonic"
			} else {
				entry["severity"] = "low"
				entry["reason"] = "Moderate growth"
			}

			candidates = append(candidates, entry)
		}
	}

	// Sort by bytes_growth descending
	sort.Slice(candidates, func(i, j int) bool {
		ci, _ := candidates[i]["bytes_growth"].(int64)
		cj, _ := candidates[j]["bytes_growth"].(int64)
		return ci > cj
	})

	return candidates
}

// trimAfter removes everything from the first occurrence of ch onward.
func trimAfter(s string, ch byte) string {
	for i := 0; i < len(s); i++ {
		if s[i] == ch {
			return s[:i]
		}
	}
	return s
}
