package debugagent

import (
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"sync"
	"time"
	"unsafe"
)

// ─── Outbound HTTP tracking ─────────────────────────────────────────────────────

// OutboundStats holds aggregated outbound HTTP call statistics.
type OutboundStats struct {
	mu           sync.Mutex
	totalCalls   int64
	totalErrors  int64
	totalLatency time.Duration
	perHost      map[string]*hostCallStats
	statusCodes  map[int]int64
}

type hostCallStats struct {
	count        int64
	totalLatency time.Duration
	errorCount   int64
}

var globalOutboundStats = &OutboundStats{
	perHost:     map[string]*hostCallStats{},
	statusCodes: map[int]int64{},
}

// trackingTransport wraps an http.RoundTripper and records call statistics.
type trackingTransport struct {
	inner http.RoundTripper
	stats *OutboundStats
}

func (t *trackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.inner.RoundTrip(req)
	elapsed := time.Since(start)

	host := ""
	if req.URL != nil {
		host = req.URL.Host
	}
	if host == "" {
		host = req.Host
	}

	t.stats.mu.Lock()
	t.stats.totalCalls++
	t.stats.totalLatency += elapsed

	hs := t.stats.perHost[host]
	if hs == nil {
		hs = &hostCallStats{}
		t.stats.perHost[host] = hs
	}
	hs.count++
	hs.totalLatency += elapsed

	if err != nil {
		t.stats.totalErrors++
		hs.errorCount++
	} else if resp != nil {
		t.stats.statusCodes[resp.StatusCode]++
	}
	t.stats.mu.Unlock()

	return resp, err
}

var (
	httpClientRegistry   = map[string]*http.Client{}
	httpClientRegistryMu sync.RWMutex
)

// RegisterHttpClient registers an *http.Client for outbound tracking. The
// client's transport is wrapped so all calls are monitored. If the client has
// no transport set, http.DefaultTransport is used as the inner transport.
func RegisterHttpClient(name string, client *http.Client) {
	if client == nil {
		return
	}

	inner := client.Transport
	if inner == nil {
		inner = http.DefaultTransport
	}

	// Avoid double-wrapping
	if _, ok := inner.(*trackingTransport); ok {
		httpClientRegistryMu.Lock()
		httpClientRegistry[name] = client
		httpClientRegistryMu.Unlock()
		return
	}

	client.Transport = &trackingTransport{
		inner: inner,
		stats: globalOutboundStats,
	}

	httpClientRegistryMu.Lock()
	httpClientRegistry[name] = client
	httpClientRegistryMu.Unlock()
}

// WrapHttpClientTransport wraps a transport with tracking and returns the
// wrapped transport. Useful when you want tracking without registering the
// full client.
func WrapHttpClientTransport(inner http.RoundTripper) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &trackingTransport{inner: inner, stats: globalOutboundStats}
}

func registerHttpClientInspector() {
	RegisterTool("get_http_transport_stats", "Get stats for http.DefaultTransport and registered custom transports (MaxIdleConns, IdleConnsPerHost, MaxConnsPerHost, current idle connections)", map[string]ToolParam{
		"client_name": {Type: "string", Description: "Filter to a specific registered client (optional)", Required: false},
	}, func(args map[string]any) (any, error) {
		transports := make([]map[string]any, 0)

		// Always include DefaultTransport
		transports = append(transports, inspectTransport("default", http.DefaultTransport))

		// Registered clients
		httpClientRegistryMu.RLock()
		filterName, _ := args["client_name"].(string)
		for name, client := range httpClientRegistry {
			if filterName != "" && name != filterName {
				continue
			}
			t := client.Transport
			if t == nil {
				t = http.DefaultTransport
			}
			transports = append(transports, inspectTransport(name, t))
		}
		httpClientRegistryMu.RUnlock()

		return map[string]any{
			"count":      len(transports),
			"transports": transports,
		}, nil
	})

	RegisterTool("get_outbound_summary", "Summary of outbound HTTP calls tracked by the agent (total calls, avg latency, error rate, top destinations)", nil, func(args map[string]any) (any, error) {
		stats := globalOutboundStats
		stats.mu.Lock()
		defer stats.mu.Unlock()

		if stats.totalCalls == 0 {
			return map[string]any{
				"message":     "No outbound HTTP calls tracked yet. Call RegisterHttpClient(name, client) to enable tracking.",
				"total_calls": 0,
			}, nil
		}

		avgLatency := time.Duration(0)
		if stats.totalCalls > 0 {
			avgLatency = stats.totalLatency / time.Duration(stats.totalCalls)
		}

		type destEntry struct {
			Host       string `json:"host"`
			Calls      int64  `json:"calls"`
			AvgLatency string `json:"avg_latency"`
			Errors     int64  `json:"errors"`
		}

		dests := make([]destEntry, 0, len(stats.perHost))
		for host, hs := range stats.perHost {
			hAvg := time.Duration(0)
			if hs.count > 0 {
				hAvg = hs.totalLatency / time.Duration(hs.count)
			}
			dests = append(dests, destEntry{
				Host:       host,
				Calls:      hs.count,
				AvgLatency: hAvg.String(),
				Errors:     hs.errorCount,
			})
		}
		sort.Slice(dests, func(i, j int) bool { return dests[i].Calls > dests[j].Calls })
		if len(dests) > 10 {
			dests = dests[:10]
		}

		errorRate := "0%"
		if stats.totalCalls > 0 {
			errorRate = fmt.Sprintf("%.2f%%", float64(stats.totalErrors)/float64(stats.totalCalls)*100)
		}

		return map[string]any{
			"total_calls":      stats.totalCalls,
			"total_errors":     stats.totalErrors,
			"avg_latency":      avgLatency.String(),
			"error_rate":       errorRate,
			"status_codes":     stats.statusCodes,
			"top_destinations": dests,
		}, nil
	})
}

// inspectTransport extracts configuration from an http.RoundTripper.
func inspectTransport(name string, rt http.RoundTripper) map[string]any {
	entry := map[string]any{"name": name}
	if rt == nil {
		entry["error"] = "nil transport"
		return entry
	}

	// Unwrap tracking transport
	if tt, ok := rt.(*trackingTransport); ok {
		entry["tracked"] = true
		if tt.inner != nil {
			rt = tt.inner
		}
	}

	// Read config from *http.Transport
	if t, ok := rt.(*http.Transport); ok {
		entry["max_idle_conns"] = t.MaxIdleConns
		entry["idle_conns_per_host"] = t.MaxIdleConnsPerHost
		entry["max_conns_per_host"] = t.MaxConnsPerHost
		entry["disable_keepalives"] = t.DisableKeepAlives
		entry["force_attempt_http2"] = t.ForceAttemptHTTP2
		if t.TLSHandshakeTimeout > 0 {
			entry["tls_handshake_timeout"] = t.TLSHandshakeTimeout.String()
		}
		if t.IdleConnTimeout > 0 {
			entry["idle_conn_timeout"] = t.IdleConnTimeout.String()
		}

		// Try to read idle connection count via reflection
		idleHosts, idleTotal := readIdleConnStats(t)
		if idleHosts >= 0 {
			entry["idle_conn_hosts"] = idleHosts
			entry["total_idle_conns"] = idleTotal
		}
	} else {
		entry["type"] = fmt.Sprintf("%T", rt)
	}

	return entry
}

// readIdleConnStats reads the internal idleConn map from *http.Transport using
// reflection. Returns (-1, -1) if the field cannot be accessed.
func readIdleConnStats(t *http.Transport) (hosts int, total int) {
	defer func() {
		if r := recover(); r != nil {
			hosts = -1
			total = -1
		}
	}()

	rv := reflect.ValueOf(t).Elem()
	idleField := rv.FieldByName("idleConn")
	if !idleField.IsValid() || idleField.Kind() != reflect.Map {
		return -1, -1
	}

	keys := idleField.MapKeys()
	hosts = len(keys)
	total = 0
	for _, key := range keys {
		val := idleField.MapIndex(key)
		if val.IsValid() && val.Kind() == reflect.Slice {
			total += val.Len()
		}
	}
	return hosts, total
}

// readUnexportedField reads an unexported struct field value using unsafe.
// This is used by the context and sync inspectors to access internal state.
func readUnexportedField(rv reflect.Value, index int) any {
	defer func() {
		recover()
	}()
	field := rv.Field(index)
	if !field.IsValid() || !field.CanAddr() {
		return nil
	}
	ptr := unsafe.Pointer(field.UnsafeAddr())
	return reflect.NewAt(field.Type(), ptr).Elem().Interface()
}
