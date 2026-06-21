package debugagent

import (
	"runtime"
	"sort"
	"strings"
)

func registerGoroutineInspector() {
	RegisterTool("get_goroutine_count", "Get the current number of goroutines", nil, func(args map[string]any) (any, error) {
		return map[string]any{
			"goroutine_count": runtime.NumGoroutine(),
		}, nil
	})

	RegisterTool("get_goroutine_stacks", "Get goroutine stack traces grouped by similarity (top N groups)", map[string]ToolParam{
		"top_n": {Type: "integer", Description: "Number of top groups to return (default 10)", Required: false},
	}, func(args map[string]any) (any, error) {
		raw := captureAllGoroutineStacks()
		groups := groupGoroutineStacks(raw)

		topN := 10
		if v, ok := args["top_n"].(float64); ok && int(v) > 0 {
			topN = int(v)
		}
		if topN < len(groups) {
			groups = groups[:topN]
		}

		return map[string]any{
			"total_goroutines": runtime.NumGoroutine(),
			"unique_stacks":    len(groups),
			"top_groups":       groups,
		}, nil
	})

	RegisterTool("get_goroutine_states", "Get goroutine state distribution (running, waiting, sleeping, etc.)", nil, func(args map[string]any) (any, error) {
		raw := captureAllGoroutineStacks()
		states := analyzeGoroutineStates(raw)

		return map[string]any{
			"total_goroutines": runtime.NumGoroutine(),
			"states":           states,
		}, nil
	})
}

// goroutineGroup represents a group of goroutines with similar stack traces.
type goroutineGroup struct {
	Count      int    `json:"count"`
	State      string `json:"state"`
	Sample     string `json:"sample_stack"`
	FirstLines string `json:"first_lines"`
}

// captureAllGoroutineStacks returns the full goroutine stack dump as a string.
func captureAllGoroutineStacks() string {
	bufSize := 1024 * 1024
	if n := runtime.NumGoroutine(); n > 100 {
		bufSize = n * 32 * 1024
	}
	buf := make([]byte, bufSize)
	n := runtime.Stack(buf, true)
	return string(buf[:n])
}

// groupGoroutineStacks groups goroutines by similar stack signatures and returns sorted by count desc.
func groupGoroutineStacks(raw string) []goroutineGroup {
	blocks := strings.Split(raw, "\ngoroutine ")
	groupMap := map[string]*goroutineGroup{}

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 2 {
			continue
		}

		// Extract state from the first line (e.g. "goroutine 1 [running]:")
		state := "running"
		if idx := strings.Index(lines[0], "["); idx >= 0 {
			if endIdx := strings.Index(lines[0], "]"); endIdx > idx {
				state = lines[0][idx+1 : endIdx]
			}
		}

		// Build a signature from the first few non-trivial stack lines
		var sigLines []string
		for i := 1; i < len(lines) && len(sigLines) < 3; i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" || strings.HasPrefix(line, "created by") {
				continue
			}
			sigLines = append(sigLines, line)
		}
		sig := strings.Join(sigLines, "\n")

		if g, ok := groupMap[sig]; ok {
			g.Count++
		} else {
			sample := block
			if len(sample) > 500 {
				sample = sample[:500] + "..."
			}
			groupMap[sig] = &goroutineGroup{
				Count:      1,
				State:      state,
				Sample:     sample,
				FirstLines: strings.Join(sigLines, "\n"),
			}
		}
	}

	groups := make([]goroutineGroup, 0, len(groupMap))
	for _, g := range groupMap {
		groups = append(groups, *g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Count > groups[j].Count
	})
	return groups
}

// analyzeGoroutineStates counts goroutines by their state from the stack dump.
func analyzeGoroutineStates(raw string) map[string]int {
	states := map[string]int{}
	blocks := strings.Split(raw, "\ngoroutine ")

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 1 {
			continue
		}
		state := "running"
		if idx := strings.Index(lines[0], "["); idx >= 0 {
			if endIdx := strings.Index(lines[0], "]"); endIdx > idx {
				state = lines[0][idx+1 : endIdx]
			}
		}
		states[state]++
	}
	return states
}
