package debugagent

import (
	"reflect"
	"sync"
)

// ─── Security registry ──────────────────────────────────────────────────────

var (
	registeredAuthConfigs   = map[string]any{}
	registeredSessionStores = map[string]any{}
	securityMu              sync.RWMutex
)

// RegisterAuthConfig registers an auth middleware configuration for inspection.
// The config can be any struct or map describing JWT secrets, OAuth providers,
// session settings, etc.
func RegisterAuthConfig(name string, config any) {
	securityMu.Lock()
	defer securityMu.Unlock()
	registeredAuthConfigs[name] = config
}

// RegisterSessionStore registers a session store for active-session inspection.
// The store can be any type that supports listing active sessions via reflection.
func RegisterSessionStore(name string, store any) {
	securityMu.Lock()
	defer securityMu.Unlock()
	registeredSessionStores[name] = store
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerSecurityInspector() {
	RegisterTool("get_auth_config", "List all registered auth middleware configs (JWT secret presence, OAuth providers, session settings, API key headers)", nil, func(args map[string]any) (any, error) {
		securityMu.RLock()
		defer securityMu.RUnlock()

		if len(registeredAuthConfigs) == 0 {
			return map[string]any{
				"message": "No auth configs registered. Call debugagent.RegisterAuthConfig(name, config) to enable security inspection.",
				"count":   0,
			}, nil
		}

		configs := make([]map[string]any, 0, len(registeredAuthConfigs))
		for name, cfg := range registeredAuthConfigs {
			entry := map[string]any{"name": name}
			extractAuthFields(cfg, entry)
			configs = append(configs, entry)
		}

		return map[string]any{
			"count":   len(registeredAuthConfigs),
			"configs": configs,
		}, nil
	})

	RegisterTool("get_active_sessions", "List active user sessions from registered session stores (session ID, user ID, creation time, last access, IP)", map[string]ToolParam{
		"store": {Type: "string", Description: "Specific session store name (default: all registered stores)", Required: false},
	}, func(args map[string]any) (any, error) {
		securityMu.RLock()
		defer securityMu.RUnlock()

		if len(registeredSessionStores) == 0 {
			return map[string]any{
				"message": "No session stores registered. Call debugagent.RegisterSessionStore(name, store) to enable session inspection.",
				"count":   0,
			}, nil
		}

		targetStore, _ := args["store"].(string)
		stores := make([]map[string]any, 0, len(registeredSessionStores))
		for name, store := range registeredSessionStores {
			if targetStore != "" && targetStore != name {
				continue
			}
			entry := map[string]any{"name": name}
			extractSessionInfo(store, entry)
			stores = append(stores, entry)
		}

		return map[string]any{
			"count":   len(stores),
			"stores":  stores,
		}, nil
	})

	RegisterTool("get_password_policy", "Show configured password requirements (min length, complexity rules, expiry) from registered auth configs", nil, func(args map[string]any) (any, error) {
		securityMu.RLock()
		defer securityMu.RUnlock()

		if len(registeredAuthConfigs) == 0 {
			return map[string]any{
				"message": "No auth configs registered. Register a config with password policy fields via RegisterAuthConfig().",
				"count":   0,
			}, nil
		}

		policies := make([]map[string]any, 0, len(registeredAuthConfigs))
		for name, cfg := range registeredAuthConfigs {
			entry := extractPasswordPolicy(cfg)
			entry["auth_config"] = name
			policies = append(policies, entry)
		}

		return map[string]any{
			"count":    len(policies),
			"policies": policies,
		}, nil
	})
}

// extractAuthFields pulls common auth-related fields from a config via reflection.
func extractAuthFields(cfg any, entry map[string]any) {
	defer func() { recover() }()

	// Handle map configs directly
	if m, ok := cfg.(map[string]any); ok {
		for k, v := range m {
			entry[k] = v
		}
		return
	}

	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		// Store as-is
		entry["config"] = cfg
		return
	}

	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)
		if !fieldVal.CanInterface() {
			continue
		}
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			jsonTag = field.Name
		}
		// Check if it's a secret field (mask it)
		if isSecretField(field.Name) {
			entry[jsonTag] = maskSecret(fieldVal)
		} else {
			entry[jsonTag] = fieldVal.Interface()
		}
	}
}

// extractSessionInfo attempts to list sessions from a store via reflection.
func extractSessionInfo(store any, entry map[string]any) {
	defer func() { recover() }()

	// Try calling a common method like ListSessions / GetAll / Sessions
	v := reflect.ValueOf(store)
	for _, methodName := range []string{"ListSessions", "GetAll", "Sessions", "Active"} {
		method := v.MethodByName(methodName)
		if method.IsValid() {
			results := method.Call(nil)
			if len(results) > 0 && results[0].CanInterface() {
				entry["sessions"] = results[0].Interface()
			}
			return
		}
	}

	// Fall back to treating store as a map of sessions
	if v.Kind() == reflect.Map {
		sessions := make([]map[string]any, 0)
		for _, key := range v.MapKeys() {
			val := v.MapIndex(key)
			if val.CanInterface() {
				sessEntry := map[string]any{"session_id": key.Interface()}
				extractAuthFields(val.Interface(), sessEntry)
				sessions = append(sessions, sessEntry)
			}
		}
		entry["sessions"] = sessions
		entry["active_count"] = len(sessions)
		return
	}

	// If it's a struct with a Count or similar field
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		extractAuthFields(store, entry)
	}
}

// extractPasswordPolicy pulls password-policy related fields from a config.
func extractPasswordPolicy(cfg any) map[string]any {
	result := map[string]any{}
	defer func() { recover() }()

	if m, ok := cfg.(map[string]any); ok {
		for _, key := range []string{"min_length", "min_password_length", "require_uppercase", "require_lowercase", "require_digit", "require_symbol", "expiry_days", "password_expiry"} {
			if v, exists := m[key]; exists {
				result[key] = v
			}
		}
		// Include any nested password_policy map
		if pp, ok := m["password_policy"]; ok {
			if pm, ok := pp.(map[string]any); ok {
				for k, v := range pm {
					result[k] = v
				}
			}
		}
		return result
	}

	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return result
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return result
	}

	// Check for a nested PasswordPolicy struct field
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldVal := v.Field(i)
		if !fieldVal.CanInterface() {
			continue
		}
		nameLower := toLower(field.Name)
		if containsAny(nameLower, "password", "minlength", "min_length", "expiry", "complexity") {
			jsonTag := field.Tag.Get("json")
			if jsonTag == "" || jsonTag == "-" {
				jsonTag = toLowerSnake(field.Name)
			}
			result[jsonTag] = fieldVal.Interface()
		}
		// Recurse into nested password policy struct
		if fieldVal.Kind() == reflect.Struct || (fieldVal.Kind() == reflect.Ptr && !fieldVal.IsNil() && fieldVal.Elem().Kind() == reflect.Struct) {
			fv := fieldVal
			if fv.Kind() == reflect.Ptr {
				fv = fv.Elem()
			}
			ft := fv.Type()
			if containsAny(toLower(field.Name), "password", "policy") {
				for j := 0; j < ft.NumField(); j++ {
					nestedField := ft.Field(j)
					nestedVal := fv.Field(j)
					if nestedVal.CanInterface() {
						jsonTag := nestedField.Tag.Get("json")
						if jsonTag == "" || jsonTag == "-" {
							jsonTag = toLowerSnake(nestedField.Name)
						}
						result[jsonTag] = nestedVal.Interface()
					}
				}
			}
		}
	}
	return result
}

func isSecretField(name string) bool {
	lower := toLower(name)
	return containsAny(lower, "secret", "password", "token", "apikey", "api_key", "privatekey")
}

func maskSecret(v reflect.Value) string {
	if !v.CanInterface() {
		return "[redacted]"
	}
	s := ""
	switch v.Kind() {
	case reflect.String:
		s = v.String()
	default:
		s = "[redacted]"
	}
	if len(s) == 0 {
		return "[empty]"
	}
	if len(s) <= 4 {
		return "****"
	}
	return s[:2] + "****" + s[len(s)-2:]
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			result[i] = c + 32
		} else {
			result[i] = c
		}
	}
	return string(result)
}

func toLowerSnake(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if i > 0 {
				result = append(result, '_')
			}
			result = append(result, c+32)
		} else {
			result = append(result, c)
		}
	}
	return string(result)
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
