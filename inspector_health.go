package debugagent

import (
	"fmt"
	"sync"
)

// ─── Health check registry ──────────────────────────────────────────────────

// HealthCheck represents a single health check component.
type HealthCheck struct {
	Name   string
	Check  func() (status string, details map[string]any)
}

var (
	healthChecks = map[string]HealthCheck{}
	healthMu     sync.RWMutex
)

// RegisterHealthCheck registers a health check that can be executed on demand.
// The check function returns a status ("UP" or "DOWN") and optional details.
func RegisterHealthCheck(name string, check func() (string, map[string]any)) {
	healthMu.Lock()
	defer healthMu.Unlock()
	healthChecks[name] = HealthCheck{Name: name, Check: check}
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerHealthInspector() {
	RegisterTool("get_health_status", "Get aggregated health status from all registered health checks (each check: name, status UP/DOWN, details)", nil, func(args map[string]any) (any, error) {
		healthMu.RLock()
		defer healthMu.RUnlock()

		if len(healthChecks) == 0 {
			return map[string]any{
				"message": "No health checks registered. Call debugagent.RegisterHealthCheck(name, check) to enable health inspection.",
				"count":   0,
			}, nil
		}

		checks := make([]map[string]any, 0, len(healthChecks))
		overallStatus := "UP"
		upCount := 0
		downCount := 0

		for name, hc := range healthChecks {
			entry := map[string]any{"name": name}
			status, details := executeHealthCheck(hc)
			entry["status"] = status
			if details != nil {
				entry["details"] = details
			}
			if status == "UP" {
				upCount++
			} else {
				downCount++
				overallStatus = "DEGRADED"
			}
			checks = append(checks, entry)
		}

		if downCount > 0 && upCount == 0 {
			overallStatus = "DOWN"
		}

		return map[string]any{
			"overall_status": overallStatus,
			"total_checks":   len(healthChecks),
			"up":             upCount,
			"down":           downCount,
			"checks":         checks,
		}, nil
	})

	RegisterTool("get_health_detail", "Deep dive into a specific health check component with full details", map[string]ToolParam{
		"component_name": {Type: "string", Description: "Name of the health check component to inspect", Required: true},
	}, func(args map[string]any) (any, error) {
		healthMu.RLock()
		defer healthMu.RUnlock()

		componentName, _ := args["component_name"].(string)
		if componentName == "" {
			return nil, fmtErrorf("component_name is required")
		}

		hc, ok := healthChecks[componentName]
		if !ok {
			// List available components
			available := make([]string, 0, len(healthChecks))
			for name := range healthChecks {
				available = append(available, name)
			}
			return map[string]any{
				"error":     "Health check component not found: " + componentName,
				"available": available,
			}, nil
		}

		status, details := executeHealthCheck(hc)
		result := map[string]any{
			"name":    componentName,
			"status":  status,
			"details": details,
		}
		return result, nil
	})

	RegisterTool("run_health_check", "Execute a specific health check on demand and return the result", map[string]ToolParam{
		"component_name": {Type: "string", Description: "Name of the health check to run (default: all)", Required: false},
	}, func(args map[string]any) (any, error) {
		healthMu.RLock()
		defer healthMu.RUnlock()

		targetName, _ := args["component_name"].(string)

		if targetName != "" {
			hc, ok := healthChecks[targetName]
			if !ok {
				return map[string]any{"error": "Unknown health check: " + targetName}, nil
			}
			status, details := executeHealthCheck(hc)
			return map[string]any{
				"component": targetName,
				"status":    status,
				"details":   details,
			}, nil
		}

		// Run all checks
		results := make([]map[string]any, 0, len(healthChecks))
		for name, hc := range healthChecks {
			status, details := executeHealthCheck(hc)
			results = append(results, map[string]any{
				"component": name,
				"status":    status,
				"details":   details,
			})
		}
		return map[string]any{
			"count":   len(results),
			"results": results,
		}, nil
	})
}

// executeHealthCheck safely runs a health check with panic recovery.
func executeHealthCheck(hc HealthCheck) (status string, details map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			status = "DOWN"
			details = map[string]any{
				"error":     "Health check panicked",
				"panic":     fmtSprintf("%v", r),
			}
		}
	}()

	if hc.Check == nil {
		return "UNKNOWN", map[string]any{"error": "No check function registered"}
	}

	status, details = hc.Check()
	if status == "" {
		status = "UNKNOWN"
	}
	return status, details
}

// fmtSprintf wraps fmt.Sprintf for use across inspectors.
func fmtSprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// fmtErrorf wraps fmt.Errorf for use across inspectors.
func fmtErrorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}
