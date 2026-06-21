package debugagent

import (
	"runtime"
	"runtime/debug"
)

func registerAllocInspector() {
	RegisterTool("get_alloc_stats", "Get memory allocation statistics: mallocs, frees, total alloc bytes, GC pause total", nil, func(args map[string]any) (any, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		var gcStats debug.GCStats
		debug.ReadGCStats(&gcStats)

		return map[string]any{
			"mallocs":           m.Mallocs,
			"frees":             m.Frees,
			"total_alloc_bytes": m.TotalAlloc,
			"total_alloc_mb":    m.TotalAlloc / 1024 / 1024,
			"alloc_bytes":       m.Alloc,
			"alloc_mb":          m.Alloc / 1024 / 1024,
			"gc_pause_total":    gcStats.PauseTotal.String(),
			"gc_count":          m.NumGC,
			"heap_objects":      m.HeapObjects,
			"lookups":           m.Lookups,
		}, nil
	})

	RegisterTool("get_mem_stats", "Get detailed runtime.MemStats: HeapAlloc, HeapSys, NumGC, PauseNs, StackInuse, etc.", nil, func(args map[string]any) (any, error) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		return map[string]any{
			"alloc":           m.Alloc,
			"total_alloc":     m.TotalAlloc,
			"sys":             m.Sys,
			"lookups":         m.Lookups,
			"mallocs":         m.Mallocs,
			"frees":           m.Frees,
			"heap_alloc":      m.HeapAlloc,
			"heap_sys":        m.HeapSys,
			"heap_idle":       m.HeapIdle,
			"heap_inuse":      m.HeapInuse,
			"heap_released":   m.HeapReleased,
			"heap_objects":    m.HeapObjects,
			"stack_inuse":     m.StackInuse,
			"stack_sys":       m.StackSys,
			"mspan_inuse":     m.MSpanInuse,
			"mspan_sys":       m.MSpanSys,
			"mcache_inuse":    m.MCacheInuse,
			"mcache_sys":      m.MCacheSys,
			"buck_hash_sys":   m.BuckHashSys,
			"gc_sys":          m.GCSys,
			"other_sys":       m.OtherSys,
			"next_gc":         m.NextGC,
			"last_gc":         m.LastGC,
			"pause_total_ns":  m.PauseTotalNs,
			"last_pause_ns":   m.PauseNs[(m.NumGC+255)%256],
			"num_gc":          m.NumGC,
			"num_forced_gc":   m.NumForcedGC,
			"gc_cpu_fraction": m.GCCPUFraction,
			"enable_gc":       m.EnableGC,
			"debug_gc":        m.DebugGC,
		}, nil
	})
}
