# Go Debug Agent

An AI-powered runtime debugging agent that embeds directly into your Go application. Add one import, configure an LLM key, and chat with your live app at `/agent` to inspect goroutines, memory, GC, HTTP requests, and more.

## Quick Start

### 1. Install

```bash
go get github.com/ggcode/debugagent
```

### 2. Integrate

```go
package main

import (
    "net/http"
    agent "github.com/ggcode/debugagent"
)

func main() {
    // One line to integrate
    http.Handle("/agent/", agent.Middleware(nil))
    http.ListenAndServe(":8080", nil)
}
```

### 3. Configure LLM

```bash
export LLM_API_KEY=your-key
export LLM_BASE_URL=https://api.openai.com/v1  # optional
export LLM_MODEL=gpt-4o                         # optional
```

### 4. Run and open

```
http://localhost:8080/agent
```

## Built-in Tools (15+)

| Tool | Description |
|------|-------------|
| `get_memory_stats` | Go runtime heap, stack, GC stats |
| `trigger_gc` | Force GC with before/after comparison |
| `get_goroutine_dump` | Goroutine count and optional stack traces |
| `get_runtime_info` | Go version, GOMAXPROCS, CPU, PID |
| `get_gc_stats` | GC pause times, collection count |
| `get_cpu_profile` | Quick CPU sampling snapshot |
| `get_recent_requests` | HTTP request ring buffer |
| `get_slow_requests` | Slowest requests by duration |
| `get_error_requests` | Error requests (4xx/5xx) |
| `get_request_stats` | P50/P95/P99 latency, error rate |
| `get_process_info` | PID, platform, CPU count |
| `get_system_info` | Hostname, OS, goroutines |
| `get_disk_usage` | Disk usage for working directory |
| `get_environment_variables` | Environment variables (masked secrets) |

## Custom Tools

```go
import agent "github.com/ggcode/debugagent"

agent.RegisterTool("check_redis", "Check Redis connection", func(args map[string]any) (any, error) {
    return map[string]any{"connected": true}, nil
}, nil)
```

## Run the Demo

```bash
export LLM_API_KEY=your-key
cd demo && go run main.go
# Open http://localhost:8080/agent
```

## License

MIT
