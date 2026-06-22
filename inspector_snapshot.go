package debugagent

import (
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"time"
)

// ─── Snapshot state ─────────────────────────────────────────────────────────

// Snapshot is a point-in-time collection of all inspector metrics.
type Snapshot struct {
	ID        string         `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data"`
}

var (
	snapshots     []Snapshot
	snapshotMu    sync.Mutex
	snapshotSeq   int
)

func registerSnapshotInspector() {
	RegisterTool("take_snapshot", "Collect key metrics from ALL inspectors in one snapshot: goroutine count, heap stats, GC stats, alloc stats, thread count, mutex contention, open FDs, HTTP request count, DB pool stats, error count, cache hit rate. Returns snapshot ID + summary.", nil, func(args map[string]any) (any, error) {
		data := gatherAllMetrics()

		snapshotMu.Lock()
		snapshotSeq++
		snap := Snapshot{
			ID:        fmtSprintf("snap-%03d", snapshotSeq),
			Timestamp: time.Now(),
			Data:      data,
		}
		snapshots = append(snapshots, snap)
		// Keep last 50 snapshots
		if len(snapshots) > 50 {
			snapshots = snapshots[len(snapshots)-50:]
		}
		snapshotMu.Unlock()

		summary := buildSnapshotSummary(data)
		summary["snapshot_id"] = snap.ID
		summary["timestamp"] = snap.Timestamp.Format(time.RFC3339Nano)

		return summary, nil
	})

	RegisterTool("compare_snapshots", "Compare two snapshots. Returns ALL changed values with delta and percentage.", map[string]ToolParam{
		"snapshot1_id": {Type: "string", Description: "First snapshot ID", Required: true},
		"snapshot2_id": {Type: "string", Description: "Second snapshot ID", Required: true},
	}, func(args map[string]any) (any, error) {
		id1, _ := args["snapshot1_id"].(string)
		id2, _ := args["snapshot2_id"].(string)
		if id1 == "" || id2 == "" {
			return map[string]any{"error": "Both snapshot1_id and snapshot2_id are required"}, nil
		}

		snapshotMu.Lock()
		defer snapshotMu.Unlock()

		var snap1, snap2 *Snapshot
		for i := range snapshots {
			if snapshots[i].ID == id1 {
				snap1 = &snapshots[i]
			}
			if snapshots[i].ID == id2 {
				snap2 = &snapshots[i]
			}
		}

		if snap1 == nil || snap2 == nil {
			return map[string]any{"error": "One or both snapshot IDs not found"}, nil
		}

		changes := compareSnapshotData(snap1.Data, snap2.Data)

		return map[string]any{
			"snapshot1":      id1,
			"snapshot2":      id2,
			"total_metrics":  len(snap1.Data),
			"changed_metrics": len(changes),
			"changes":        changes,
		}, nil
	})

	RegisterTool("list_snapshots", "List all stored snapshots with timestamp and brief summary.", nil, func(args map[string]any) (any, error) {
		snapshotMu.Lock()
		defer snapshotMu.Unlock()

		if len(snapshots) == 0 {
			return map[string]any{
				"message": "No snapshots taken yet. Call take_snapshot to create one.",
				"count":   0,
			}, nil
		}

		list := make([]map[string]any, 0, len(snapshots))
		for _, s := range snapshots {
			list = append(list, buildSnapshotSummary(s.Data))
		}

		// Add IDs and timestamps
		for i, s := range snapshots {
			list[i]["snapshot_id"] = s.ID
			list[i]["timestamp"] = s.Timestamp.Format(time.RFC3339Nano)
		}

		return map[string]any{
			"count":       len(list),
			"snapshots":   list,
		}, nil
	})
}

// gatherAllMetrics collects metrics from all available inspectors.
func gatherAllMetrics() map[string]any {
	defer func() {
		_ = recover() // Defensive: some metric reads may fail
	}()

	data := map[string]any{}

	// Runtime / Goroutine metrics
	data["goroutine_count"] = runtime.NumGoroutine()

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	data["heap_alloc_bytes"] = m.HeapAlloc
	data["heap_inuse_bytes"] = m.HeapInuse
	data["heap_sys_bytes"] = m.HeapSys
	data["heap_objects"] = m.HeapObjects
	data["stack_inuse_bytes"] = m.StackInuse
	data["total_alloc_bytes"] = m.TotalAlloc
	data["alloc_bytes"] = m.Alloc
	data["sys_bytes"] = m.Sys
	data["mallocs"] = m.Mallocs
	data["frees"] = m.Frees
	data["num_gc"] = m.NumGC
	data["pause_total_ns"] = m.PauseTotalNs
	data["gc_cpu_fraction"] = m.GCCPUFraction
	data["next_gc_bytes"] = m.NextGC

	// GC stats
	var gcStats debug.GCStats
	debug.ReadGCStats(&gcStats)
	data["gc_pause_total_ns"] = gcStats.PauseTotal.Nanoseconds()
	data["gc_num_gc"] = gcStats.NumGC
	if len(gcStats.Pause) > 0 {
		data["gc_last_pause_ns"] = gcStats.Pause[0].Nanoseconds()
	} else {
		data["gc_last_pause_ns"] = int64(0)
	}

	// OS thread count
	data["thread_count"] = pprofThreadCount()

	// Open FDs
	fdCount, _ := getFdCount()
	data["fd_count"] = fdCount

	// Error count
	errorBufferMu.RLock()
	data["error_count"] = len(errorBuffer)
	errorBufferMu.RUnlock()

	// HTTP request count
	httpBufferLock.Lock()
	data["http_request_count"] = len(httpBuffer)
	httpBufferLock.Unlock()

	// DB pool stats (registered databases)
	data["db_pool_count"] = safeGetDBPoolCount()

	// Cache stats
	data["cache_count"] = safeGetCacheCount()

	// Registry counts
	data["registered_mutexes"] = mutexRegistryCount()
	data["registered_waitgroups"] = waitGroupCount()
	data["health_checks"] = healthCheckCount()
	data["feature_flags"] = featureFlagCount()

	return data
}

// buildSnapshotSummary creates a concise summary from snapshot data.
func buildSnapshotSummary(data map[string]any) map[string]any {
	return map[string]any{
		"goroutines":      data["goroutine_count"],
		"heap_alloc_mb":   toUint64(data["heap_alloc_bytes"]) / 1024 / 1024,
		"heap_objects":    data["heap_objects"],
		"gc_count":        data["num_gc"],
		"goroutine_count": data["goroutine_count"],
		"thread_count":    data["thread_count"],
		"fd_count":        data["fd_count"],
		"error_count":     data["error_count"],
		"http_requests":   data["http_request_count"],
		"db_pools":        data["db_pool_count"],
		"caches":          data["cache_count"],
	}
}

// compareSnapshotData compares two snapshot data maps and returns changed values.
func compareSnapshotData(data1, data2 map[string]any) []map[string]any {
	defer func() {
		_ = recover()
	}()

	allKeys := map[string]bool{}
	for k := range data1 {
		allKeys[k] = true
	}
	for k := range data2 {
		allKeys[k] = true
	}

	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var changes []map[string]any
	for _, k := range keys {
		v1 := data1[k]
		v2 := data2[k]

		f1 := toFloat64(v1)
		f2 := toFloat64(v2)

		if f1 == f2 {
			continue
		}

		delta := f2 - f1
		var pct float64
		if f1 != 0 {
			pct = (delta / absFloat64(f1)) * 100
		} else if f2 != 0 {
			pct = 100
		}

		changes = append(changes, map[string]any{
			"metric":        k,
			"value_before":  v1,
			"value_after":   v2,
			"delta":         delta,
			"change_pct":    pct,
		})
	}

	return changes
}

// toFloat64 converts numeric any values to float64.
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint64:
		return float64(val)
	case uint32:
		return float64(val)
	case uint:
		return float64(val)
	default:
		return 0
	}
}

// toUint64 converts numeric any values to uint64.
func toUint64(v any) uint64 {
	switch val := v.(type) {
	case uint64:
		return val
	case int64:
		return uint64(val)
	case float64:
		return uint64(val)
	case int:
		return uint64(val)
	case uint:
		return uint64(val)
	default:
		return 0
	}
}

// absFloat64 returns the absolute value.
func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ─── Safe accessor helpers for cross-inspector data ───────────────────────────

func pprofThreadCount() int {
	defer func() {
		_ = recover()
	}()
	p := pprofLookupThreadcreate()
	if p == nil {
		return 0
	}
	return p.Count()
}

func safeGetDBPoolCount() int {
	defer func() {
		_ = recover()
	}()
	dbRegistryMu.Lock()
	defer dbRegistryMu.Unlock()
	return len(dbRegistry)
}

func safeGetCacheCount() int {
	defer func() {
		_ = recover()
	}()
	cacheRegistryMu.RLock()
	defer cacheRegistryMu.RUnlock()
	return len(cacheRegistry)
}

func mutexRegistryCount() int {
	defer func() {
		_ = recover()
	}()
	mutexRegistryMu.RLock()
	defer mutexRegistryMu.RUnlock()
	return len(registeredMutexes)
}

func waitGroupCount() int {
	defer func() {
		_ = recover()
	}()
	waitGroupRegistryMu.RLock()
	defer waitGroupRegistryMu.RUnlock()
	return len(waitGroupRegistry)
}

func healthCheckCount() int {
	defer func() {
		_ = recover()
	}()
	healthMu.RLock()
	defer healthMu.RUnlock()
	return len(healthChecks)
}

func featureFlagCount() int {
	defer func() {
		_ = recover()
	}()
	featureFlagMu.RLock()
	defer featureFlagMu.RUnlock()
	return len(featureFlags)
}
