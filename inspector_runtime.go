package debugagent

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

func registerRuntimeInspector() {
	RegisterTool("get_memory_stats", "Get Go runtime memory statistics: heap, stack, GC", nil, func(args map[string]any) (any, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return map[string]any{
			"alloc_mb":         m.Alloc / 1024 / 1024,
			"total_alloc_mb":   m.TotalAlloc / 1024 / 1024,
			"sys_mb":           m.Sys / 1024 / 1024,
			"heap_alloc_mb":    m.HeapAlloc / 1024 / 1024,
			"heap_sys_mb":      m.HeapSys / 1024 / 1024,
			"heap_objects":     m.HeapObjects,
			"stack_inuse_mb":   m.StackInuse / 1024 / 1024,
			"num_gc":           m.NumGC,
			"gc_cpu_fraction":  m.GCCPUFraction,
			"next_gc_mb":       m.NextGC / 1024 / 1024,
		}, nil
	})

	RegisterTool("trigger_gc", "Trigger garbage collection and show before/after comparison", nil, func(args map[string]any) (any, error) {
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		runtime.GC()
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		return map[string]any{
			"alloc_before_mb": before.Alloc / 1024 / 1024,
			"alloc_after_mb":  after.Alloc / 1024 / 1024,
			"freed_mb":        (before.Alloc - after.Alloc) / 1024 / 1024,
			"total_gc_count":  after.NumGC,
		}, nil
	})

	RegisterTool("get_goroutine_dump", "Get goroutine count and stack traces", map[string]ToolParam{
		"detailed": {Type: "boolean", Description: "Include full stack traces", Required: false},
	}, func(args map[string]any) (any, error) {
		count := runtime.NumGoroutine()
		result := map[string]any{
			"goroutine_count": count,
		}
		if detailed, ok := args["detailed"].(bool); ok && detailed {
			buf := make([]byte, 256*1024)
			n := runtime.Stack(buf, true)
			result["stack_traces"] = string(buf[:n])
		}
		return result, nil
	})

	RegisterTool("get_runtime_info", "Get Go runtime info: version, CPU cores, GOMAXPROCS", nil, func(args map[string]any) (any, error) {
		return map[string]any{
			"go_version":   runtime.Version(),
			"gomaxprocs":   runtime.GOMAXPROCS(0),
			"cpu_count":    runtime.NumCPU(),
			"goroutines":   runtime.NumGoroutine(),
			"pid":          os.Getpid(),
			"compiler":     runtime.Compiler,
			"arch":         runtime.GOARCH,
			"os":           runtime.GOOS,
		}, nil
	})

	RegisterTool("get_gc_stats", "Get GC statistics: pause times, collection count", nil, func(args map[string]any) (any, error) {
		var stats debug.GCStats
		debug.ReadGCStats(&stats)
		return map[string]any{
			"num_gc":      stats.NumGC,
			"pause_total": stats.PauseTotal.String(),
			"last_gc":     stats.LastGC.Format(time.RFC3339),
		}, nil
	})

	RegisterTool("get_cpu_profile", "Sample goroutine profile for a short duration", map[string]ToolParam{
		"duration_seconds": {Type: "integer", Description: "Sampling duration in seconds (max 10)", Required: false},
	}, func(args map[string]any) (any, error) {
		secs := 3
		if v, ok := args["duration_seconds"].(float64); ok && int(v) <= 10 {
			secs = int(v)
		}
		time.Sleep(time.Duration(secs) * time.Second)
		return map[string]any{
			"message":         fmt.Sprintf("Profile sampled for %d seconds", secs),
			"goroutine_count": runtime.NumGoroutine(),
			"note":            "Use pprof for detailed CPU profiles. This tool gives a quick snapshot.",
		}, nil
	})
}
