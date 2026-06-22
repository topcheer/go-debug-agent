package debugagent

import (
	"bytes"
	"sort"
	"sync"
	"time"

	"github.com/google/pprof/profile"
	"runtime/pprof"
)

// ─── CPU profiling state ─────────────────────────────────────────────────────

var (
	cpuProfileBuf      bytes.Buffer
	cpuProfileMu       sync.Mutex
	cpuProfilingActive bool
	cpuProfileDuration int
	cpuProfileStart    time.Time
	lastTopFunctions   []cpuFuncEntry
)

type cpuFuncEntry struct {
	Name        string `json:"name"`
	File        string `json:"file"`
	Line        int64  `json:"line"`
	CumulativeNs int64  `json:"cumulative_ns"`
	SelfNs      int64  `json:"self_ns"`
	Count       int64  `json:"count"`
}

func registerCPUProfileInspector() {
	RegisterTool("start_cpu_profile", "Start CPU profiling with runtime/pprof. Writes to an internal buffer. Returns a confirmation message.", map[string]ToolParam{
		"duration_seconds": {Type: "integer", Description: "Duration in seconds before auto-stopping (default 30, max 120)", Required: false},
	}, func(args map[string]any) (any, error) {
		cpuProfileMu.Lock()
		defer cpuProfileMu.Unlock()

		if cpuProfilingActive {
			return map[string]any{
				"error":   "CPU profiling is already active. Call stop_cpu_profile first.",
				"active":  true,
				"elapsed": time.Since(cpuProfileStart).Round(time.Second).String(),
			}, nil
		}

		duration := 30
		if v, ok := args["duration_seconds"].(float64); ok && int(v) > 0 {
			duration = int(v)
			if duration > 120 {
				duration = 120
			}
		}

		cpuProfileBuf.Reset()
		if err := pprof.StartCPUProfile(&cpuProfileBuf); err != nil {
			return map[string]any{"error": "Failed to start CPU profiling: " + err.Error()}, nil
		}

		cpuProfilingActive = true
		cpuProfileDuration = duration
		cpuProfileStart = time.Now()

		time.AfterFunc(time.Duration(duration)*time.Second, func() {
			cpuProfileMu.Lock()
			defer cpuProfileMu.Unlock()
			if cpuProfilingActive {
				pprof.StopCPUProfile()
				cpuProfilingActive = false
				lastTopFunctions = parseCPUProfile(&cpuProfileBuf)
			}
		})

		return map[string]any{
			"message": fmtSprintf("profiling started, will run for %d seconds", duration),
			"active":  true,
			"started_at": time.Now().Format(time.RFC3339),
		}, nil
	})

	RegisterTool("stop_cpu_profile", "Stop active CPU profiling and return top 20 functions by cumulative time.", nil, func(args map[string]any) (any, error) {
		cpuProfileMu.Lock()
		defer cpuProfileMu.Unlock()

		if !cpuProfilingActive {
			return map[string]any{
				"error":         "No CPU profiling is active.",
				"last_available": len(lastTopFunctions) > 0,
			}, nil
		}

		elapsed := time.Since(cpuProfileStart)
		pprof.StopCPUProfile()
		cpuProfilingActive = false

		lastTopFunctions = parseCPUProfile(&cpuProfileBuf)

		limit := 20
		top := lastTopFunctions
		if limit < len(top) {
			top = top[:limit]
		}

		return map[string]any{
			"message":       fmtSprintf("CPU profiling stopped after %s", elapsed.Round(time.Millisecond)),
			"elapsed":       elapsed.Round(time.Millisecond).String(),
			"total_samples": len(lastTopFunctions),
			"top_functions": top,
		}, nil
	})

	RegisterTool("get_top_functions", "Get top N functions from the last CPU profile by cumulative, self, or count.", map[string]ToolParam{
		"limit":   {Type: "integer", Description: "Number of top functions to return (default 20)", Required: false},
		"sort_by": {Type: "string", Description: "Sort order: cumulative (default), self, count", Required: false},
	}, func(args map[string]any) (any, error) {
		cpuProfileMu.Lock()
		defer cpuProfileMu.Unlock()

		if len(lastTopFunctions) == 0 {
			return map[string]any{
				"message": "No CPU profile data available. Call start_cpu_profile then stop_cpu_profile first.",
				"count":   0,
			}, nil
		}

		limit := 20
		if v, ok := args["limit"].(float64); ok && int(v) > 0 {
			limit = int(v)
		}

		sortBy := "cumulative"
		if v, ok := args["sort_by"].(string); ok && v != "" {
			sortBy = v
		}

		funcs := make([]cpuFuncEntry, len(lastTopFunctions))
		copy(funcs, lastTopFunctions)

		switch sortBy {
		case "self":
			sort.Slice(funcs, func(i, j int) bool { return funcs[i].SelfNs > funcs[j].SelfNs })
		case "count":
			sort.Slice(funcs, func(i, j int) bool { return funcs[i].Count > funcs[j].Count })
		default:
			sort.Slice(funcs, func(i, j int) bool { return funcs[i].CumulativeNs > funcs[j].CumulativeNs })
		}

		if limit < len(funcs) {
			funcs = funcs[:limit]
		}

		return map[string]any{
			"count":    len(funcs),
			"sort_by":  sortBy,
			"functions": funcs,
		}, nil
	})
}

// parseCPUProfile parses a protobuf CPU profile buffer and returns function
// entries sorted by cumulative nanoseconds descending.
func parseCPUProfile(buf *bytes.Buffer) []cpuFuncEntry {
	defer func() {
		// Defensive: recover from any parsing panic
		_ = recover()
	}()

	if buf.Len() == 0 {
		return nil
	}

	data := buf.Bytes()
	p, err := profile.ParseData(data)
	if err != nil || p == nil {
		return nil
	}

	type funcKey struct {
		name string
		file string
		line int64
	}

	cumulative := map[funcKey]int64{}
	self := map[funcKey]int64{}
	counts := map[funcKey]int64{}

	for _, s := range p.Sample {
		var cumValue int64
		for _, v := range s.Value {
			cumValue += v
		}

		// Self time = last location in the stack
		if len(s.Location) > 0 {
			loc := s.Location[0]
			if len(loc.Line) > 0 {
				line := loc.Line[0]
				var fnName, fileName string
				if line.Function != nil {
					fnName = line.Function.Name
					if line.Function.Filename != "" {
						fileName = line.Function.Filename
					}
				}
				k := funcKey{name: fnName, file: fileName, line: line.Line}
				self[k] += cumValue
				counts[k]++
			}
		}

		// Cumulative time = every location in the stack
		for _, loc := range s.Location {
			for _, line := range loc.Line {
				var fnName, fileName string
				if line.Function != nil {
					fnName = line.Function.Name
					if line.Function.Filename != "" {
						fileName = line.Function.Filename
					}
				}
				k := funcKey{name: fnName, file: fileName, line: line.Line}
				cumulative[k] += cumValue
			}
		}
	}

	allKeys := make(map[funcKey]bool)
	for k := range cumulative {
		allKeys[k] = true
	}

	entries := make([]cpuFuncEntry, 0, len(allKeys))
	for k := range allKeys {
		entries = append(entries, cpuFuncEntry{
			Name:         k.name,
			File:         k.file,
			Line:         k.line,
			CumulativeNs: cumulative[k],
			SelfNs:       self[k],
			Count:        counts[k],
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CumulativeNs > entries[j].CumulativeNs
	})

	return entries
}
