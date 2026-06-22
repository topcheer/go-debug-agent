package debugagent

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"
)

// ─── Redis client registry ──────────────────────────────────────────────────

var (
	registeredRedisClients = map[string]any{}
	redisRegistryMu        sync.RWMutex
)

// RegisterRedisClient registers a go-redis client (any type with PoolStats/Info/Ping
// methods, e.g. *redis.Client) for inspection by the debug agent.
func RegisterRedisClient(name string, client any) {
	redisRegistryMu.Lock()
	defer redisRegistryMu.Unlock()
	registeredRedisClients[name] = client
}

// ─── Reflection helpers ─────────────────────────────────────────────────────

// redisCallCmdNoArg calls a no-arg method (other than ctx) on the redis client,
// then invokes .Result() on the returned Cmder and returns (stringResult, error).
func redisCallStringCmd(client any, methodName string, ctx context.Context) (string, error) {
	v := reflect.ValueOf(client)
	method := v.MethodByName(methodName)
	if !method.IsValid() {
		return "", fmt.Errorf("method %s not found on redis client", methodName)
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(ctx)})
	if len(results) == 0 {
		return "", fmt.Errorf("%s returned no results", methodName)
	}

	cmd := results[0]
	resultMethod := cmd.MethodByName("Result")
	if !resultMethod.IsValid() {
		return "", fmt.Errorf("Result method not found on cmd returned by %s", methodName)
	}

	cmdResults := resultMethod.Call(nil)
	if len(cmdResults) >= 2 && cmdResults[1].CanInterface() {
		if err, ok := cmdResults[1].Interface().(error); ok && err != nil {
			return "", err
		}
	}
	if len(cmdResults) >= 1 && cmdResults[0].CanInterface() {
		return cmdResults[0].String(), nil
	}
	return "", nil
}

// redisCallIntCmd is like redisCallStringCmd but returns int64.
func redisCallIntCmd(client any, methodName string, ctx context.Context) (int64, error) {
	v := reflect.ValueOf(client)
	method := v.MethodByName(methodName)
	if !method.IsValid() {
		return 0, fmt.Errorf("method %s not found on redis client", methodName)
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(ctx)})
	if len(results) == 0 {
		return 0, fmt.Errorf("%s returned no results", methodName)
	}

	cmd := results[0]
	resultMethod := cmd.MethodByName("Result")
	if !resultMethod.IsValid() {
		return 0, fmt.Errorf("Result method not found on cmd returned by %s", methodName)
	}

	cmdResults := resultMethod.Call(nil)
	if len(cmdResults) >= 2 && cmdResults[1].CanInterface() {
		if err, ok := cmdResults[1].Interface().(error); ok && err != nil {
			return 0, err
		}
	}
	if len(cmdResults) >= 1 {
		return cmdResults[0].Int(), nil
	}
	return 0, nil
}

// redisPoolStatsViaReflect calls PoolStats() on the client and extracts fields.
func redisPoolStatsViaReflect(client any) (map[string]any, error) {
	v := reflect.ValueOf(client)
	method := v.MethodByName("PoolStats")
	if !method.IsValid() {
		return nil, fmt.Errorf("PoolStats method not found on redis client")
	}

	results := method.Call(nil)
	if len(results) == 0 {
		return nil, fmt.Errorf("PoolStats returned no results")
	}

	stats := results[0]
	if stats.Kind() == reflect.Ptr {
		stats = stats.Elem()
	}

	getField := func(name string) uint64 {
		f := stats.FieldByName(name)
		if !f.IsValid() {
			return 0
		}
		return f.Uint()
	}

	return map[string]any{
		"hits":         getField("Hits"),
		"misses":       getField("Misses"),
		"timeouts":     getField("Timeouts"),
		"total_conns":  getField("TotalConns"),
		"idle_conns":   getField("IdleConns"),
		"stale_conns":  getField("StaleConns"),
	}, nil
}

// ─── INFO parser ────────────────────────────────────────────────────────────

func parseRedisInfo(raw string) map[string]any {
	result := map[string]any{}
	lines := strings.Split(raw, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "connected_clients", "used_memory", "used_memory_human",
			"used_memory_peak", "used_memory_peak_human",
			"uptime_in_seconds", "uptime_in_days",
			"redis_version", "redis_mode",
			"total_connections_received", "total_commands_processed",
			"rejected_connections", "expired_keys", "evicted_keys":
			result[key] = val
		}
	}
	return result
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerRedisInspector() {
	RegisterTool("get_redis_pool_stats", "Get connection pool stats for all registered Redis clients (Hits, Misses, Timeouts, TotalConns, IdleConns, StaleConns)", nil, func(args map[string]any) (any, error) {
		redisRegistryMu.RLock()
		defer redisRegistryMu.RUnlock()

		if len(registeredRedisClients) == 0 {
			return map[string]any{
				"message": "No Redis clients registered. Call debugagent.RegisterRedisClient(name, client) to enable Redis inspection.",
				"count":   0,
			}, nil
		}

		clients := make([]map[string]any, 0, len(registeredRedisClients))
		for name, client := range registeredRedisClients {
			stats, err := redisPoolStatsViaReflect(client)
			entry := map[string]any{"name": name}
			if err != nil {
				entry["error"] = err.Error()
			} else {
				for k, v := range stats {
					entry[k] = v
				}
			}
			clients = append(clients, entry)
		}

		return map[string]any{
			"count":   len(registeredRedisClients),
			"clients": clients,
		}, nil
	})

	RegisterTool("get_redis_info", "Run INFO command on all registered Redis clients and parse key fields (connected_clients, used_memory, db_size, uptime)", map[string]ToolParam{
		"name": {Type: "string", Description: "Specific client name (default: all registered clients)", Required: false},
	}, func(args map[string]any) (any, error) {
		redisRegistryMu.RLock()
		defer redisRegistryMu.RUnlock()

		if len(registeredRedisClients) == 0 {
			return map[string]any{
				"message": "No Redis clients registered.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["name"].(string)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		results := make([]map[string]any, 0, len(registeredRedisClients))
		for name, client := range registeredRedisClients {
			if targetName != "" && targetName != name {
				continue
			}
			entry := map[string]any{"name": name}

			// Run INFO
			infoRaw, err := redisCallStringCmd(client, "Info", ctx)
			if err != nil {
				entry["error"] = fmt.Sprintf("INFO failed: %v", err)
				results = append(results, entry)
				continue
			}
			entry["info"] = parseRedisInfo(infoRaw)

			// Run DBSize
			dbSize, err := redisCallIntCmd(client, "DBSize", ctx)
			if err != nil {
				entry["db_size_error"] = err.Error()
			} else {
				entry["db_size"] = dbSize
			}

			results = append(results, entry)
		}

		return map[string]any{
			"count":   len(results),
			"clients": results,
		}, nil
	})

	RegisterTool("get_redis_latency", "Measure Redis PING latency (min/avg/max) over 10 samples for all registered clients", map[string]ToolParam{
		"name":     {Type: "string", Description: "Specific client name (default: all registered clients)", Required: false},
		"samples":  {Type: "integer", Description: "Number of PING samples (default 10, max 50)", Required: false},
	}, func(args map[string]any) (any, error) {
		redisRegistryMu.RLock()
		defer redisRegistryMu.RUnlock()

		if len(registeredRedisClients) == 0 {
			return map[string]any{
				"message": "No Redis clients registered.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["name"].(string)
		numSamples := 10
		if v, ok := args["samples"].(float64); ok && int(v) > 0 && int(v) <= 50 {
			numSamples = int(v)
		}

		results := make([]map[string]any, 0, len(registeredRedisClients))
		for name, client := range registeredRedisClients {
			if targetName != "" && targetName != name {
				continue
			}
			entry := map[string]any{"name": name, "samples": numSamples}

			var durations []time.Duration
			var lastErr error
			for i := 0; i < numSamples; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				start := time.Now()
				_, err := redisCallStringCmd(client, "Ping", ctx)
				elapsed := time.Since(start)
				cancel()
				if err != nil {
					lastErr = err
					continue
				}
				durations = append(durations, elapsed)
			}

			if len(durations) == 0 {
				entry["error"] = fmt.Sprintf("All PING attempts failed: %v", lastErr)
				results = append(results, entry)
				continue
			}

			var minDur, maxDur, totalDur time.Duration
			minDur = durations[0]
			maxDur = durations[0]
			for _, d := range durations {
				if d < minDur {
					minDur = d
				}
				if d > maxDur {
					maxDur = d
				}
				totalDur += d
			}
			avgDur := totalDur / time.Duration(len(durations))

			entry["min_us"] = minDur.Microseconds()
			entry["avg_us"] = avgDur.Microseconds()
			entry["max_us"] = maxDur.Microseconds()
			entry["min"] = minDur.String()
			entry["avg"] = avgDur.String()
			entry["max"] = maxDur.String()
			entry["successful_pings"] = len(durations)
			entry["failed_pings"] = numSamples - len(durations)

			results = append(results, entry)
		}

		return map[string]any{
			"count":   len(results),
			"clients": results,
		}, nil
	})
}
