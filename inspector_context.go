package debugagent

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// ─── Context registry ──────────────────────────────────────────────────────────

var (
	contextRegistry = map[string]context.Context{}
	contextMu       sync.RWMutex
)

// RegisterContext registers a context.Context for inspection. Call this to
// enable context tree inspection (deadlines, cancellation, key-value pairs).
func RegisterContext(name string, ctx context.Context) {
	contextMu.Lock()
	defer contextMu.Unlock()
	contextRegistry[name] = ctx
}

func registerContextInspector() {
	RegisterTool("get_context_tree", "Inspect registered context.Context trees: deadlines, cancellation status, key-value pairs", map[string]ToolParam{
		"name": {Type: "string", Description: "Filter to a specific context name (optional)", Required: false},
	}, func(args map[string]any) (any, error) {
		contextMu.RLock()
		defer contextMu.RUnlock()

		if len(contextRegistry) == 0 {
			return map[string]any{
				"message": "No contexts registered. Call RegisterContext(name, ctx) to enable context inspection.",
				"count":   0,
			}, nil
		}

		filterName, _ := args["name"].(string)
		entries := make([]map[string]any, 0, len(contextRegistry))
		for name, ctx := range contextRegistry {
			if filterName != "" && name != filterName {
				continue
			}
			entries = append(entries, inspectContext(name, ctx))
		}

		return map[string]any{
			"count":    len(entries),
			"contexts": entries,
		}, nil
	})
}

func inspectContext(name string, ctx context.Context) map[string]any {
	entry := map[string]any{"name": name}
	if ctx == nil {
		entry["error"] = "nil context"
		return entry
	}

	// Deadline
	if deadline, ok := ctx.Deadline(); ok {
		entry["deadline"] = deadline.Format(time.RFC3339)
		entry["time_until_deadline"] = time.Until(deadline).String()
	} else {
		entry["deadline"] = nil
	}

	// Cancellation status
	if err := ctx.Err(); err != nil {
		entry["error"] = err.Error()
		entry["canceled"] = true
	} else {
		entry["canceled"] = false
	}

	// Values via reflection
	values := extractContextValues(ctx)
	if len(values) > 0 {
		entry["values"] = values
	}

	return entry
}

// extractContextValues walks the context chain using reflection and extracts
// all key-value pairs from valueCtx nodes.
func extractContextValues(ctx context.Context) []map[string]any {
	var values []map[string]any
	visited := map[uintptr]bool{}
	walkContext(ctx, &values, visited, 0)
	return values
}

func walkContext(c context.Context, values *[]map[string]any, visited map[uintptr]bool, depth int) {
	if c == nil || depth > 100 {
		return
	}

	// Use a deferred recover since internal struct layouts may vary.
	defer func() {
		recover()
	}()

	rv := reflect.ValueOf(c)
	if rv.Kind() != reflect.Ptr {
		return
	}

	ptr := rv.Pointer()
	if visited[ptr] {
		return
	}
	visited[ptr] = true

	rv = rv.Elem()
	if !rv.IsValid() || rv.Kind() != reflect.Struct {
		return
	}

	typeName := rv.Type().String()

	// Extract key/val from valueCtx
	if typeName == "context.valueCtx" && rv.NumField() >= 3 {
		key := readUnexportedField(rv, 1)
		val := readUnexportedField(rv, 2)
		vEntry := map[string]any{
			"key":   fmt.Sprintf("%v", key),
			"value": fmt.Sprintf("%v", val),
		}
		if t := reflect.TypeOf(key); t != nil {
			vEntry["key_type"] = t.String()
		}
		*values = append(*values, vEntry)
	}

	// Walk all fields looking for parent context
	scanForContextParent(rv, values, visited, depth)
}

func scanForContextParent(rv reflect.Value, values *[]map[string]any, visited map[uintptr]bool, depth int) {
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		if !f.IsValid() {
			continue
		}

		// Check if this field is a context.Context (interface field)
		val := readUnexportedField(rv, i)
		if parentCtx, ok := val.(context.Context); ok && parentCtx != nil {
			walkContext(parentCtx, values, visited, depth+1)
			continue
		}

		// For embedded structs (like cancelCtx embedded in timerCtx), recurse
		if f.Kind() == reflect.Struct {
			scanForContextParent(f, values, visited, depth)
		}
	}
}
