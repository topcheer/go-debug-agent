package debugagent

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ─── Cache registry ───────────────────────────────────────────────────────────

var (
	cacheRegistry   = map[string]any{}
	cacheRegistryMu sync.RWMutex
)

// RegisterCache registers a cache (sync.Map, bigcache, ristretto, go-cache, or
// any cache exposing Stats()/Len() methods) for inspection.
func RegisterCache(name string, cache any) {
	cacheRegistryMu.Lock()
	defer cacheRegistryMu.Unlock()
	cacheRegistry[name] = cache
}

func registerCacheInspector() {
	RegisterTool("get_cache_stats", "Get stats for all registered caches (hit rate, miss count, key count, size bytes)", map[string]ToolParam{
		"cache_name": {Type: "string", Description: "Filter to a specific cache (optional)", Required: false},
	}, func(args map[string]any) (any, error) {
		cacheRegistryMu.RLock()
		defer cacheRegistryMu.RUnlock()

		if len(cacheRegistry) == 0 {
			return map[string]any{
				"message": "No caches registered. Call RegisterCache(name, cache) to enable cache inspection.",
				"count":   0,
			}, nil
		}

		filterName, _ := args["cache_name"].(string)
		caches := make([]map[string]any, 0, len(cacheRegistry))
		for name, c := range cacheRegistry {
			if filterName != "" && name != filterName {
				continue
			}
			caches = append(caches, inspectCache(name, c))
		}

		return map[string]any{"count": len(caches), "caches": caches}, nil
	})

	RegisterTool("get_cache_keys", "List keys in a registered cache with optional prefix filter", map[string]ToolParam{
		"cache_name": {Type: "string", Description: "Name of the registered cache", Required: true},
		"prefix":     {Type: "string", Description: "Filter keys by prefix (optional)", Required: false},
		"limit":      {Type: "integer", Description: "Max number of keys to return (default 100)", Required: false},
	}, func(args map[string]any) (any, error) {
		name, _ := args["cache_name"].(string)
		if name == "" {
			return nil, fmt.Errorf("cache_name is required")
		}
		prefix, _ := args["prefix"].(string)
		limit := 100
		if v, ok := args["limit"].(float64); ok && int(v) > 0 {
			limit = int(v)
		}

		cacheRegistryMu.RLock()
		c, ok := cacheRegistry[name]
		cacheRegistryMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("cache %q not registered", name)
		}

		keys := extractCacheKeys(c, prefix, limit)
		return map[string]any{
			"cache_name": name,
			"count":      len(keys),
			"keys":       keys,
		}, nil
	})

	RegisterTool("clear_cache", "Clear all entries from a registered cache", map[string]ToolParam{
		"cache_name": {Type: "string", Description: "Name of the registered cache", Required: true},
	}, func(args map[string]any) (any, error) {
		name, _ := args["cache_name"].(string)
		if name == "" {
			return nil, fmt.Errorf("cache_name is required")
		}

		cacheRegistryMu.RLock()
		c, ok := cacheRegistry[name]
		cacheRegistryMu.RUnlock()
		if !ok {
			return nil, fmt.Errorf("cache %q not registered", name)
		}

		err := clearCache(c)
		if err != nil {
			return nil, err
		}
		return map[string]any{"cache_name": name, "status": "cleared"}, nil
	})
}

// ─── Cache inspection helpers ─────────────────────────────────────────────────

func inspectCache(name string, c any) map[string]any {
	entry := map[string]any{"name": name}
	if c == nil {
		entry["error"] = "nil cache"
		return entry
	}

	// Direct type check for sync.Map (stdlib)
	if m, ok := c.(*sync.Map); ok {
		entry["type"] = "sync.Map"
		count := 0
		m.Range(func(_, _ any) bool {
			count++
			return true
		})
		entry["key_count"] = count
		return entry
	}

	// Use reflection for third-party caches
	rv := reflect.ValueOf(c)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			entry["error"] = "nil pointer"
			return entry
		}
		rv = rv.Elem()
	}

	typeName := rv.Type().String()
	entry["type"] = typeName

	// Try Stats() method → returns a struct with hit/miss/etc fields
	if stats := reflectCallStructMethod(c, "Stats"); stats != nil {
		entry["stats"] = stats
	}

	// Try Len() method
	if n := reflectCallIntMethod(c, "Len"); n >= 0 {
		entry["key_count"] = n
	}

	// Try ItemCount() method (go-cache)
	if n := reflectCallIntMethod(c, "ItemCount"); n >= 0 {
		entry["key_count"] = n
	}

	// For bigcache, also try to get hits/misses from stats
	// For ristretto, the metrics are accessible via Metrics() or similar
	if metrics := reflectCallStructMethod(c, "Metrics"); metrics != nil {
		entry["metrics"] = metrics
	}

	return entry
}

func extractCacheKeys(c any, prefix string, limit int) []string {
	// sync.Map: iterate with Range
	if m, ok := c.(*sync.Map); ok {
		var keys []string
		m.Range(func(key, _ any) bool {
			keyStr := fmt.Sprintf("%v", key)
			if prefix == "" || strings.HasPrefix(keyStr, prefix) {
				keys = append(keys, keyStr)
				if len(keys) >= limit {
					return false
				}
			}
			return true
		})
		sort.Strings(keys)
		return keys
	}

	// Reflection: try Items() method (go-cache, bigcache)
	rv := reflect.ValueOf(c)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	// Try Items() returning a map
	if method := reflect.ValueOf(c).MethodByName("Items"); method.IsValid() {
		results := method.Call(nil)
		if len(results) > 0 && results[0].Kind() == reflect.Map {
			var keys []string
			for _, key := range results[0].MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				if prefix == "" || strings.HasPrefix(keyStr, prefix) {
					keys = append(keys, keyStr)
					if len(keys) >= limit {
						break
					}
				}
			}
			sort.Strings(keys)
			return keys
		}
	}

	// Try GetKeys() or Keys() method
	for _, methodName := range []string{"Keys", "GetKeys"} {
		if method := reflect.ValueOf(c).MethodByName(methodName); method.IsValid() {
			results := method.Call(nil)
			if len(results) > 0 && results[0].Kind() == reflect.Slice {
				var keys []string
				slice := results[0]
				for i := 0; i < slice.Len(); i++ {
					keyStr := fmt.Sprintf("%v", slice.Index(i).Interface())
					if prefix == "" || strings.HasPrefix(keyStr, prefix) {
						keys = append(keys, keyStr)
						if len(keys) >= limit {
							break
						}
					}
				}
				return keys
			}
		}
	}

	return nil
}

func clearCache(c any) error {
	// sync.Map: delete all
	if m, ok := c.(*sync.Map); ok {
		m.Range(func(key, _ any) bool {
			m.Delete(key)
			return true
		})
		return nil
	}

	// Reflection: try Clear, Flush, Reset, or Purge methods
	for _, methodName := range []string{"Clear", "Flush", "Reset", "Purge"} {
		if method := reflect.ValueOf(c).MethodByName(methodName); method.IsValid() {
			method.Call(nil)
			return nil
		}
	}

	return fmt.Errorf("cache type %T does not support Clear/Flush/Reset/Purge", c)
}
