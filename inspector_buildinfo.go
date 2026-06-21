package debugagent

import (
	"runtime/debug"
)

func registerBuildInfoInspector() {
	RegisterTool("get_build_info", "Get Go build information: Go version, module path, build settings (from runtime/debug.ReadBuildInfo)", nil, func(args map[string]any) (any, error) {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return map[string]any{"error": "Build info not available (binary may not have been built with module support)"}, nil
		}

		settings := map[string]string{}
		for _, s := range info.Settings {
			settings[s.Key] = s.Value
		}

		return map[string]any{
			"go_version":     info.GoVersion,
			"path":          info.Main.Path,
			"version":       info.Main.Version,
			"sum":           info.Main.Sum,
			"build_settings": settings,
		}, nil
	})

	RegisterTool("get_module_deps", "List all module dependencies with versions and checksums", nil, func(args map[string]any) (any, error) {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return map[string]any{"error": "Build info not available"}, nil
		}

		deps := make([]map[string]any, 0, len(info.Deps))
		for _, d := range info.Deps {
			entry := map[string]any{
				"path":    d.Path,
				"version": d.Version,
				"sum":     d.Sum,
				"replaced": d.Replace != nil,
			}
			if d.Replace != nil {
				entry["replace_path"] = d.Replace.Path
				entry["replace_version"] = d.Replace.Version
			}
			deps = append(deps, entry)
		}

		return map[string]any{
			"main_module":       info.Main.Path,
			"main_version":      info.Main.Version,
			"dependency_count":  len(deps),
			"dependencies":      deps,
		}, nil
	})
}
