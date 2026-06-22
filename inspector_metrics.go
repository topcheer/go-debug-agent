package debugagent

import (
	"fmt"
	"reflect"
	"sync"
)

// ─── Metrics registry ──────────────────────────────────────────────────────────

var (
	metricsGatherers   = map[string]any{}
	metricsGathererMu  sync.RWMutex
)

// RegisterMetricsGatherer registers a Prometheus-compatible gatherer (anything
// with a Gather() method returning metric families, e.g. prometheus.Gatherer).
func RegisterMetricsGatherer(name string, gatherer any) {
	metricsGathererMu.Lock()
	defer metricsGathererMu.Unlock()
	metricsGatherers[name] = gatherer
}

func registerMetricsInspector() {
	RegisterTool("get_registered_metrics", "List all registered Prometheus metrics (name, type, help text, sample count). Use RegisterMetricsGatherer to register a prometheus.Gatherer.", map[string]ToolParam{
		"source": {Type: "string", Description: "Filter by gatherer name (optional)", Required: false},
	}, func(args map[string]any) (any, error) {
		metricsGathererMu.RLock()
		defer metricsGathererMu.RUnlock()

		if len(metricsGatherers) == 0 {
			return map[string]any{
				"message": "No metrics gatherers registered. Call RegisterMetricsGatherer(name, gatherer) to enable metrics inspection.",
				"metrics": []any{},
				"count":  0,
			}, nil
		}

		filterSource, _ := args["source"].(string)
		allMetrics := make([]map[string]any, 0)

		for source, g := range metricsGatherers {
			if filterSource != "" && source != filterSource {
				continue
			}
			families := callGather(g)
			for _, fam := range families {
				m := extractMetricFamily(fam)
				m["source"] = source
				allMetrics = append(allMetrics, m)
			}
		}

		return map[string]any{
			"metrics": allMetrics,
			"count":   len(allMetrics),
		}, nil
	})

	RegisterTool("get_metric_value", "Get current value of a specific metric by name", map[string]ToolParam{
		"metric_name": {Type: "string", Description: "Name of the metric to retrieve", Required: true},
	}, func(args map[string]any) (any, error) {
		metricName, _ := args["metric_name"].(string)
		if metricName == "" {
			return nil, fmt.Errorf("metric_name is required")
		}

		metricsGathererMu.RLock()
		defer metricsGathererMu.RUnlock()

		var found []map[string]any
		for source, g := range metricsGatherers {
			families := callGather(g)
			for _, fam := range families {
				m := extractMetricFamily(fam)
				name, _ := m["name"].(string)
				if name == metricName {
					m["source"] = source
					m["samples"] = extractMetricSamples(fam)
					found = append(found, m)
				}
			}
		}

		if len(found) == 0 {
			return map[string]any{
				"metric_name": metricName,
				"found":       false,
				"message":     "Metric not found. Make sure a gatherer is registered with RegisterMetricsGatherer.",
			}, nil
		}

		return map[string]any{
			"metric_name": metricName,
			"found":       true,
			"matches":     found,
		}, nil
	})
}

// callGather calls the Gather() method on a gatherer via reflection and returns
// a slice of reflect.Value, each pointing to a MetricFamily struct.
func callGather(gatherer any) []reflect.Value {
	defer func() {
		recover()
	}()

	rv := reflect.ValueOf(gatherer)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil
	}

	method := rv.MethodByName("Gather")
	if !method.IsValid() {
		return nil
	}

	results := method.Call(nil)
	if len(results) < 1 || results[0].Kind() != reflect.Slice {
		return nil
	}

	slice := results[0]
	families := make([]reflect.Value, 0, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		families = append(families, slice.Index(i))
	}
	return families
}

// extractMetricFamily reads a MetricFamily struct via reflection.
func extractMetricFamily(famVal reflect.Value) map[string]any {
	result := map[string]any{}

	if famVal.Kind() == reflect.Ptr {
		if famVal.IsNil() {
			return result
		}
		famVal = famVal.Elem()
	}

	if famVal.Kind() != reflect.Struct {
		return result
	}

	// Name: *string
	nameField := famVal.FieldByName("Name")
	if nameField.IsValid() {
		result["name"] = derefReflectString(nameField)
	}

	// Help: *string
	helpField := famVal.FieldByName("Help")
	if helpField.IsValid() {
		result["help"] = derefReflectString(helpField)
	}

	// Type: *MetricType (int32 enum)
	typeField := famVal.FieldByName("Type")
	if typeField.IsValid() {
		result["type"] = metricTypeString(derefReflectInt(typeField))
	}

	// Metric: []*Metric
	metricField := famVal.FieldByName("Metric")
	if metricField.IsValid() && metricField.Kind() == reflect.Slice {
		result["sample_count"] = metricField.Len()
	}

	return result
}

// extractMetricSamples reads the Metric slice from a MetricFamily and returns
// label values plus the metric value.
func extractMetricSamples(famVal reflect.Value) []map[string]any {
	if famVal.Kind() == reflect.Ptr {
		if famVal.IsNil() {
			return nil
		}
		famVal = famVal.Elem()
	}

	metricField := famVal.FieldByName("Metric")
	if !metricField.IsValid() || metricField.Kind() != reflect.Slice {
		return nil
	}

	samples := make([]map[string]any, 0, metricField.Len())
	for i := 0; i < metricField.Len(); i++ {
		metricVal := metricField.Index(i)
		if metricVal.Kind() == reflect.Ptr {
			if metricVal.IsNil() {
				continue
			}
			metricVal = metricVal.Elem()
		}
		if metricVal.Kind() != reflect.Struct {
			continue
		}

		sample := map[string]any{}

		// Extract labels
		labelField := metricVal.FieldByName("Label")
		if labelField.IsValid() && labelField.Kind() == reflect.Slice {
			labels := map[string]string{}
			for j := 0; j < labelField.Len(); j++ {
				lp := labelField.Index(j)
				if lp.Kind() == reflect.Ptr {
					if lp.IsNil() {
						continue
					}
					lp = lp.Elem()
				}
				name := derefReflectString(lp.FieldByName("Name"))
				value := derefReflectString(lp.FieldByName("Value"))
				if name != "" {
					labels[name] = value
				}
			}
			if len(labels) > 0 {
				sample["labels"] = labels
			}
		}

		// Extract value based on type (Counter, Gauge, Untyped → Value *float64)
		for _, fieldName := range []string{"Counter", "Gauge", "Untyped"} {
			f := metricVal.FieldByName(fieldName)
			if f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
				val := f.Elem().FieldByName("Value")
				if val.IsValid() {
					valElem := val
					if valElem.Kind() == reflect.Ptr && !valElem.IsNil() {
						valElem = valElem.Elem()
					}
					if valElem.Kind() == reflect.Float64 {
						sample["value"] = valElem.Float()
					}
				}
			}
		}

		// Histogram: Sum, Count
		histField := metricVal.FieldByName("Histogram")
		if histField.IsValid() && histField.Kind() == reflect.Ptr && !histField.IsNil() {
			hist := histField.Elem()
			if sumVal := hist.FieldByName("SampleSum"); sumVal.IsValid() {
				sample["histogram_sum"] = sumVal.Float()
			}
			if countVal := hist.FieldByName("SampleCount"); countVal.IsValid() {
				sample["histogram_count"] = countVal.Uint()
			}
		}

		samples = append(samples, sample)
		if len(samples) >= 50 {
			break
		}
	}

	return samples
}

// ─── Reflection helpers (shared) ───────────────────────────────────────────────

func derefReflectString(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	return v.String()
}

func derefReflectInt(v reflect.Value) int64 {
	if !v.IsValid() {
		return 0
	}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return 0
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(v.Uint())
	}
	return 0
}

func metricTypeString(typeVal int64) string {
	switch typeVal {
	case 0:
		return "counter"
	case 1:
		return "gauge"
	case 2:
		return "summary"
	case 3:
		return "histogram"
	case 4:
		return "untyped"
	default:
		return "unknown"
	}
}

func reflectTypeName(v any) string {
	if v == nil {
		return "nil"
	}
	t := reflect.TypeOf(v)
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.String()
}

func reflectCallStringMethod(v any, methodName string) string {
	defer func() {
		recover()
	}()
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return ""
	}
	method := rv.MethodByName(methodName)
	if !method.IsValid() {
		return ""
	}
	results := method.Call(nil)
	if len(results) > 0 {
		return fmt.Sprintf("%v", results[0].Interface())
	}
	return ""
}

func reflectCallIntMethod(v any, methodName string) int {
	defer func() {
		recover()
	}()
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return -1
	}
	method := rv.MethodByName(methodName)
	if !method.IsValid() {
		return -1
	}
	results := method.Call(nil)
	if len(results) > 0 {
		switch results[0].Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int(results[0].Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return int(results[0].Uint())
		}
	}
	return -1
}

func reflectCallStructMethod(v any, methodName string) map[string]any {
	defer func() {
		recover()
	}()
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return nil
	}
	method := rv.MethodByName(methodName)
	if !method.IsValid() {
		return nil
	}
	results := method.Call(nil)
	if len(results) == 0 {
		return nil
	}
	return structToMap(results[0])
}

func reflectCallSetLevel(v any, levelArg string) bool {
	defer func() {
		recover()
	}()
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Ptr && rv.IsNil() {
		return false
	}
	for _, methodName := range []string{"SetLevel", "SetLevelStr"} {
		method := rv.MethodByName(methodName)
		if !method.IsValid() {
			continue
		}
		methodType := method.Type()
		if methodType.NumIn() < 1 {
			continue
		}
		// Try calling with a string
		if methodType.In(0).Kind() == reflect.String {
			method.Call([]reflect.Value{reflect.ValueOf(levelArg)})
			return true
		}
		// Try calling with an int (common for slog/zap level enums)
		if methodType.In(0).Kind() >= reflect.Int && methodType.In(0).Kind() <= reflect.Int64 {
			levelMap := map[string]int64{"debug": 0, "info": 1, "warn": 2, "warning": 2, "error": 3}
			if n, ok := levelMap[levelArg]; ok {
				method.Call([]reflect.Value{reflect.ValueOf(n).Convert(methodType.In(0))})
				return true
			}
		}
	}
	return false
}

func structToMap(v reflect.Value) map[string]any {
	result := map[string]any{}
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return result
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return result
	}
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fv := v.Field(i)
		if fv.CanInterface() {
			result[field.Name] = fv.Interface()
		}
	}
	return result
}
