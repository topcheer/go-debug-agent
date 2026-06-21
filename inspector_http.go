package debugagent

import (
	"fmt"
	"sync"
	"time"
)

// HTTP request ring buffer
var (
	httpBuffer     []httpRecord
	httpBufferLock sync.Mutex
)

const httpBufferMaxSize = 500

type httpRecord struct {
	Timestamp  time.Time
	Method     string
	Path       string
	Status     int
	DurationMs float64
	Client     string
}

// RecordHTTPRequest records an HTTP request. Call this from your middleware.
func RecordHTTPRequest(method, path string, status int, durationMs float64, client string) {
	httpBufferLock.Lock()
	defer httpBufferLock.Unlock()

	httpBuffer = append(httpBuffer, httpRecord{
		Timestamp:  time.Now(),
		Method:     method,
		Path:       path,
		Status:     status,
		DurationMs: durationMs,
		Client:     client,
	})
	if len(httpBuffer) > httpBufferMaxSize {
		httpBuffer = httpBuffer[len(httpBuffer)-httpBufferMaxSize:]
	}
}

func httpBufferSnapshot() []httpRecord {
	httpBufferLock.Lock()
	defer httpBufferLock.Unlock()
	result := make([]httpRecord, len(httpBuffer))
	copy(result, httpBuffer)
	return result
}

func registerHTTPTrackerInspector() {
	RegisterTool("get_recent_requests", "Get recent HTTP requests from the in-memory ring buffer", map[string]ToolParam{
		"limit": {Type: "integer", Description: "Max results to return", Required: false},
	}, func(args map[string]any) (any, error) {
		reqs := httpBufferSnapshot()
		if limit, ok := args["limit"].(float64); ok && int(limit) < len(reqs) {
			reqs = reqs[len(reqs)-int(limit):]
		}
		// Reverse (most recent first)
		for i, j := 0, len(reqs)-1; i < j; i, j = i+1, j-1 {
			reqs[i], reqs[j] = reqs[j], reqs[i]
		}
		return map[string]any{"total": len(httpBuffer), "requests": reqs}, nil
	})

	RegisterTool("get_slow_requests", "Get slowest HTTP requests sorted by duration", map[string]ToolParam{
		"threshold_ms": {Type: "number", Description: "Minimum duration in ms", Required: false},
	}, func(args map[string]any) (any, error) {
		reqs := httpBufferSnapshot()
		if thr, ok := args["threshold_ms"].(float64); ok {
			var filtered []httpRecord
			for _, r := range reqs {
				if r.DurationMs >= thr {
					filtered = append(filtered, r)
				}
			}
			reqs = filtered
		}
		// Sort descending by duration
		for i := 0; i < len(reqs); i++ {
			for j := i + 1; j < len(reqs); j++ {
				if reqs[j].DurationMs > reqs[i].DurationMs {
					reqs[i], reqs[j] = reqs[j], reqs[i]
				}
			}
		}
		maxReturn := 20
		if len(reqs) > maxReturn {
			reqs = reqs[:maxReturn]
		}
		return map[string]any{"count": len(reqs), "requests": reqs}, nil
	})

	RegisterTool("get_error_requests", "Get all error requests (4xx/5xx status codes)", nil, func(args map[string]any) (any, error) {
		reqs := httpBufferSnapshot()
		var errors []httpRecord
		for _, r := range reqs {
			if r.Status >= 400 {
				errors = append(errors, r)
			}
		}
		return map[string]any{"count": len(errors), "requests": errors}, nil
	})

	RegisterTool("get_request_stats", "Get HTTP request statistics: count, P50/P95/P99 latency, error rate", nil, func(args map[string]any) (any, error) {
		reqs := httpBufferSnapshot()
		if len(reqs) == 0 {
			return map[string]any{"message": "No requests recorded yet"}, nil
		}
		// Sort durations
		durations := make([]float64, len(reqs))
		for i, r := range reqs {
			durations[i] = r.DurationMs
		}
		for i := 0; i < len(durations); i++ {
			for j := i + 1; j < len(durations); j++ {
				if durations[j] < durations[i] {
					durations[i], durations[j] = durations[j], durations[i]
				}
			}
		}
		n := len(durations)
		pct := func(p float64) float64 { return durations[min(int(float64(p*float64(n)))-1, n-1)] }
		errorCount := 0
		for _, r := range reqs {
			if r.Status >= 400 {
				errorCount++
			}
		}
		return map[string]any{
			"total_requests": n,
			"error_count":   errorCount,
			"error_rate":    fmt.Sprintf("%.1f%%", float64(errorCount)/float64(n)*100),
			"latency_ms": map[string]float64{
				"min": durations[0],
				"p50": pct(0.5),
				"p95": pct(0.95),
				"p99": pct(0.99),
				"max": durations[n-1],
			},
		}, nil
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
