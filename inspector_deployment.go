package debugagent

import (
	"os"
	"runtime"
	"runtime/debug"
	"time"
)

// agentProcessStart records when the debug agent package was loaded.
var agentProcessStart = time.Now()

func registerDeploymentInspector() {
	RegisterTool("get_build_info", "Get comprehensive build info: Go version, module path/version, VCS revision (git SHA), VCS time, VCS modified flag, CGO enabled, compiler.", nil, func(args map[string]any) (any, error) {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return map[string]any{"error": "Build info not available (binary may not have been built with module support)"}, nil
		}

		settings := map[string]string{}
		for _, s := range info.Settings {
			settings[s.Key] = s.Value
		}

		result := map[string]any{
			"go_version":    info.GoVersion,
			"build_path":    info.Main.Path,
			"module_version": info.Main.Version,
			"module_sum":    info.Main.Sum,
			"compiler":      runtime.Compiler,
			"cgo_enabled":   settings["CGO_ENABLED"] == "1",
		}

		// VCS info
		if rev, ok := settings["vcs.revision"]; ok {
			result["vcs_revision"] = rev
			// Short SHA
			if len(rev) >= 8 {
				result["vcs_short_sha"] = rev[:8]
			}
		}
		if t, ok := settings["vcs.time"]; ok {
			result["vcs_time"] = t
		}
		if modified, ok := settings["vcs.modified"]; ok {
			result["vcs_modified"] = modified == "true"
		}

		// Build tags and other settings
		buildTags := []string{}
		for _, s := range info.Settings {
			if s.Key == "-tags" && s.Value != "" {
				buildTags = append(buildTags, s.Value)
			}
		}
		if len(buildTags) > 0 {
			result["build_tags"] = buildTags
		}

		result["dependency_count"] = len(info.Deps)

		return result, nil
	})

	RegisterTool("get_deployment_info", "Get deployment info: hostname, OS, arch, container detected, PID, start time, uptime seconds.", nil, func(args map[string]any) (any, error) {
		hostname, _ := os.Hostname()

		result := map[string]any{
			"hostname":     hostname,
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"pid":          os.Getpid(),
			"ppid":         os.Getppid(),
			"start_time":   agentProcessStart.Format(time.RFC3339),
			"uptime_seconds": int(time.Since(agentProcessStart).Seconds()),
		}

		// Container detection
		containerType := detectContainer()
		if containerType != "" {
			result["container_detected"] = true
			result["container_type"] = containerType
		} else {
			result["container_detected"] = false
		}

		// Working directory
		if wd, err := os.Getwd(); err == nil {
			result["working_dir"] = wd
		}

		return result, nil
	})

	RegisterTool("get_runtime_version", "Get runtime version details: Go runtime version, GOMAXPROCS, NumCPU, key module versions from build info.", nil, func(args map[string]any) (any, error) {
		result := map[string]any{
			"go_runtime_version": runtime.Version(),
			"gomaxprocs":         runtime.GOMAXPROCS(0),
			"num_cpu":            runtime.NumCPU(),
			"goroutines":         runtime.NumGoroutine(),
			"compiler":           runtime.Compiler,
		}

		// Key module versions from build info
		info, ok := debug.ReadBuildInfo()
		if ok {
			result["main_module"] = info.Main.Path
			result["main_version"] = info.Main.Version

			keyModules := map[string]string{}
			for _, d := range info.Deps {
				// Include major framework dependencies
				if isKeyModule(d.Path) {
					keyModules[d.Path] = d.Version
				}
			}
			result["key_module_versions"] = keyModules
			result["total_dependencies"] = len(info.Deps)
		}

		return result, nil
	})
}

// detectContainer checks for common container environment indicators.
func detectContainer() string {
	defer func() {
		_ = recover()
	}()

	// Check /.dockerenv (Docker)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}

	// Check /proc/1/cgroup for container runtime indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if containsAny(content, "docker", "containerd", "kubepods", "k8s") {
			return "kubernetes"
		}
	}

	// Check environment variables
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "kubernetes"
	}

	return ""
}

// isKeyModule returns true for well-known Go framework/module paths.
func isKeyModule(path string) bool {
	keyPrefixes := []string{
		"github.com/gin-gonic/",
		"github.com/gorilla/",
		"github.com/redis/go-redis/",
		"gorm.io/",
		"github.com/labstack/",
		"github.com/jackc/",
		"go.mongodb.org/",
		"github.com/go-redis/",
		"github.com/spf13/",
		"github.com/sirupsen/",
		"go.uber.org/",
		"github.com/prometheus/",
		"google.golang.org/grpc",
		"google.golang.org/protobuf",
	}
	for _, prefix := range keyPrefixes {
		if len(path) >= len(prefix) && path[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}


