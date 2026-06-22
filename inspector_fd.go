package debugagent

import (
	"os"
	"runtime"
	"syscall"
)

func registerFdInspector() {
	RegisterTool("get_fd_count", "Get the current number of open file descriptors", nil, func(args map[string]any) (any, error) {
		count, method := getFdCount()
		return map[string]any{
			"fd_count": count,
			"method":   method,
			"os":       runtime.GOOS,
		}, nil
	})

	RegisterTool("get_fd_limit", "Get file descriptor soft and hard limits (RLIMIT_NOFILE)", nil, func(args map[string]any) (any, error) {
		var rlim syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim); err != nil {
			return nil, err
		}
		return map[string]any{
			"soft_limit": rlim.Cur,
			"hard_limit": rlim.Max,
		}, nil
	})
}

// getFdCount returns the number of open file descriptors and the detection
// method used ("procfs", "fstat", or "unknown").
func getFdCount() (int, string) {
	// Linux: read /proc/self/fd
	if runtime.GOOS == "linux" {
		entries, err := os.ReadDir("/proc/self/fd")
		if err == nil {
			return len(entries), "procfs"
		}
	}

	// Generic fallback: iterate FDs 0..limit and probe with Fstat
	var rlim syscall.Rlimit
	maxFd := uint64(4096)
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim); err == nil {
		if rlim.Cur > 0 && rlim.Cur < 1<<20 {
			maxFd = rlim.Cur
		}
	}

	count := 0
	var st syscall.Stat_t
	for fd := 0; uint64(fd) < maxFd; fd++ {
		if syscall.Fstat(fd, &st) == nil {
			count++
		}
	}
	return count, "fstat"
}
