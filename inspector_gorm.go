package debugagent

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// ─── GORM registries ────────────────────────────────────────────────────────

type gormRegistration struct {
	db     any   // *gorm.DB (stored as any to avoid import dependency)
	models []any // models registered via AutoMigrate
}

var (
	gormRegistry   = map[string]*gormRegistration{}
	gormRegistryMu sync.RWMutex
)

// RegisterGormDB registers a *gorm.DB instance for inspection.
// Call this after opening and configuring the GORM database.
func RegisterGormDB(name string, db any) {
	gormRegistryMu.Lock()
	defer gormRegistryMu.Unlock()
	if reg, ok := gormRegistry[name]; ok {
		reg.db = db
	} else {
		gormRegistry[name] = &gormRegistration{db: db}
	}
}

// RegisterGormModels registers model structs (typically those passed to AutoMigrate)
// so they can be listed by the inspector.
func RegisterGormModels(name string, models ...any) {
	gormRegistryMu.Lock()
	defer gormRegistryMu.Unlock()
	if reg, ok := gormRegistry[name]; ok {
		reg.models = append(reg.models, models...)
	} else {
		gormRegistry[name] = &gormRegistration{models: models}
	}
}

// ─── Reflection helpers ─────────────────────────────────────────────────────

// gormConnectionStats calls db.DB() via reflection to get *sql.DB and returns its stats.
func gormConnectionStats(db any) (*sql.DBStats, error) {
	v := reflect.ValueOf(db)
	method := v.MethodByName("DB")
	if !method.IsValid() {
		return nil, fmt.Errorf("DB() method not found — object is not a *gorm.DB")
	}

	results := method.Call(nil)
	if len(results) < 1 {
		return nil, fmt.Errorf("DB() returned no results")
	}

	// Check for error (second return value)
	if len(results) >= 2 && results[1].CanInterface() {
		if err, ok := results[1].Interface().(error); ok && err != nil {
			return nil, err
		}
	}

	sqlDB, ok := results[0].Interface().(*sql.DB)
	if !ok {
		return nil, fmt.Errorf("DB() did not return *sql.DB")
	}

	stats := sqlDB.Stats()
	return &stats, nil
}

// gormDBConfigViaReflect tries to extract DB name/driver from gorm.Config.
func gormDBConfigViaReflect(db any) map[string]any {
	result := map[string]any{}

	v := reflect.ValueOf(db)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	configField := v.FieldByName("Config")
	if !configField.IsValid() || configField.Kind() != reflect.Ptr {
		return result
	}
	config := configField.Elem()

	// Try to get the Dialector name
	if dialField := config.FieldByName("Dialector"); dialField.IsValid() && !dialField.IsNil() {
		dial := dialField.Elem()
		if nameField := dial.FieldByName("Name"); nameField.IsValid() {
			result["dialector"] = nameField.String()
		}
	}

	return result
}

// modelInfo extracts type info from a registered model struct.
func modelInfo(model any) map[string]any {
	t := reflect.TypeOf(model)
	if t == nil {
		return map[string]any{"error": "nil model"}
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	result := map[string]any{
		"struct_type": t.Name(),
		"table_name":  toSnakeCase(t.Name()),
		"package":     t.PkgPath(),
	}

	// Extract fields with gorm tags
	fields := make([]map[string]any, 0)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		entry := map[string]any{
			"name": field.Name,
			"type": field.Type.String(),
		}
		tag := field.Tag.Get("gorm")
		if tag != "" {
			entry["gorm_tag"] = tag
		}
		jsonTag := field.Tag.Get("json")
		if jsonTag != "" {
			entry["json_tag"] = jsonTag
		}
		fields = append(fields, entry)
	}
	result["fields"] = fields

	return result
}

// toSnakeCase converts CamelCase to snake_case (simplified GORM naming).
func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		if r >= 'A' && r <= 'Z' {
			result.WriteRune(r + 32)
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerGormInspector() {
	RegisterTool("get_gorm_stats", "List registered GORM databases with connection pool stats (open, idle, in-use, max connections, wait count)", nil, func(args map[string]any) (any, error) {
		gormRegistryMu.RLock()
		defer gormRegistryMu.RUnlock()

		if len(gormRegistry) == 0 {
			return map[string]any{
				"message": "No GORM databases registered. Call debugagent.RegisterGormDB(name, db) to enable GORM inspection.",
				"count":   0,
			}, nil
		}

		dbs := make([]map[string]any, 0, len(gormRegistry))
		for name, reg := range gormRegistry {
			entry := map[string]any{"name": name}

			if reg.db == nil {
				entry["error"] = "no *gorm.DB registered for this name"
				dbs = append(dbs, entry)
				continue
			}

			// Get config info
			for k, v := range gormDBConfigViaReflect(reg.db) {
				entry[k] = v
			}

			// Get connection stats
			stats, err := gormConnectionStats(reg.db)
			if err != nil {
				entry["error"] = err.Error()
			} else {
				entry["max_open_connections"] = stats.MaxOpenConnections
				entry["open_connections"] = stats.OpenConnections
				entry["in_use"] = stats.InUse
				entry["idle"] = stats.Idle
				entry["wait_count"] = stats.WaitCount
				entry["wait_duration"] = stats.WaitDuration.String()
				entry["max_idle_closed"] = stats.MaxIdleClosed
				entry["max_lifetime_closed"] = stats.MaxLifetimeClosed
			}

			entry["model_count"] = len(reg.models)
			dbs = append(dbs, entry)
		}

		return map[string]any{
			"count": len(gormRegistry),
			"dbs":   dbs,
		}, nil
	})

	RegisterTool("get_gorm_models", "List models registered for each GORM database (table names, struct types, fields with gorm tags)", map[string]ToolParam{
		"name": {Type: "string", Description: "Specific DB name (default: all registered databases)", Required: false},
	}, func(args map[string]any) (any, error) {
		gormRegistryMu.RLock()
		defer gormRegistryMu.RUnlock()

		if len(gormRegistry) == 0 {
			return map[string]any{
				"message": "No GORM databases registered.",
				"count":   0,
			}, nil
		}

		targetName, _ := args["name"].(string)
		dbs := make([]map[string]any, 0, len(gormRegistry))
		for name, reg := range gormRegistry {
			if targetName != "" && targetName != name {
				continue
			}
			entry := map[string]any{
				"name":        name,
				"model_count": len(reg.models),
			}

			if len(reg.models) == 0 {
				entry["message"] = "No models registered. Call debugagent.RegisterGormModels(name, &Model{}) after AutoMigrate."
			} else {
				models := make([]map[string]any, 0, len(reg.models))
				for _, m := range reg.models {
					models = append(models, modelInfo(m))
				}
				entry["models"] = models
			}

			dbs = append(dbs, entry)
		}

		return map[string]any{
			"count": len(dbs),
			"dbs":   dbs,
		}, nil
	})
}
