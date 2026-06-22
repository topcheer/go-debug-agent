package debugagent

import (
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// ─── Error tracking registry ────────────────────────────────────────────────

// ErrorEntry represents a captured error or panic.
type ErrorEntry struct {
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
	Stack     string `json:"stack"`
	Type     string `json:"type"`
	Goroutine int   `json:"goroutine"`
}

var (
	errorBuffer   []ErrorEntry
	errorBufferMu sync.RWMutex
	maxErrors     = 50
	errorStart    = time.Now()
)

// CaptureError records an error in the ring buffer for later inspection.
func CaptureError(err error, stack string) {
	if err == nil {
		return
	}
	entry := ErrorEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   err.Error(),
		Stack:     stack,
		Type:      errorType(err),
	}
	captureErrorEntry(entry)
}

// CapturePanic records a recovered panic value in the ring buffer.
func CapturePanic(r any, stack string) {
	if r == nil {
		return
	}
	entry := ErrorEntry{
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Message:   fmtSprintf("%v", r),
		Stack:     stack,
		Type:      "panic",
	}
	captureErrorEntry(entry)
}

func captureErrorEntry(entry ErrorEntry) {
	errorBufferMu.Lock()
	defer errorBufferMu.Unlock()
	errorBuffer = append(errorBuffer, entry)
	// Ring buffer: keep last maxErrors entries
	if len(errorBuffer) > maxErrors {
		errorBuffer = errorBuffer[len(errorBuffer)-maxErrors:]
	}
}

// GetStack returns a formatted stack trace string.
func GetStack() string {
	return string(debug.Stack())
}

func errorType(err error) string {
	msg := err.Error()
	if idx := strings.Index(msg, ":"); idx > 0 {
		return msg[:idx]
	}
	return "error"
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerErrorInspector() {
	RegisterTool("get_recent_errors", "Get recent unhandled errors and panics captured by the agent (timestamp, message, stack trace, goroutine). Ring buffer max 50.", map[string]ToolParam{
		"limit": {Type: "integer", Description: "Maximum number of errors to return (default 20, max 50)", Required: false},
	}, func(args map[string]any) (any, error) {
		errorBufferMu.RLock()
		defer errorBufferMu.RUnlock()

		limit := 20
		if v, ok := args["limit"].(float64); ok && int(v) > 0 && int(v) <= maxErrors {
			limit = int(v)
		}

		total := len(errorBuffer)
		if total == 0 {
			return map[string]any{
				"message": "No errors captured yet. Errors are captured via CaptureError(), CapturePanic(), or the error recovery middleware.",
				"count":   0,
			}, nil
		}

		startIdx := 0
		if total > limit {
			startIdx = total - limit
		}

		result := make([]ErrorEntry, 0, total-startIdx)
		result = append(result, errorBuffer[startIdx:]...)

		return map[string]any{
			"count":         len(result),
			"total_captured": total,
			"errors":        result,
		}, nil
	})

	RegisterTool("get_error_stats", "Get error statistics: total errors, error rate (errors per minute), top error types", nil, func(args map[string]any) (any, error) {
		errorBufferMu.RLock()
		defer errorBufferMu.RUnlock()

		total := len(errorBuffer)
		if total == 0 {
			return map[string]any{
				"total_errors":    0,
				"error_rate":      "0/min",
				"top_error_types": []any{},
			}, nil
		}

		elapsed := time.Since(errorStart)
		if elapsed < time.Second {
			elapsed = time.Second
		}
		ratePerMin := float64(total) / elapsed.Minutes()

		// Count error types
		typeCounts := map[string]int{}
		for _, entry := range errorBuffer {
			typeCounts[entry.Type]++
		}

		topTypes := make([]map[string]any, 0, len(typeCounts))
		for errType, count := range typeCounts {
			topTypes = append(topTypes, map[string]any{
				"type":  errType,
				"count": count,
				"pct":   fmtSprintf("%.1f%%", float64(count)/float64(total)*100),
			})
		}

		return map[string]any{
			"total_errors":    total,
			"error_rate":      fmtSprintf("%.1f/min", ratePerMin),
			"uptime":          elapsed.Round(time.Second).String(),
			"top_error_types": topTypes,
		}, nil
	})

	RegisterTool("get_error_patterns", "Group similar errors by message pattern and show count and first/last occurrence", nil, func(args map[string]any) (any, error) {
		errorBufferMu.RLock()
		defer errorBufferMu.RUnlock()

		if len(errorBuffer) == 0 {
			return map[string]any{
				"message":  "No errors captured yet.",
				"patterns": []any{},
			}, nil
		}

		// Group by error type + simplified message pattern
		type patternInfo struct {
			count    int
			first    string
			last     string
			example  string
			errType  string
		}

		patterns := map[string]*patternInfo{}
		for _, entry := range errorBuffer {
			pattern := errorPattern(entry.Message)
			if _, ok := patterns[pattern]; !ok {
				patterns[pattern] = &patternInfo{
					first:   entry.Timestamp,
					example: entry.Message,
					errType: entry.Type,
			}
			}
			pi := patterns[pattern]
			pi.count++
			pi.last = entry.Timestamp
		}

		result := make([]map[string]any, 0, len(patterns))
		for pattern, info := range patterns {
			result = append(result, map[string]any{
				"pattern":      pattern,
				"type":         info.errType,
				"count":        info.count,
				"first_seen":   info.first,
				"last_seen":    info.last,
				"example":      info.example,
			})
		}

		return map[string]any{
			"total_patterns": len(result),
			"patterns":       result,
		}, nil
	})
}

// errorPattern normalizes an error message into a pattern for grouping.
// Replaces numbers, UUIDs, and hex values with placeholders.
func errorPattern(msg string) string {
	result := make([]byte, 0, len(msg))
	i := 0
	for i < len(msg) {
		c := msg[i]
		// Replace digit sequences with #
		if c >= '0' && c <= '9' {
			result = append(result, '#')
			i++
			for i < len(msg) && msg[i] >= '0' && msg[i] <= '9' {
				i++
			}
			continue
		}
		result = append(result, c)
		i++
	}
	return string(result)
}
