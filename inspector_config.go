package debugagent

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ─── Config registry ────────────────────────────────────────────────────────

var (
	registeredConfigs   = map[string]any{}
	configRegistryMu   sync.RWMutex
)

// RegisterConfig registers a configuration struct/value for inspection.
// The debug agent will expose all fields and automatically mask sensitive keys.
func RegisterConfig(name string, config any) {
	configRegistryMu.Lock()
	defer configRegistryMu.Unlock()
	registeredConfigs[name] = config
}

func registerConfigInspector() {
	RegisterTool("get_config_snapshot", "Get all registered config values in one view. Automatically masks sensitive keys (password, secret, token, api_key, private_key, credential). Returns key-value pairs.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		configRegistryMu.RLock()
		defer configRegistryMu.RUnlock()

		if len(registeredConfigs) == 0 {
			return map[string]any{
				"message": "No configs registered. Call debugagent.RegisterConfig(name, config) to enable config inspection.",
				"count":   0,
			}, nil
		}

		configs := make([]map[string]any, 0, len(registeredConfigs))
		maskedCount := 0

		for name, cfg := range registeredConfigs {
			entry := map[string]any{"name": name}
			fields, masked := flattenConfig(cfg)
			entry["values"] = fields
			entry["field_count"] = len(fields)
			entry["masked_fields"] = masked
			maskedCount += masked
			configs = append(configs, entry)
		}

		return map[string]any{
			"count":         len(registeredConfigs),
			"configs":       configs,
			"total_masked":  maskedCount,
		}, nil
	})

	RegisterTool("get_env_vars", "Get process environment variables with optional prefix filter. Masks sensitive values (password, secret, token, api_key, private_key, credential).", map[string]ToolParam{
		"prefix": {Type: "string", Description: "Optional prefix to filter environment variables (e.g. 'APP_', 'REDIS_')", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		prefix, _ := args["prefix"].(string)

		envVars := os.Environ()
		result := make(map[string]string, len(envVars))
		maskedCount := 0

		for _, env := range envVars {
			if prefix != "" && !strings.HasPrefix(env, prefix) {
				continue
			}
			idx := indexOfByte(env, '=')
			if idx < 0 {
				continue
			}
			key := env[:idx]
			value := env[idx+1:]
			if isSensitiveKey(key) {
				result[key] = "***"
				maskedCount++
			} else {
				result[key] = value
			}
		}

		return map[string]any{
			"count":        len(result),
			"masked_count": maskedCount,
			"env_vars":     result,
			"prefix":       prefix,
		}, nil
	})

	RegisterTool("get_config_diff", "Compare registered config values against their defaults (shows overrides). Returns fields where actual differs from expected.", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		configRegistryMu.RLock()
		defer configRegistryMu.RUnlock()

		if len(registeredConfigs) == 0 {
			return map[string]any{
				"message": "No configs registered. Call debugagent.RegisterConfig(name, config) to enable config inspection.",
				"count":   0,
			}, nil
		}

		diffs := make([]map[string]any, 0)
		overrideCount := 0

		for name, cfg := range registeredConfigs {
			fields, _ := flattenConfig(cfg)

			// Check each field for env override
			for key, value := range fields {
				// Derive env var name: CONFIGNAME_FIELDNAME
				envKey := strings.ToUpper(name + "_" + key)
				envKey = strings.ReplaceAll(envKey, " ", "_")
				envKey = strings.ReplaceAll(envKey, "-", "_")

				envVal := getenv(envKey)
				if envVal != "" {
					isMasked := isSensitiveKey(key)
					displayActual := value
					if isMasked {
						displayActual = "***"
					}
					diffs = append(diffs, map[string]any{
						"config":        name,
						"field":         key,
						"default":       displayActual,
						"env_override":  isMasked,
						"env_key":       envKey,
					})
					overrideCount++
				}
			}
		}

		return map[string]any{
			"count":           len(diffs),
			"override_count":  overrideCount,
			"diffs":           diffs,
		}, nil
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// sensitiveKeyWords are substrings that indicate a sensitive config key.
var sensitiveKeyWords = []string{"password", "secret", "token", "api_key", "private_key", "credential"}

// isSensitiveKey returns true if the key (lowercased) contains any sensitive substring.
func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveKeyWords {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

// flattenConfig uses reflection to extract all fields from a config struct/map
// into a flat key-value map, masking sensitive values.
func flattenConfig(cfg any) (map[string]string, int) {
	result := map[string]string{}
	maskedCount := 0

	defer func() {
		if r := recover(); r != nil {
			// On any reflection error, return what we have
		}
	}()

	if cfg == nil {
		return result, 0
	}

	rv := reflect.ValueOf(cfg)
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return result, 0
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Struct:
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			field := rt.Field(i)
			fieldValue := rv.Field(i)

			// Skip unexported fields
			if !fieldValue.CanInterface() {
				continue
		}

			// Use json tag if available, otherwise field name
			name := field.Name
			jsonTag := field.Tag.Get("json")
			if jsonTag != "" {
				if commaIdx := strings.Index(jsonTag, ","); commaIdx >= 0 {
					name = jsonTag[:commaIdx]
				} else {
					name = jsonTag
				}
				if name == "-" {
					continue // skip fields explicitly excluded from JSON
				}
			}

			strVal := stringifyValue(fieldValue)
			if isSensitiveKey(name) {
				result[name] = "***"
				maskedCount++
			} else {
				result[name] = strVal
			}
		}

	case reflect.Map:
		for _, key := range rv.MapKeys() {
			name := fmtSprintf("%v", key.Interface())
			strVal := stringifyValue(rv.MapIndex(key))
			if isSensitiveKey(name) {
				result[name] = "***"
				maskedCount++
			} else {
				result[name] = strVal
			}
		}

	default:
		// For non-struct/map types, just stringify
		result["value"] = stringifyValue(rv)
	}

	return result, maskedCount
}

// stringifyValue converts a reflect.Value to a string representation.
func stringifyValue(rv reflect.Value) string {
	defer func() {
		if r := recover(); r != nil {
		}
	}()

	if !rv.IsValid() {
		return ""
	}

	switch rv.Kind() {
	case reflect.String:
		return rv.String()
	case reflect.Bool:
		if rv.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmtSprintf("%d", rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmtSprintf("%d", rv.Uint())
	case reflect.Float32, reflect.Float64:
		return fmtSprintf("%v", rv.Float())
	case reflect.Ptr:
		if rv.IsNil() {
			return "<nil>"
		}
		return stringifyValue(rv.Elem())
	case reflect.Slice, reflect.Array:
		return fmtSprintf("%v", rv.Interface())
	case reflect.Map:
		return fmtSprintf("%v", rv.Interface())
	case reflect.Interface:
		return fmtSprintf("%v", rv.Interface())
	default:
		return fmtSprintf("%v", rv.Interface())
	}
}

// init ensures the sensitiveKeyWords are sorted for deterministic behavior.
func init() {
	sort.Strings(sensitiveKeyWords)
}
