# Go Debug Agent

An AI-powered runtime debugging agent that embeds directly into your Go application. Add one import, configure an LLM key, and chat with your live app at `/agent` to inspect goroutines, memory, GC, database connections, build info, HTTP requests, and more.

## Quick Start

### 1. Install

```bash
go get github.com/topcheer/go-debug-agent
```

### 2. Integrate

```go
package main

import (
    "net/http"
    agent "github.com/topcheer/go-debug-agent"
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
export LLM_BASE_URL=https://open.bigmodel.cn/api/coding/paas/v4  # default
export LLM_MODEL=glm-5.2                                          # default
```

Supports any OpenAI-compatible endpoint.

### 4. Run and open

```
http://localhost:8080/agent
```

## Features

- **Streaming AI responses** with real-time tool call badges (pending / success / error)
- **Context compression** — automatically summarizes old conversation when token limit is approached
- **Dark-themed chat UI** with full markdown rendering (tables, code blocks, lists)
- **Max tool rounds** (25) with forced final summary when limit is reached
- **25 diagnostic tools** across 8 inspectors

## Inspectors & Tools (25)

### Runtime Inspector
| Tool | Description |
|------|-------------|
| `get_memory_stats` | Go runtime memory statistics: heap, stack, GC |
| `get_alloc_stats` | Memory allocation stats: mallocs, frees, total alloc bytes |
| `get_mem_stats` | Detailed runtime.MemStats: HeapAlloc, HeapSys, NumGC, PauseNs |
| `trigger_gc` | Force GC with before/after comparison |
| `get_runtime_info` | Go version, CPU cores, GOMAXPROCS |
| `get_gc_stats` | GC pause times, collection count |

### Goroutine Inspector
| Tool | Description |
|------|-------------|
| `get_goroutine_count` | Current number of goroutines |
| `get_goroutine_stacks` | Goroutine stack traces grouped by similarity |
| `get_goroutine_states` | Goroutine state distribution (running, waiting, sleeping) |
| `get_goroutine_dump` | Full goroutine dump with stack traces |
| `sample_goroutine_profile` | Sample goroutine profile for a short duration |

### Database Inspector
| Tool | Description |
|------|-------------|
| `get_db_connections` | List database connection pools with stats |
| `get_db_stats` | Detailed DBStats: wait count/duration, max idle/open |

### Build Info Inspector
| Tool | Description |
|------|-------------|
| `get_build_info` | Go version, module path, build settings |
| `get_module_deps` | List module dependencies with versions |

### Network Inspector
| Tool | Description |
|------|-------------|
| `get_network_info` | Local addresses, hostname, connection overview |
| `get_dns_info` | DNS resolver info and resolution test |

### HTTP Tracker Inspector
| Tool | Description |
|------|-------------|
| `get_recent_requests` | HTTP request ring buffer |
| `get_slow_requests` | Slowest requests by duration |
| `get_error_requests` | Error requests (4xx/5xx) |
| `get_request_stats` | P50/P95/P99 latency, error rate |

### System Inspector
| Tool | Description |
|------|-------------|
| `get_process_info` | PID, memory limits, container detection |
| `get_system_info` | Hostname, CPU, disk |
| `get_disk_usage` | Disk usage for working directory |
| `get_environment_variables` | Environment variables (masked secrets) |

## Custom Tools

```go
import agent "github.com/topcheer/go-debug-agent"

agent.RegisterTool("check_redis", "Check Redis connection", func(args map[string]any) (any, error) {
    return map[string]any{"connected": true}, nil
}, nil)
```

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `LLM_BASE_URL` | `https://open.bigmodel.cn/api/coding/paas/v4` | LLM endpoint |
| `LLM_API_KEY` | (required) | API key |
| `LLM_MODEL` | `glm-5.2` | Model name |
| `LLM_MAX_TOOL_ROUNDS` | `25` | Max tool-calling rounds |
| `LLM_CONTEXT_WINDOW_TOKENS` | `100000` | Context window size |

## Run the Demo

```bash
export LLM_API_KEY=your-key
cd demo && go run main.go
# Open http://localhost:8080/agent
```

## License

MIT
