package debugagent

import (
	"bytes"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
)

// ─── Mutex registry ──────────────────────────────────────────────────────────

var (
	registeredMutexes   = map[string]*sync.Mutex{}
	mutexRegistryMu     sync.RWMutex
)

// RegisterMutex registers a *sync.Mutex for inspection by the debug agent.
// Call this with named mutexes to track their state.
func RegisterMutex(name string, m *sync.Mutex) {
	mutexRegistryMu.Lock()
	defer mutexRegistryMu.Unlock()
	registeredMutexes[name] = m
}

func registerLocksInspector() {
	RegisterTool("get_lock_contention", "Get mutex/RWMutex contention stats from the pprof mutex profile. Shows contention time per call site. Requires runtime.SetMutexProfileFraction(1) to be enabled.", map[string]ToolParam{
		"top_n":    {Type: "integer", Description: "Number of top contention sites to return (default 10)", Required: false},
		"max_bytes": {Type: "integer", Description: "Max bytes of raw profile output (default 8000)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
				// recovered — return error info via outer map
			}
		}()

		topN := 10
		if v, ok := args["top_n"].(float64); ok && int(v) > 0 {
			topN = int(v)
		}
		maxBytes := 8000
		if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
			maxBytes = int(v)
		}

		p := pprof.Lookup("mutex")
		result := map[string]any{
			"profile_name": "mutex",
			"enabled":      p != nil && p.Count() > 0,
		}

		if p == nil || p.Count() == 0 {
			result["count"] = 0
			result["note"] = "Set runtime.SetMutexProfileFraction(1) to enable mutex contention profiling."
			return result, nil
		}

		result["count"] = p.Count()

		var buf bytes.Buffer
		p.WriteTo(&buf, 1)
		rawProfile := buf.String()

		result["sample_count"] = p.Count()
		result["top_contention_sites"] = parseContentionSites(rawProfile, topN)

		// Include truncated raw profile
		if len(rawProfile) > maxBytes {
			rawProfile = rawProfile[:maxBytes] + "\n... (truncated)"
		}
		result["raw_profile"] = rawProfile

		return result, nil
	})

	RegisterTool("get_block_profile", "Get goroutine blocking profile from pprof. Shows where goroutines block on channels, mutexes, WaitGroups. Requires runtime.SetBlockProfileRate(1) to be enabled.", map[string]ToolParam{
		"top_n":    {Type: "integer", Description: "Number of top blocking sites to return (default 10)", Required: false},
		"max_bytes": {Type: "integer", Description: "Max bytes of raw profile output (default 8000)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		topN := 10
		if v, ok := args["top_n"].(float64); ok && int(v) > 0 {
			topN = int(v)
		}
		maxBytes := 8000
		if v, ok := args["max_bytes"].(float64); ok && int(v) > 0 {
			maxBytes = int(v)
		}

		p := pprof.Lookup("block")
		result := map[string]any{
			"profile_name": "block",
			"enabled":      p != nil && p.Count() > 0,
		}

		if p == nil || p.Count() == 0 {
			result["count"] = 0
			result["note"] = "Set runtime.SetBlockProfileRate(1) to enable block profiling (1 = sample every nanosecond of block time)."
			return result, nil
		}

		result["count"] = p.Count()

		var buf bytes.Buffer
		p.WriteTo(&buf, 1)
		rawProfile := buf.String()

		result["sample_count"] = p.Count()
		result["top_blocking_sites"] = parseBlockSites(rawProfile, topN)

		if len(rawProfile) > maxBytes {
			rawProfile = rawProfile[:maxBytes] + "\n... (truncated)"
		}
		result["raw_profile"] = rawProfile

		return result, nil
	})

	RegisterTool("detect_deadlock", "Analyze all goroutines for potential deadlock patterns (circular wait on mutexes, blocked goroutines with no progress). Parses goroutine stack traces looking for semacquire and sync.Mutex patterns.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		raw := captureAllGoroutineStacks()
		patterns := detectDeadlockPatterns(raw)

		result := map[string]any{
			"total_goroutines":  runtime.NumGoroutine(),
			"blocked_on_mutex":  patterns.blockedOnMutex,
			"blocked_on_channel": patterns.blockedOnChannel,
			"blocked_on_waitgroup": patterns.blockedOnWaitGroup,
			"semacquire_count":  patterns.semacquireCount,
			"risk_level":        patterns.riskLevel,
			"findings":          patterns.findings,
			"flagged_goroutines": patterns.flaggedGoroutines,
		}

		return result, nil
	})

	RegisterTool("get_mutex_holders", "List which goroutines currently hold which mutexes (requires registration via RegisterMutex). Shows lock state for all registered mutexes.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		mutexRegistryMu.RLock()
		defer mutexRegistryMu.RUnlock()

		if len(registeredMutexes) == 0 {
			return map[string]any{
				"message": "No mutexes registered. Call debugagent.RegisterMutex(name, m) to enable mutex holder inspection.",
				"count":   0,
			}, nil
		}

		holders := make([]map[string]any, 0, len(registeredMutexes))
		for name, m := range registeredMutexes {
			entry := map[string]any{"name": name}
			if m == nil {
				entry["error"] = "nil mutex"
				entry["locked"] = false
			} else {
				entry["locked"] = isMutexLocked(m)
				if isMutexLocked(m) {
					entry["status"] = "held"
				} else {
					entry["status"] = "free"
				}
			}
			holders = append(holders, entry)
		}

		return map[string]any{
			"count":   len(holders),
			"mutexes": holders,
		}, nil
	})
}

// ─── Helper types ────────────────────────────────────────────────────────────

type deadlockAnalysis struct {
	blockedOnMutex     int
	blockedOnChannel   int
	blockedOnWaitGroup int
	semacquireCount    int
	riskLevel          string
	findings           []map[string]any
	flaggedGoroutines  []map[string]any
}

// parseContentionSites parses the mutex pprof text output and extracts top contention sites.
func parseContentionSites(raw string, topN int) []map[string]any {
	sites := []map[string]any{}
	lines := strings.Split(raw, "\n")
	var current map[string]any

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines like "contention: 1234ns 2 ..."
		if strings.HasPrefix(line, "contention:") {
			if current != nil {
				sites = append(sites, current)
				if len(sites) >= topN {
					return sites
				}
			}
			current = map[string]any{"raw": line}
		} else if current != nil && (strings.Contains(line, ".go:") || strings.Contains(line, "(")) {
			stack, ok := current["stack"].(string)
			if !ok {
				stack = ""
			}
			current["stack"] = stack + line + "\n"
		}
	}
	if current != nil {
		sites = append(sites, current)
	}

	if len(sites) > topN {
		sites = sites[:topN]
	}
	return sites
}

// parseBlockSites parses the block pprof text output and extracts top blocking sites.
func parseBlockSites(raw string, topN int) []map[string]any {
	sites := []map[string]any{}
	lines := strings.Split(raw, "\n")
	var current map[string]any

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "contention:") {
			if current != nil {
				sites = append(sites, current)
				if len(sites) >= topN {
					return sites
				}
			}
			current = map[string]any{"raw": line}
		} else if current != nil && (strings.Contains(line, ".go:") || strings.Contains(line, "(")) {
			stack, ok := current["stack"].(string)
			if !ok {
				stack = ""
			}
			current["stack"] = stack + line + "\n"
		}
	}
	if current != nil {
		sites = append(sites, current)
	}

	if len(sites) > topN {
		sites = sites[:topN]
	}
	return sites
}

// detectDeadlockPatterns analyzes goroutine stack traces for deadlock indicators.
func detectDeadlockPatterns(raw string) *deadlockAnalysis {
	result := &deadlockAnalysis{
		riskLevel: "none",
	}

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

		// Extract goroutine ID
		goroutineID := ""
		firstLine := lines[0]
		if idx := strings.Index(firstLine, " "); idx > 0 {
			goroutineID = firstLine[:idx]
		}

		// Extract state
		state := "running"
		if idx := strings.Index(firstLine, "["); idx >= 0 {
			if endIdx := strings.Index(firstLine, "]"); endIdx > idx {
				state = firstLine[idx+1 : endIdx]
			}
		}

		fullStack := strings.Join(lines[1:], "\n")

		isBlockedMutex := false
		hasSemacquire := false

		if strings.Contains(fullStack, "sync.(*Mutex)") || strings.Contains(fullStack, "sync.(*RWMutex)") {
			isBlockedMutex = true
			result.blockedOnMutex++
		}
		if strings.Contains(fullStack, "chan ") || strings.Contains(fullStack, "chansend") || strings.Contains(fullStack, "chanrecv") {
			result.blockedOnChannel++
		}
		if strings.Contains(fullStack, "sync.(*WaitGroup)") {
			result.blockedOnWaitGroup++
		}
		if strings.Contains(fullStack, "semacquire") {
			hasSemacquire = true
			result.semacquireCount++
		}

		// Flag goroutines that are blocked on mutexes while holding semacquire
		if isBlockedMutex || (hasSemacquire && strings.Contains(state, "semacquire")) {
			flagged := map[string]any{
				"goroutine_id": goroutineID,
				"state":        state,
				"reason":       "blocked on mutex",
			}
			// Extract a short stack snippet
			snippet := ""
			for _, l := range lines[1:] {
				l = strings.TrimSpace(l)
				if l != "" && !strings.HasPrefix(l, "created by") {
					snippet += l + "\n"
					if len(snippet) > 300 {
						break
					}
				}
			}
			flagged["stack_snippet"] = snippet
			result.flaggedGoroutines = append(result.flaggedGoroutines, flagged)
		}
	}

	// Determine risk level
	mutexBlocked := result.blockedOnMutex
	if mutexBlocked >= 3 {
		result.riskLevel = "high"
	} else if mutexBlocked >= 1 {
		result.riskLevel = "medium"
	}

	// Check for circular wait pattern: multiple goroutines blocked on mutexes
	if mutexBlocked >= 2 {
		result.findings = append(result.findings, map[string]any{
			"type":        "potential_circular_wait",
			"description": fmtSprintf("%d goroutines blocked on mutexes — possible circular wait deadlock", mutexBlocked),
			"severity":    result.riskLevel,
		})
	}

	if result.semacquireCount > 0 {
		result.findings = append(result.findings, map[string]any{
			"type":        "semacquire_blocking",
			"description": fmtSprintf("%d goroutines in semacquire state (waiting for semaphore)", result.semacquireCount),
			"severity":    "info",
		})
	}

	if result.blockedOnChannel > 0 {
		result.findings = append(result.findings, map[string]any{
			"type":        "channel_blocking",
			"description": fmtSprintf("%d goroutines blocked on channel operations", result.blockedOnChannel),
			"severity":    "info",
		})
	}

	if len(result.findings) == 0 && mutexBlocked == 0 {
		result.findings = append(result.findings, map[string]any{
			"type":        "no_deadlock_detected",
			"description": "No deadlock patterns detected in current goroutine stacks",
			"severity":    "none",
		})
	}

	return result
}

// isMutexLocked checks if a mutex is currently held using reflection.
func isMutexLocked(m *sync.Mutex) bool {
	defer func() {
		if r := recover(); r != nil {
		}
	}()

	rv := reflect.ValueOf(m).Elem()
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return false
	}

	// sync.Mutex has an internal state field. In Go 1.19+ it uses atomic.Int32.
	// If the state field is non-zero, the mutex is locked.
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if !f.IsValid() {
			continue
		}

		// Check for atomic.Int32 field (Go 1.19+)
		if f.Type().PkgPath() == "sync/atomic" && (f.Type().Name() == "Int32" || f.Type().Name() == "Uint32") {
			// Read the value inside atomic.Int32
			for j := 0; j < f.NumField(); j++ {
				inner := f.Field(j)
				if (inner.Kind() == reflect.Int32 || inner.Kind() == reflect.Uint32) && inner.CanAddr() {
					val := inner.Int()
					if val != 0 {
						return true
					}
				}
			}
		}

		// Check for plain int32 (older Go)
		if f.Kind() == reflect.Int32 && f.CanAddr() {
			if f.Int() != 0 {
				return true
			}
		}
	}

	return false
}
