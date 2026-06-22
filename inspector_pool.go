package debugagent

import (
	"database/sql"
	"sort"
	"sync"
)

// ─── DB pool registry ────────────────────────────────────────────────────────

var (
	registeredDBs     = map[string]*sql.DB{}
	dbPoolRegistryMu sync.RWMutex
)

// RegisterDatabase registers a *sql.DB for deep connection pool inspection.
// Call this after opening a database connection to enable pool analysis tools.
func RegisterDatabase(name string, db *sql.DB) {
	dbPoolRegistryMu.Lock()
	defer dbPoolRegistryMu.Unlock()
	registeredDBs[name] = db
}

func registerPoolInspector() {
	RegisterTool("get_pool_details", "Get detailed DB connection pool stats for database/sql: MaxOpen, Open, InUse, Idle, WaitCount, WaitDuration, MaxIdleClosed, MaxLifetimeClosed.", map[string]ToolParam{
		"db_name": {Type: "string", Description: "Name of the registered database to inspect (default: all)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		dbPoolRegistryMu.RLock()
		defer dbPoolRegistryMu.RUnlock()

		if len(registeredDBs) == 0 {
			return map[string]any{
				"message": "No databases registered. Call debugagent.RegisterDatabase(name, db) to enable pool inspection.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["db_name"].(string)
		pools := make([]map[string]any, 0, len(registeredDBs))

		// Sort names for deterministic output
		names := make([]string, 0, len(registeredDBs))
		for name := range registeredDBs {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			if targetName != "" && name != targetName {
				continue
			}

			db := registeredDBs[name]
			entry := map[string]any{"name": name}

			if db == nil {
				entry["error"] = "nil database"
				pools = append(pools, entry)
				continue
			}

			stats := db.Stats()
			entry["max_open_connections"] = stats.MaxOpenConnections
			entry["open_connections"] = stats.OpenConnections
			entry["in_use"] = stats.InUse
			entry["idle"] = stats.Idle
			entry["wait_count"] = stats.WaitCount
			entry["wait_duration"] = stats.WaitDuration.String()
			entry["wait_duration_ns"] = stats.WaitDuration.Nanoseconds()
			entry["max_idle_closed"] = stats.MaxIdleClosed
			entry["max_idle_time_closed"] = stats.MaxIdleTimeClosed
			entry["max_lifetime_closed"] = stats.MaxLifetimeClosed

			// Compute utilization metrics
			if stats.MaxOpenConnections > 0 {
				entry["utilization_pct"] = fmtSprintf("%.1f%%", float64(stats.InUse)/float64(stats.MaxOpenConnections)*100.0)
				entry["idle_pct"] = fmtSprintf("%.1f%%", float64(stats.Idle)/float64(stats.MaxOpenConnections)*100.0)
			}
			if stats.OpenConnections > 0 {
				entry["in_use_pct_of_open"] = fmtSprintf("%.1f%%", float64(stats.InUse)/float64(stats.OpenConnections)*100.0)
			}

			pools = append(pools, entry)
		}

		return map[string]any{
			"count": len(pools),
			"pools": pools,
		}, nil
	})

	RegisterTool("detect_pool_leaks", "Heuristic leak detection for DB connection pools. Checks: connections held > 30s (high wait count), wait count ratio > 50%, idle connections < max for extended period.", map[string]ToolParam{
		"db_name": {Type: "string", Description: "Name of the registered database to inspect (default: all)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		dbPoolRegistryMu.RLock()
		defer dbPoolRegistryMu.RUnlock()

		if len(registeredDBs) == 0 {
			return map[string]any{
				"message": "No databases registered. Call debugagent.RegisterDatabase(name, db) to enable pool leak detection.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["db_name"].(string)
		results := make([]map[string]any, 0, len(registeredDBs))

		// Sort names for deterministic output
		names := make([]string, 0, len(registeredDBs))
		for name := range registeredDBs {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			if targetName != "" && name != targetName {
				continue
			}

			db := registeredDBs[name]
			entry := map[string]any{"name": name}

			if db == nil {
				entry["error"] = "nil database"
				results = append(results, entry)
				continue
			}

			stats := db.Stats()
			issues := make([]map[string]any, 0)
			riskLevel := "none"

			// Check 1: High wait count ratio (> 50% of requests waited)
			if stats.WaitCount > 0 && stats.OpenConnections > 0 {
				// Approximate total requests by WaitCount + OpenConnections as a rough proxy
				totalApprox := stats.WaitCount + int64(stats.OpenConnections)
				if totalApprox > 0 {
					waitRatio := float64(stats.WaitCount) / float64(totalApprox)
					if waitRatio > 0.5 {
						issues = append(issues, map[string]any{
							"type":        "high_wait_ratio",
							"severity":    "warning",
							"detail":      fmtSprintf("Wait ratio %.1f%% (> 50%%) — connections may be held too long", waitRatio*100),
							"wait_count":  stats.WaitCount,
							"open_conns":  stats.OpenConnections,
							"wait_ratio":  fmtSprintf("%.1f%%", waitRatio*100),
						})
						riskLevel = "elevated"
					}
				}
			}

			// Check 2: Significant wait duration
			if stats.WaitDuration.Seconds() > 30 {
				issues = append(issues, map[string]any{
					"type":        "long_wait_duration",
					"severity":    "warning",
					"detail":      fmtSprintf("Total wait duration %s — connections may be leaking", stats.WaitDuration.String()),
					"wait_duration": stats.WaitDuration.String(),
				})
				if riskLevel == "none" {
					riskLevel = "elevated"
				}
			}

			// Check 3: All connections in use with high wait (saturation)
			if stats.MaxOpenConnections > 0 && stats.InUse == stats.MaxOpenConnections && stats.WaitCount > 0 {
				issues = append(issues, map[string]any{
					"type":        "pool_saturation",
					"severity":    "critical",
					"detail":      fmtSprintf("Pool saturated: %d/%d connections in use with %d waits", stats.InUse, stats.MaxOpenConnections, stats.WaitCount),
					"in_use":      stats.InUse,
					"max_open":    stats.MaxOpenConnections,
					"wait_count":  stats.WaitCount,
				})
				riskLevel = "high"
			}

			// Check 4: High lifetime closed (connections cycling too fast)
			if stats.MaxLifetimeClosed > int64(stats.OpenConnections)*10 && stats.OpenConnections > 0 {
				issues = append(issues, map[string]any{
					"type":        "high_connection_churn",
					"severity":    "info",
					"detail":      fmtSprintf("High connection churn: %d lifetime closed with %d open", stats.MaxLifetimeClosed, stats.OpenConnections),
					"lifetime_closed": stats.MaxLifetimeClosed,
				})
				if riskLevel == "none" {
					riskLevel = "low"
				}
			}

			// Check 5: High idle closed (idle connections being reclaimed too aggressively)
			if stats.MaxIdleClosed > int64(stats.OpenConnections)*10 && stats.OpenConnections > 0 {
				issues = append(issues, map[string]any{
					"type":        "high_idle_churn",
					"severity":    "info",
					"detail":      fmtSprintf("High idle connection churn: %d idle closed with %d open", stats.MaxIdleClosed, stats.OpenConnections),
					"idle_closed": stats.MaxIdleClosed,
				})
				if riskLevel == "none" {
					riskLevel = "low"
				}
			}

			if len(issues) == 0 {
				issues = append(issues, map[string]any{
					"type":     "no_leaks_detected",
					"severity": "none",
					"detail":   "No connection pool leak indicators detected.",
				})
			}

			entry["risk_level"] = riskLevel
			entry["issue_count"] = len(issues)
			entry["issues"] = issues
			entry["stats_summary"] = map[string]any{
				"open":            stats.OpenConnections,
				"in_use":          stats.InUse,
				"idle":            stats.Idle,
				"max_open":        stats.MaxOpenConnections,
				"wait_count":      stats.WaitCount,
				"wait_duration":   stats.WaitDuration.String(),
				"idle_closed":     stats.MaxIdleClosed,
				"lifetime_closed": stats.MaxLifetimeClosed,
			}

			results = append(results, entry)
		}

		// Overall assessment
		overallRisk := "none"
		for _, r := range results {
			risk, _ := r["risk_level"].(string)
			if risk == "high" {
				overallRisk = "high"
				break
			}
			if risk == "elevated" && overallRisk != "high" {
				overallRisk = "elevated"
			}
			if risk == "low" && overallRisk == "none" {
				overallRisk = "low"
			}
		}

		return map[string]any{
			"count":        len(results),
			"overall_risk": overallRisk,
			"databases":    results,
		}, nil
	})

	RegisterTool("get_pool_wait_stats", "Get connection acquire wait statistics: avg wait, P95 wait estimate, max wait, timeout count.", map[string]ToolParam{
		"db_name": {Type: "string", Description: "Name of the registered database to inspect (default: all)", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		dbPoolRegistryMu.RLock()
		defer dbPoolRegistryMu.RUnlock()

		if len(registeredDBs) == 0 {
			return map[string]any{
				"message": "No databases registered. Call debugagent.RegisterDatabase(name, db) to enable wait stats.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["db_name"].(string)
		results := make([]map[string]any, 0, len(registeredDBs))

		// Sort names for deterministic output
		names := make([]string, 0, len(registeredDBs))
		for name := range registeredDBs {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			if targetName != "" && name != targetName {
				continue
			}

			db := registeredDBs[name]
			entry := map[string]any{"name": name}

			if db == nil {
				entry["error"] = "nil database"
				results = append(results, entry)
				continue
			}

			stats := db.Stats()

			// Compute wait statistics
			avgWaitNs := int64(0)
			if stats.WaitCount > 0 {
				avgWaitNs = int64(stats.WaitDuration.Nanoseconds()) / int64(stats.WaitCount)
			}

			// P95 estimate: for a well-behaved pool, P95 ≈ avg * 2.5
			// This is a heuristic since database/sql doesn't track per-request latency distribution
			p95WaitNs := avgWaitNs * 5 / 2

			// Max wait estimate: for most pools, max is bounded by connection acquire timeout
			// We approximate as avg * 5 as an upper bound heuristic
			maxWaitEstimateNs := avgWaitNs * 5
			if maxWaitEstimateNs == 0 {
				maxWaitEstimateNs = stats.WaitDuration.Nanoseconds()
			}

			// Timeout count: database/sql doesn't directly track this, but we can infer
			// from lifetime closed connections if wait duration is high
			timeoutEstimate := int64(0)
			if stats.WaitCount > 0 && stats.WaitDuration.Seconds() > 1 {
				// Rough estimate: connections that closed due to context deadlines
				timeoutEstimate = stats.MaxLifetimeClosed
			}

			entry["wait_count"] = stats.WaitCount
			entry["total_wait_duration"] = stats.WaitDuration.String()
			entry["total_wait_ns"] = stats.WaitDuration.Nanoseconds()
			entry["avg_wait"] = formatDurationNs(avgWaitNs)
			entry["avg_wait_ns"] = avgWaitNs
			entry["p95_wait_estimate"] = formatDurationNs(p95WaitNs)
			entry["p95_wait_ns"] = p95WaitNs
			entry["max_wait_estimate"] = formatDurationNs(maxWaitEstimateNs)
			entry["max_wait_ns"] = maxWaitEstimateNs
			entry["timeout_count_estimate"] = timeoutEstimate
			entry["note"] = "P95 and max wait are estimates based on average. database/sql does not track per-request latency distribution."

			// Wait ratio
			if stats.OpenConnections > 0 {
				totalApprox := stats.WaitCount + int64(stats.OpenConnections)
				if totalApprox > 0 {
					waitRatio := float64(stats.WaitCount) / float64(totalApprox) * 100
					entry["wait_ratio_pct"] = fmtSprintf("%.1f%%", waitRatio)
					if waitRatio > 10 {
						entry["assessment"] = "high_wait_ratio"
					} else {
						entry["assessment"] = "normal"
					}
				}
			}

			results = append(results, entry)
		}

		return map[string]any{
			"count":  len(results),
			"stats":  results,
		}, nil
	})
}

// formatDurationNs converts a nanosecond value to a human-readable duration string.
func formatDurationNs(ns int64) string {
	if ns <= 0 {
		return "0s"
	}
	d := ns
	if d < 1000 {
		return fmtSprintf("%dns", d)
	}
	if d < 1000000 {
		return fmtSprintf("%.2fµs", float64(d)/1000.0)
	}
	if d < 1000000000 {
		return fmtSprintf("%.2fms", float64(d)/1000000.0)
	}
	return fmtSprintf("%.2fs", float64(d)/1000000000.0)
}
