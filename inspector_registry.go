package debugagent

import (
	"runtime/pprof"
	"sort"
)

func registerRegistryInspector() {
	RegisterTool("get_registered_services", "List all services registered with the debug agent: inspector name, tool count, description. Shows the full agent capability map.", nil, func(args map[string]any) (any, error) {
		services := groupToolsByInspector()

		list := make([]map[string]any, 0, len(services))
		totalTools := 0
		for _, svc := range services {
			list = append(list, map[string]any{
				"inspector":   svc.Name,
				"tool_count":  len(svc.Tools),
				"tools":       svc.Tools,
				"description": svc.Description,
			})
			totalTools += len(svc.Tools)
		}

		return map[string]any{
			"total_tools":     totalTools,
			"total_inspectors": len(list),
			"services":        list,
		}, nil
	})

	RegisterTool("get_service_dependencies", "Show dependency graph between registered components (if any registered).", nil, func(args map[string]any) (any, error) {
		// Build dependency graph from registered components
		deps := buildDependencyGraph()

		return map[string]any{
			"component_count":   len(deps),
			"dependencies":      deps,
			"note":              "Dependencies are inferred from registered components (DBs, caches, loggers, etc.)",
		}, nil
	})
}

// inspectorGroup represents a group of tools registered by one inspector.
type inspectorGroup struct {
	Name        string
	Tools       []string
	Description string
}

// groupToolsByInspector groups all registered tools by their inspector name,
// inferred from the tool name prefix.
func groupToolsByInspector() []inspectorGroup {
	names := ToolNames()
	sort.Strings(names)

	// Group by inspector prefix (first word before _ or known inspector patterns)
	groups := map[string][]string{}
	for _, name := range names {
		inspector := inferInspectorName(name)
		groups[inspector] = append(groups[inspector], name)
	}

	result := make([]inspectorGroup, 0, len(groups))
	for name, tools := range groups {
		result = append(result, inspectorGroup{
			Name:        name,
			Tools:       tools,
			Description: inspectorDescription(name),
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// inferInspectorName maps a tool name to its inspector category.
func inferInspectorName(toolName string) string {
	// Check known prefixes for each inspector
	categories := []struct {
		prefix   string
		category string
	}{
		{"get_memory_stats", "runtime"},
		{"trigger_gc", "runtime"},
		{"get_goroutine_dump", "runtime"},
		{"get_runtime_info", "runtime"},
		{"get_gc_stats", "runtime"},
		{"get_cpu_profile", "runtime"},
		{"get_goroutine", "goroutine"},
		{"get_http", "http"},
		{"get_process", "system"},
		{"get_system", "system"},
		{"get_disk", "system"},
		{"get_environment", "system"},
		{"get_build", "deployment"},
		{"get_deployment", "deployment"},
		{"get_module", "deployment"},
		{"get_alloc", "alloc"},
		{"get_mem", "alloc"},
		{"get_network", "network"},
		{"get_redis", "redis"},
		{"get_route", "routes"},
		{"get_gorm", "gorm"},
		{"get_mutex", "pprof"},
		{"get_block", "pprof"},
		{"get_threadcreate", "pprof"},
		{"start_cpu", "cpu_profile"},
		{"stop_cpu", "cpu_profile"},
		{"get_top_func", "cpu_profile"},
		{"take_heap_snap", "leak"},
		{"compare_heap", "leak"},
		{"get_leak", "leak"},
		{"get_runtime_version", "deployment"},
		{"take_snapshot", "snapshot"},
		{"compare_snapshot", "snapshot"},
		{"list_snapshot", "snapshot"},
		{"get_registered_services", "registry"},
		{"get_service_dep", "registry"},
		{"get_log", "logging"},
		{"get_cache", "cache"},
		{"get_http_client", "http_client"},
		{"get_fd", "fd"},
		{"get_registered_metric", "metrics"},
		{"get_context", "context"},
		{"get_wait", "sync"},
		{"get_security", "security"},
		{"get_health", "health"},
		{"get_schedul", "scheduler"},
		{"get_recent_error", "error"},
		{"get_error", "error"},
		{"get_websocket", "websocket"},
		{"detect_deadlock", "locks"},
		{"get_lock", "locks"},
		{"get_mutex_cont", "locks"},
		{"get_migration", "migration"},
		{"get_config", "config"},
		{"get_feature", "featureflag"},
		{"test_endpoint", "endpoint_test"},
		{"get_endpoint", "endpoint_test"},
		{"get_db_pool", "pool"},
		{"get_database_pool", "pool"},
		{"get_pool", "pool"},
	}

	for _, cat := range categories {
		if hasPrefix(toolName, cat.prefix) {
			return cat.category
		}
	}

	// Default: use first word
	for i := 0; i < len(toolName); i++ {
		if toolName[i] == '_' {
			return toolName[:i]
		}
	}
	return toolName
}

// hasPrefix checks if s starts with prefix.
func hasPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// inspectorDescription returns a human-readable description for an inspector.
func inspectorDescription(name string) string {
	descriptions := map[string]string{
		"runtime":       "Core Go runtime: memory, GC, goroutines, version info",
		"goroutine":     "Goroutine inspection: count, stacks, states",
		"http":          "HTTP request tracking: in-flight, completed, latency",
		"system":        "System info: process, disk, environment",
		"deployment":    "Build and deployment info: VCS, container, module versions",
		"alloc":         "Memory allocation: mallocs, frees, GC stats",
		"network":       "Network: connections, listeners, DNS",
		"redis":         "Redis client inspection: pool, latency, commands",
		"routes":        "Route inspection: Gin, Echo, Chi routers",
		"gorm":          "GORM ORM inspection: queries, models, operations",
		"pprof":         "pprof profiles: mutex, block, threadcreate",
		"cpu_profile":   "CPU profiling: start/stop, top functions by time",
		"leak":          "Memory leak detection: heap snapshots, comparison, candidates",
		"snapshot":      "Cross-inspector snapshots: gather all metrics, diff over time",
		"registry":      "Service registry: agent capability map, dependencies",
		"logging":       "Logging: recent logs, log levels, registered loggers",
		"cache":         "Cache inspection: registered caches, hit rates, sizes",
		"http_client":   "HTTP client inspection: outbound calls, timeouts",
		"fd":            "File descriptor: open count, limits",
		"metrics":       "Metrics: Prometheus-compatible metric gathering",
		"context":       "Context inspection: registered contexts, deadlines",
		"sync":          "Sync primitives: WaitGroup states",
		"security":      "Security: auth configs, session stores",
		"health":        "Health checks: registered health checks, status",
		"scheduler":     "Scheduler: scheduled jobs, execution history",
		"error":         "Error tracking: recent errors, patterns, statistics",
		"websocket":     "WebSocket: connections, rooms, messages",
		"locks":         "Locks & deadlocks: mutex contention, deadlock detection",
		"migration":     "Database migrations: status, pending, applied",
		"config":        "Configuration: registered configs, values",
		"featureflag":   "Feature flags: registered flags, states",
		"endpoint_test": "Endpoint testing: tested endpoints, coverage",
		"pool":          "Connection pool: DB pool stats, saturation, leaks",
	}
	if desc, ok := descriptions[name]; ok {
		return desc
	}
	return "Custom inspector"
}

// buildDependencyGraph builds a dependency graph from registered components.
func buildDependencyGraph() []map[string]any {
	defer func() {
		_ = recover()
	}()

	var deps []map[string]any

	// DB pools
	dbRegistryMu.Lock()
	for i := range dbNames {
		deps = append(deps, map[string]any{
			"component":  "database_pool",
			"name":       dbNames[i],
			"depends_on": []string{"network"},
			"type":       "sql.DB",
		})
	}
	dbRegistryMu.Unlock()

	// GORM
	gormRegistryMu.RLock()
	for name := range gormRegistry {
		deps = append(deps, map[string]any{
			"component":  "gorm_db",
			"name":       name,
			"depends_on": []string{"database_pool"},
			"type":       "gorm.DB",
		})
	}
	gormRegistryMu.RUnlock()

	// Redis
	redisRegistryMu.RLock()
	for name := range registeredRedisClients {
		deps = append(deps, map[string]any{
			"component":  "redis_client",
			"name":       name,
			"depends_on": []string{"network"},
			"type":       "redis.Client",
		})
	}
	redisRegistryMu.RUnlock()

	// Caches
	cacheRegistryMu.RLock()
	for name := range cacheRegistry {
		deps = append(deps, map[string]any{
			"component":  "cache",
			"name":       name,
			"depends_on": []string{},
			"type":       "cache",
		})
	}
	cacheRegistryMu.RUnlock()

	// HTTP servers (routes)
	routesRegistryMu.RLock()
	for name := range registeredGinEngines {
		deps = append(deps, map[string]any{
			"component":  "http_server",
			"name":       name,
			"depends_on": []string{"network"},
			"type":       "gin.Engine",
		})
	}
	for name := range registeredEchoApps {
		deps = append(deps, map[string]any{
			"component":  "http_server",
			"name":       name,
			"depends_on": []string{"network"},
			"type":       "echo.Echo",
		})
	}
	routesRegistryMu.RUnlock()

	// Loggers
	loggerRegistryMu.RLock()
	for name := range loggerRegistry {
		deps = append(deps, map[string]any{
			"component":  "logger",
			"name":       name,
			"depends_on": []string{},
			"type":       "logger",
		})
	}
	loggerRegistryMu.RUnlock()

	// HTTP clients
	httpClientRegistryMu.RLock()
	for name := range httpClientRegistry {
		deps = append(deps, map[string]any{
			"component":  "http_client",
			"name":       name,
			"depends_on": []string{"network"},
			"type":       "http.Client",
		})
	}
	httpClientRegistryMu.RUnlock()

	return deps
}

// pprofLookupThreadcreate wraps pprof.Lookup for the threadcreate profile.
func pprofLookupThreadcreate() *pprof.Profile {
	return pprof.Lookup("threadcreate")
}
