package debugagent

import (
	"bytes"
	"runtime"
	"runtime/pprof"
)

func registerPprofInspector() {
	RegisterTool("get_mutex_profile", "Get mutex contention profile (rate, sample count, top stack frames). Use runtime.SetMutexProfileRate to enable.", map[string]ToolParam{
		"debug":    {Type: "integer", Description: "Debug level for pprof output (0=protobuf, 1=text, 2=full text). Default 1.", Required: false},
		"max_bytes": {Type: "integer", Description: "Max bytes of profile output (default 8000)", Required: false},
	}, func(args map[string]any) (any, error) {
		debugLevel := 1
		if v, ok := args["debug"].(float64); ok {
			debugLevel = int(v)
		}
		maxBytes := 8000
		if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
			maxBytes = int(v)
		}

		p := pprof.Lookup("mutex")
		result := map[string]any{
			"profile_name": "mutex",
			"enabled":      p != nil && p.Count() > 0,
			"note":         "Set runtime.SetMutexProfileRate(1) to enable mutex profiling, or SetMutexProfileRate(rate) where rate > 0.",
		}

		if p == nil {
			result["count"] = 0
			return result, nil
		}

		result["count"] = p.Count()

		var buf bytes.Buffer
		p.WriteTo(&buf, debugLevel)
		output := buf.String()
		if len(output) > maxBytes {
			output = output[:maxBytes] + "\n... (truncated, use higher max_bytes for full output)"
		}
		result["profile"] = output

		return result, nil
	})

	RegisterTool("get_block_profile", "Get goroutine block profile (where goroutines are waiting). Use runtime.SetBlockProfileRate to enable.", map[string]ToolParam{
		"debug":    {Type: "integer", Description: "Debug level for pprof output (0=protobuf, 1=text, 2=full text). Default 1.", Required: false},
		"max_bytes": {Type: "integer", Description: "Max bytes of profile output (default 8000)", Required: false},
	}, func(args map[string]any) (any, error) {
		debugLevel := 1
		if v, ok := args["debug"].(float64); ok {
			debugLevel = int(v)
		}
		maxBytes := 8000
		if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
			maxBytes = int(v)
		}

		p := pprof.Lookup("block")
		result := map[string]any{
			"profile_name": "block",
			"enabled":      p != nil && p.Count() > 0,
			"note":         "Set runtime.SetBlockProfileRate(1) to enable block profiling (1 = sample every nanosecond of block time).",
		}

		if p == nil {
			result["count"] = 0
			return result, nil
		}

		result["count"] = p.Count()

		var buf bytes.Buffer
		p.WriteTo(&buf, debugLevel)
		output := buf.String()
		if len(output) > maxBytes {
			output = output[:maxBytes] + "\n... (truncated, use higher max_bytes for full output)"
		}
		result["profile"] = output

		return result, nil
	})

	RegisterTool("get_threadcreate_profile", "Get OS thread creation profile (shows where new OS threads are being created)", map[string]ToolParam{
		"debug":    {Type: "integer", Description: "Debug level for pprof output (0=protobuf, 1=text, 2=full text). Default 1.", Required: false},
		"max_bytes": {Type: "integer", Description: "Max bytes of profile output (default 8000)", Required: false},
	}, func(args map[string]any) (any, error) {
		debugLevel := 1
		if v, ok := args["debug"].(float64); ok {
			debugLevel = int(v)
		}
		maxBytes := 8000
		if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
			maxBytes = int(v)
		}

		p := pprof.Lookup("threadcreate")
		result := map[string]any{
			"profile_name": "threadcreate",
		}

		if p == nil {
			result["count"] = 0
			return result, nil
		}

		result["count"] = p.Count()
		result["goroutines"] = runtime.NumGoroutine()

		var buf bytes.Buffer
		p.WriteTo(&buf, debugLevel)
		output := buf.String()
		if len(output) > maxBytes {
			output = output[:maxBytes] + "\n... (truncated, use higher max_bytes for full output)"
		}
		result["profile"] = output

		return result, nil
	})
}
