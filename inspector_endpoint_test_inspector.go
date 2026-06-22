package debugagent

import (
	"bytes"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ─── Endpoint testing state ──────────────────────────────────────────────────

var (
	testedEndpoints   = map[string]bool{} // "METHOD:path" → true
	endpointTestMu   sync.Mutex
)

// markEndpointTested records that an endpoint was tested.
func markEndpointTested(method, path string) {
	endpointTestMu.Lock()
	defer endpointTestMu.Unlock()
	key := strings.ToUpper(method) + ":" + path
	testedEndpoints[key] = true
}

// getTestedEndpoints returns a sorted copy of all tested endpoint keys.
func getTestedEndpoints() []string {
	endpointTestMu.Lock()
	defer endpointTestMu.Unlock()
	keys := make([]string, 0, len(testedEndpoints))
	for k := range testedEndpoints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func registerEndpointTestInspector() {
	RegisterTool("test_endpoint", "Make an HTTP request to own app and return full response (status, headers, body, duration). Useful for self-testing endpoints.", map[string]ToolParam{
		"method":  {Type: "string", Description: "HTTP method (GET, POST, PUT, DELETE). Default GET.", Required: false},
		"path":    {Type: "string", Description: "Request path (e.g. /api/orders)", Required: true},
		"headers": {Type: "object", Description: "Optional HTTP headers as key-value pairs", Required: false},
		"body":    {Type: "string", Description: "Optional request body (for POST/PUT)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		path, _ := args["path"].(string)
		if path == "" {
			return nil, fmtErrorf("path is required")
		}

		method, _ := args["method"].(string)
		if method == "" {
			method = "GET"
		}
		method = strings.ToUpper(method)

		// Build URL
		baseURL := "http://localhost:8080"
		port := getenv("PORT")
		if port != "" {
			baseURL = "http://localhost:" + port
		}
		url := baseURL + path

		// Build request body if provided
		var bodyReader io.Reader
		if bodyStr, ok := args["body"].(string); ok && bodyStr != "" {
			bodyReader = bytes.NewReader([]byte(bodyStr))
		}

		var req *http.Request
		var err error
		if bodyReader != nil {
			req, err = http.NewRequest(method, url, bodyReader)
		} else {
			req, err = http.NewRequest(method, url, nil)
		}
		if err != nil {
			return map[string]any{
				"error":  "Failed to create request",
				"detail": err.Error(),
			}, nil
		}

		// Set headers
		if headers, ok := args["headers"].(map[string]any); ok {
			for key, val := range headers {
				if s, ok := val.(string); ok {
					req.Header.Set(key, s)
				}
			}
		}

		// Set default content type if body provided and no content-type
		if bodyReader != nil && req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}

		// Make request with timeout
		client := &http.Client{Timeout: 10 * time.Second}
		start := time.Now()
		resp, err := client.Do(req)
		duration := time.Since(start)

		if err != nil {
			return map[string]any{
				"error":    "Request failed",
				"detail":   err.Error(),
				"method":   method,
				"url":      url,
				"duration": duration.String(),
			}, nil
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) > 5000 {
			bodyStr = bodyStr[:5000] + "\n... (truncated, 5000 chars)"
		}

		// Collect response headers
		respHeaders := make(map[string]string, len(resp.Header))
		for k, vs := range resp.Header {
			respHeaders[k] = strings.Join(vs, ", ")
		}

		// Record tested endpoint
		markEndpointTested(method, path)

		return map[string]any{
			"method":    method,
			"url":       url,
			"path":      path,
			"status":    resp.StatusCode,
			"status_text": resp.Status,
			"headers":   respHeaders,
			"body":      bodyStr,
			"body_size": len(body),
			"duration":  duration.String(),
			"duration_ms": duration.Milliseconds(),
		}, nil
	})

	RegisterTool("batch_test_endpoints", "Test multiple endpoints in one call. Returns results for each endpoint and compares against expected status codes if provided.", map[string]ToolParam{
		"tests": {Type: "array", Description: "Array of test objects: {method, path, headers (optional), body (optional), expected_status (optional)}", Required: true},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		testsRaw, ok := args["tests"].([]any)
		if !ok || len(testsRaw) == 0 {
			return nil, fmtErrorf("tests array is required and must not be empty")
		}

		baseURL := "http://localhost:8080"
		port := getenv("PORT")
		if port != "" {
			baseURL = "http://localhost:" + port
		}

		client := &http.Client{Timeout: 10 * time.Second}
		results := make([]map[string]any, 0, len(testsRaw))
		passCount := 0
		failCount := 0

		for i, testRaw := range testsRaw {
			test, ok := testRaw.(map[string]any)
			if !ok {
				results = append(results, map[string]any{
					"index": i,
					"error": "invalid test object (expected map)",
				})
				failCount++
				continue
			}

			path, _ := test["path"].(string)
			if path == "" {
				results = append(results, map[string]any{
					"index": i,
					"error": "path is required",
				})
				failCount++
				continue
			}

			method, _ := test["method"].(string)
			if method == "" {
				method = "GET"
			}
			method = strings.ToUpper(method)

			url := baseURL + path

			// Build request
			var bodyReader io.Reader
			if bodyStr, ok := test["body"].(string); ok && bodyStr != "" {
				bodyReader = bytes.NewReader([]byte(bodyStr))
			}

			var req *http.Request
			var err error
			if bodyReader != nil {
				req, err = http.NewRequest(method, url, bodyReader)
			} else {
				req, err = http.NewRequest(method, url, nil)
			}
			if err != nil {
				results = append(results, map[string]any{
					"index":  i,
					"path":   path,
					"error":  "Failed to create request: " + err.Error(),
					"passed": false,
				})
				failCount++
				continue
			}

			// Set headers
			if headers, ok := test["headers"].(map[string]any); ok {
				for key, val := range headers {
					if s, ok := val.(string); ok {
						req.Header.Set(key, s)
					}
				}
			}
			if bodyReader != nil && req.Header.Get("Content-Type") == "" {
				req.Header.Set("Content-Type", "application/json")
			}

			// Execute
			start := time.Now()
			resp, err := client.Do(req)
			duration := time.Since(start)

			if err != nil {
				results = append(results, map[string]any{
					"index":    i,
					"method":   method,
					"path":     path,
					"error":    "Request failed: " + err.Error(),
					"passed":   false,
					"duration": duration.String(),
				})
				failCount++
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// Truncate body
			bodyStr := string(body)
			if len(bodyStr) > 2000 {
				bodyStr = bodyStr[:2000] + "..."
			}

			// Check against expected status
			passed := true
			expectedStatus := 0
			if es, ok := test["expected_status"].(float64); ok {
				expectedStatus = int(es)
				if resp.StatusCode != expectedStatus {
					passed = false
				}
			}

			markEndpointTested(method, path)

			entry := map[string]any{
				"index":       i,
				"method":      method,
				"path":        path,
				"status":      resp.StatusCode,
				"duration":    duration.String(),
				"duration_ms": duration.Milliseconds(),
				"body":        bodyStr,
				"passed":      passed,
			}
			if expectedStatus > 0 {
				entry["expected_status"] = expectedStatus
				if !passed {
					entry["error"] = fmtSprintf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
				}
			}

			if passed {
				passCount++
			} else {
				failCount++
			}

			results = append(results, entry)
		}

		return map[string]any{
			"total":      len(results),
			"passed":     passCount,
			"failed":     failCount,
			"results":    results,
		}, nil
	})

	RegisterTool("get_endpoint_coverage", "Compare registered routes vs tested endpoints. Shows which routes have been tested and which have not.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		tested := getTestedEndpoints()

		// Collect all registered routes from Gin/Echo/Chi
		allRoutes := []string{}
		routesRegistryMu.RLock()
		for _, engine := range registeredGinEngines {
			routes, err := ginRoutesViaReflect(engine)
			if err != nil {
				continue
			}
			for _, r := range routes {
				method, _ := r["method"].(string)
				path, _ := r["path"].(string)
				if method != "" && path != "" {
					allRoutes = append(allRoutes, strings.ToUpper(method)+":"+path)
				}
			}
		}
		for _, app := range registeredEchoApps {
			routes, err := echoRoutesViaReflect(app)
			if err != nil {
				continue
			}
			for _, r := range routes {
				method, _ := r["method"].(string)
				path, _ := r["path"].(string)
				if method != "" && path != "" {
					allRoutes = append(allRoutes, strings.ToUpper(method)+":"+path)
				}
			}
		}
		routesRegistryMu.RUnlock()

		sort.Strings(allRoutes)

		// Build coverage map
		testedSet := map[string]bool{}
		for _, t := range tested {
			testedSet[t] = true
		}

		covered := make([]string, 0)
		uncovered := make([]string, 0)
		for _, route := range allRoutes {
			if testedSet[route] {
				covered = append(covered, route)
			} else {
				uncovered = append(uncovered, route)
			}
		}

		coveragePct := 0.0
		if len(allRoutes) > 0 {
			coveragePct = float64(len(covered)) / float64(len(allRoutes)) * 100.0
		}

		return map[string]any{
			"total_routes":      len(allRoutes),
			"total_tested":      len(tested),
			"covered_count":     len(covered),
			"uncovered_count":   len(uncovered),
			"coverage_percent":  fmtSprintf("%.1f%%", coveragePct),
			"covered":           covered,
			"uncovered":         uncovered,
			"all_tested":        tested,
		}, nil
	})
}
