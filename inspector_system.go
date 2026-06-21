package debugagent

import (
	"os"
	"runtime"
	"strings"
	"syscall"
)

func registerSystemInspector() {
	RegisterTool("get_process_info", "Get process info: PID, memory limits, container detection", nil, func(args map[string]any) (any, error) {
		return map[string]any{
			"pid":        os.Getpid(),
			"ppid":       os.Getppid(),
			"go_version": runtime.Version(),
			"platform":   runtime.GOOS + "/" + runtime.GOARCH,
			"cpu_count":  runtime.NumCPU(),
		}, nil
	})

	RegisterTool("get_system_info", "Get system information: hostname, CPU, disk", nil, func(args map[string]any) (any, error) {
		hostname, _ := os.Hostname()
		return map[string]any{
			"hostname":     hostname,
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"cpu_count":    runtime.NumCPU(),
			"gomaxprocs":   runtime.GOMAXPROCS(0),
			"goroutines":   runtime.NumGoroutine(),
		}, nil
	})

	RegisterTool("get_disk_usage", "Get disk usage for current working directory", nil, func(args map[string]any) (any, error) {
		var stat syscall.Statfs_t
		syscall.Statfs(".", &stat)
		total := stat.Blocks * uint64(stat.Bsize)
		free := stat.Bavail * uint64(stat.Bsize)
		return map[string]any{
			"total_gb": total / 1024 / 1024 / 1024,
			"free_gb":  free / 1024 / 1024 / 1024,
			"used_pct": (1 - float64(free)/float64(total)) * 100,
		}, nil
	})

	RegisterTool("get_environment_variables", "List environment variables (potential secrets masked)", map[string]ToolParam{
		"prefix": {Type: "string", Description: "Filter by prefix", Required: false},
	}, func(args map[string]any) (any, error) {
		prefix, _ := args["prefix"].(string)
		secretPatterns := []string{"KEY", "SECRET", "PASSWORD", "TOKEN", "CREDENTIAL"}

		result := map[string]string{}
		for _, env := range os.Environ() {
			// Parse key=value
			for i := 0; i < len(env); i++ {
				if env[i] == '=' {
					key := env[:i]
					val := env[i+1:]

					// Filter by prefix
					if prefix != "" && !strings.HasPrefix(strings.ToUpper(key), strings.ToUpper(prefix)) {
						continue
					}

					// Mask secrets
					isSecret := false
					for _, s := range secretPatterns {
						if strings.Contains(strings.ToUpper(key), s) {
							isSecret = true
							break
						}
					}
					if isSecret {
						result[key] = "***masked***"
					} else {
						result[key] = val
					}
					break
				}
			}
		}
		return map[string]any{"variables": result, "count": len(result)}, nil
	})
}
