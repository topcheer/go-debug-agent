package debugagent

import (
	"sort"
	"sync"
)

// ─── Feature flag types ──────────────────────────────────────────────────────

// FeatureFlag represents a registered feature flag with its current state.
type FeatureFlag struct {
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	Variant   string `json:"variant"`
	Reason   string `json:"reason"`
}

var (
	featureFlags     = map[string]FeatureFlag{}
	featureFlagMu   sync.RWMutex
)

// RegisterFeatureFlag registers a feature flag for inspection.
func RegisterFeatureFlag(name string, flag FeatureFlag) {
	featureFlagMu.Lock()
	defer featureFlagMu.Unlock()
	flag.Name = name
	featureFlags[name] = flag
}

func registerFeatureFlagInspector() {
	RegisterTool("get_feature_flags", "List all registered feature flags with their current state (on/off, variant, reason).", nil, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		featureFlagMu.RLock()
		defer featureFlagMu.RUnlock()

		if len(featureFlags) == 0 {
			return map[string]any{
				"message": "No feature flags registered. Call debugagent.RegisterFeatureFlag(name, flag) to enable feature flag inspection.",
				"count":   0,
			}, nil
		}

		flags := make([]map[string]any, 0, len(featureFlags))
		enabledCount := 0
		disabledCount := 0

		// Sort by name for deterministic output
		names := make([]string, 0, len(featureFlags))
		for name := range featureFlags {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			ff := featureFlags[name]
			entry := map[string]any{
				"name":    ff.Name,
				"enabled": ff.Enabled,
			}
			if ff.Variant != "" {
				entry["variant"] = ff.Variant
			}
			if ff.Reason != "" {
				entry["reason"] = ff.Reason
			}
			entry["state"] = "off"
			if ff.Enabled {
				entry["state"] = "on"
				enabledCount++
			} else {
				disabledCount++
			}
			flags = append(flags, entry)
		}

		return map[string]any{
			"count":           len(flags),
			"enabled_count":   enabledCount,
			"disabled_count": disabledCount,
			"feature_flags":   flags,
		}, nil
	})

	RegisterTool("evaluate_flag", "Evaluate a specific feature flag for a specific context/user. Returns whether the flag is enabled and the reason.", map[string]ToolParam{
		"flag_name":    {Type: "string", Description: "Name of the feature flag to evaluate", Required: true},
		"user_context": {Type: "string", Description: "Optional user/context identifier for targeted evaluation", Required: false},
	}, func(args map[string]any) (any, error) {
		defer func() {
			if r := recover(); r != nil {
			}
		}()

		flagName, _ := args["flag_name"].(string)
		if flagName == "" {
			return nil, fmtErrorf("flag_name is required")
		}

		userContext, _ := args["user_context"].(string)

		featureFlagMu.RLock()
		defer featureFlagMu.RUnlock()

		ff, ok := featureFlags[flagName]
		if !ok {
			// List available flags
			available := make([]string, 0, len(featureFlags))
			for name := range featureFlags {
				available = append(available, name)
			}
			sort.Strings(available)
			return map[string]any{
				"error":     "Feature flag not found: " + flagName,
				"available": available,
			}, nil
		}

		result := map[string]any{
			"flag_name":  ff.Name,
			"enabled":    ff.Enabled,
			"variant":    ff.Variant,
			"state":      "off",
		}
		if ff.Enabled {
			result["state"] = "on"
		}

		// Determine reason
		reason := ff.Reason
		if reason == "" {
			if ff.Enabled {
				reason = "flag_is_enabled"
			} else {
				reason = "flag_is_disabled"
			}
		}
		result["reason"] = reason

		if userContext != "" {
			result["user_context"] = userContext
			result["evaluated_for"] = userContext
		} else {
			result["user_context"] = "(default)"
			result["evaluated_for"] = "global"
		}

		return result, nil
	})
}
