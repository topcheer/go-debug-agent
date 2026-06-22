package debugagent

import (
	"fmt"
	"reflect"
	"sync"
)

// ─── Web framework registries ───────────────────────────────────────────────

var (
	registeredGinEngines = map[string]any{}
	registeredEchoApps   = map[string]any{}
	registeredChiRouters = map[string]any{}
	routesRegistryMu     sync.RWMutex
)

// RegisterGinEngine registers a gin.Engine for route inspection.
func RegisterGinEngine(engine any) {
	routesRegistryMu.Lock()
	defer routesRegistryMu.Unlock()
	registeredGinEngines["default"] = engine
}

// RegisterEchoApp registers an echo.Echo for route inspection.
func RegisterEchoApp(e any) {
	routesRegistryMu.Lock()
	defer routesRegistryMu.Unlock()
	registeredEchoApps["default"] = e
}

// RegisterChiRouter registers a chi.Router for route inspection.
func RegisterChiRouter(r any) {
	routesRegistryMu.Lock()
	defer routesRegistryMu.Unlock()
	registeredChiRouters["default"] = r
}

// ─── Reflection helpers ─────────────────────────────────────────────────────

// ginRoutesViaReflect calls Routes() on a gin.Engine and extracts method/path/handler.
func ginRoutesViaReflect(engine any) ([]map[string]any, error) {
	v := reflect.ValueOf(engine)
	method := v.MethodByName("Routes")
	if !method.IsValid() {
		return nil, fmt.Errorf("Routes() method not found — object is not a gin.Engine")
	}

	results := method.Call(nil)
	if len(results) == 0 {
		return nil, fmt.Errorf("Routes() returned no results")
	}

	routesSlice := results[0]
	if routesSlice.Kind() != reflect.Slice {
		return nil, fmt.Errorf("Routes() did not return a slice")
	}

	routes := make([]map[string]any, 0, routesSlice.Len())
	for i := 0; i < routesSlice.Len(); i++ {
		item := routesSlice.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}
		entry := map[string]any{}
		if f := item.FieldByName("Method"); f.IsValid() {
			entry["method"] = f.String()
		}
		if f := item.FieldByName("Path"); f.IsValid() {
			entry["path"] = f.String()
		}
		if f := item.FieldByName("Handler"); f.IsValid() {
			entry["handler"] = f.String()
		}
		if len(entry) > 0 {
			routes = append(routes, entry)
		}
	}
	return routes, nil
}

// echoRoutesViaReflect calls Routes() on an echo.Echo and extracts method/path/name.
func echoRoutesViaReflect(e any) ([]map[string]any, error) {
	v := reflect.ValueOf(e)
	method := v.MethodByName("Routes")
	if !method.IsValid() {
		return nil, fmt.Errorf("Routes() method not found — object is not an echo.Echo")
	}

	results := method.Call(nil)
	if len(results) == 0 {
		return nil, fmt.Errorf("Routes() returned no results")
	}

	routesSlice := results[0]
	if routesSlice.Kind() != reflect.Slice {
		return nil, fmt.Errorf("Routes() did not return a slice")
	}

	routes := make([]map[string]any, 0, routesSlice.Len())
	for i := 0; i < routesSlice.Len(); i++ {
		item := routesSlice.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}
		entry := map[string]any{}
		if f := item.FieldByName("Method"); f.IsValid() {
			entry["method"] = f.String()
		}
		if f := item.FieldByName("Path"); f.IsValid() {
			entry["path"] = f.String()
		}
		if f := item.FieldByName("Name"); f.IsValid() {
			entry["name"] = f.String()
		}
		if len(entry) > 0 {
			routes = append(routes, entry)
		}
	}
	return routes, nil
}

// chiRoutesViaReflect attempts to call Routes() on a chi router.
func chiRoutesViaReflect(r any) ([]map[string]any, error) {
	v := reflect.ValueOf(r)
	method := v.MethodByName("Routes")
	if !method.IsValid() {
		return nil, fmt.Errorf("Routes() method not found — object may not be a chi.Router")
	}

	results := method.Call(nil)
	if len(results) == 0 {
		return nil, fmt.Errorf("Routes() returned no results")
	}

	routesSlice := results[0]
	if routesSlice.Kind() != reflect.Slice {
		return nil, fmt.Errorf("Routes() did not return a slice")
	}

	routes := make([]map[string]any, 0, routesSlice.Len())
	for i := 0; i < routesSlice.Len(); i++ {
		item := routesSlice.Index(i)
		if item.Kind() == reflect.Ptr {
			item = item.Elem()
		}
		entry := map[string]any{}
		// chi.Route has Pattern string and Handlers map[Method]http.Handler
		if f := item.FieldByName("Pattern"); f.IsValid() {
			entry["pattern"] = f.String()
		}
		// Try to extract method names from the Handlers map
		if handlers := item.FieldByName("Handlers"); handlers.IsValid() && handlers.Kind() == reflect.Map {
			var methods []string
			for _, key := range handlers.MapKeys() {
				methods = append(methods, key.String())
			}
			entry["methods"] = methods
		}
		if f := item.FieldByName("SubRoutes"); f.IsValid() && !f.IsNil() {
			entry["has_subroutes"] = true
		}
		if len(entry) > 0 {
			routes = append(routes, entry)
		}
	}
	return routes, nil
}

// ─── Inspector registration ─────────────────────────────────────────────────

func registerRoutesInspector() {
	RegisterTool("get_gin_routes", "List all registered Gin routes (method, path, handler name) from registered gin.Engine instances", nil, func(args map[string]any) (any, error) {
		routesRegistryMu.RLock()
		defer routesRegistryMu.RUnlock()

		if len(registeredGinEngines) == 0 {
			return map[string]any{
				"message": "No Gin engines registered. Call debugagent.RegisterGinEngine(engine) to enable route inspection.",
				"count":   0,
			}, nil
		}

		engines := make([]map[string]any, 0, len(registeredGinEngines))
		for name, engine := range registeredGinEngines {
			entry := map[string]any{"name": name}
			routes, err := ginRoutesViaReflect(engine)
			if err != nil {
				entry["error"] = err.Error()
			} else {
				entry["route_count"] = len(routes)
				entry["routes"] = routes
			}
			engines = append(engines, entry)
		}

		return map[string]any{
			"count":   len(registeredGinEngines),
			"engines": engines,
		}, nil
	})

	RegisterTool("get_echo_routes", "List all registered Echo routes (method, path, name) from registered echo.Echo instances", nil, func(args map[string]any) (any, error) {
		routesRegistryMu.RLock()
		defer routesRegistryMu.RUnlock()

		if len(registeredEchoApps) == 0 {
			return map[string]any{
				"message": "No Echo apps registered. Call debugagent.RegisterEchoApp(e) to enable route inspection.",
				"count":   0,
			}, nil
		}

		apps := make([]map[string]any, 0, len(registeredEchoApps))
		for name, app := range registeredEchoApps {
			entry := map[string]any{"name": name}
			routes, err := echoRoutesViaReflect(app)
			if err != nil {
				entry["error"] = err.Error()
			} else {
				entry["route_count"] = len(routes)
				entry["routes"] = routes
			}
			apps = append(apps, entry)
		}

		return map[string]any{
			"count": len(registeredEchoApps),
			"apps":  apps,
		}, nil
	})

	RegisterTool("get_chi_routes", "List all registered Chi routes (pattern, methods) from registered chi.Router instances", nil, func(args map[string]any) (any, error) {
		routesRegistryMu.RLock()
		defer routesRegistryMu.RUnlock()

		if len(registeredChiRouters) == 0 {
			return map[string]any{
				"message": "No Chi routers registered. Call debugagent.RegisterChiRouter(r) to enable route inspection.",
				"count":   0,
			}, nil
		}

		routers := make([]map[string]any, 0, len(registeredChiRouters))
		for name, router := range registeredChiRouters {
			entry := map[string]any{"name": name}
			routes, err := chiRoutesViaReflect(router)
			if err != nil {
				entry["error"] = err.Error()
			} else {
				entry["route_count"] = len(routes)
				entry["routes"] = routes
			}
			routers = append(routers, entry)
		}

		return map[string]any{
			"count":   len(registeredChiRouters),
			"routers": routers,
		}, nil
	})
}
