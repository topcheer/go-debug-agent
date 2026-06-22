# Go Debug Agent

[![Go Reference](https://pkg.go.dev/badge/github.com/topcheer/go-debug-agent.svg)](https://pkg.go.dev/github.com/topcheer/go-debug-agent)
![Tools](https://img.shields.io/badge/tools-83-blue)
![Inspectors](https://img.shields.io/badge/inspectors-30-green)
![Go](https://img.shields.io/badge/Go-1.21%2B-00ADD8)
![go.mod](https://img.shields.io/badge/go.mod-v1.21-informational)

An AI-powered runtime debugging agent that embeds directly into your Go application. Add one import, configure an LLM key, and chat with your live app at `/agent` to inspect goroutines, memory, GC, database connections, Redis, GORM models, Gin routes, pprof profiles, build info, HTTP requests, locks & deadlocks, migrations, config, feature flags, endpoint coverage, connection pools, and more — **83 diagnostic tools across 30 inspectors**.

## Version Support

| Go Version | Status |
|------------|--------|
| 1.21       | Minimum supported |
| 1.22       | Supported |
| 1.23       | Supported |
| 1.24       | Supported |
| 1.25       | Supported |
| 1.26       | Tested |

> Requires `log/slog` (introduced in Go 1.21). No experimental features used.

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
- **82 diagnostic tools** across **30 inspectors**
- Zero external dependencies (no Datadog, no Grafana, no APM)

## Inspectors & Tools (83)

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
| `get_goroutine_leaks` | Detect potentially leaked goroutines waiting indefinitely |

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

### Redis Inspector
| Tool | Description |
|------|-------------|
| `get_redis_info` | Redis server info: memory, clients, persistence |
| `get_redis_keys` | Scan Redis keyspace with pattern matching |
| `get_redis_client_stats` | Connected clients, slow log, latency |

### Gin Routes Inspector
| Tool | Description |
|------|-------------|
| `get_gin_routes` | List all registered Gin routes with handlers and groups |

### GORM Inspector
| Tool | Description |
|------|-------------|
| `get_gorm_models` | List registered GORM models, table names, field counts |
| `get_gorm_migrations` | GORM AutoMigrate status and schema version |

### pprof Inspector
| Tool | Description |
|------|-------------|
| `get_pprof_goroutine` | Goroutine profile via runtime/pprof |
| `get_pprof_heap` | Heap allocation profile |
| `get_pprof_profile` | CPU profile snapshot for a short duration |

### Context Inspector
| Tool | Description |
|------|-------------|
| `get_context_tree` | Active context.Context tree with cancellation status |

### Logging Inspector
| Tool | Description |
|------|-------------|
| `get_log_buffer` | Recent log entries from the built-in ring buffer (filter by level/source) |
| `get_log_level` | Current log level for all registered loggers (slog, zap, zerolog) |
| `set_log_level` | Dynamically set the log level for a registered logger |
| `register_logger` | Register a logger for runtime inspection |

### Cache Inspector
| Tool | Description |
|------|-------------|
| `get_cache_stats` | Stats for registered caches (hit rate, miss count, key count) |
| `get_cache_keys` | List keys in a registered cache with optional prefix filter |
| `clear_cache` | Clear all entries from a registered cache |

### Outbound HTTP Inspector
| Tool | Description |
|------|-------------|
| `get_http_transport_stats` | http.Transport pool stats (MaxIdleConns, idle connections per host) |
| `get_outbound_summary` | Aggregated outbound HTTP call stats (total, avg latency, error rate, top hosts) |

### File Descriptor Inspector
| Tool | Description |
|------|-------------|
| `get_fd_count` | Current number of open file descriptors |
| `get_fd_limit` | File descriptor soft and hard limits (RLIMIT_NOFILE) |

### Metrics Inspector
| Tool | Description |
|------|-------------|
| `get_registered_metrics` | List all registered Prometheus metrics (name, type, help, sample count) |
| `get_metric_value` | Get current value of a specific metric by name |

### Wait Groups Inspector
| Tool | Description |
|------|-------------|
| `get_wait_groups` | List registered sync.WaitGroup states (counter, waiter count) |

### Locks & Deadlock Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `get_lock_contention` | Mutex/RWMutex contention stats from pprof mutex profile |
| `get_block_profile` | Goroutine blocking profile (channels, mutexes, WaitGroups) |
| `detect_deadlock` | Analyze all goroutines for deadlock patterns (circular wait) |
| `get_mutex_holders` | List which goroutines hold which registered mutexes |

### Migration Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `get_migration_status` | Current schema version, applied count, last migration |
| `get_pending_migrations` | Migrations not yet applied (version, description) |
| `get_migration_history` | Applied migration history (version, applied_at, duration) |

### Configuration Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `get_config_snapshot` | All registered config values (sensitive keys masked) |
| `get_env_vars` | Process environment variables with prefix filter |
| `get_config_diff` | Compare registered defaults vs actual values |

### Feature Flags Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `get_feature_flags` | List all registered feature flags with current state |
| `evaluate_flag` | Evaluate a specific flag for a context/user |

### Endpoint Testing Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `test_endpoint` | Make an HTTP request to own app, return full response |
| `batch_test_endpoints` | Test multiple endpoints in one call |
| `get_endpoint_coverage` | Compare registered routes vs tested endpoints |

### Connection Pool Inspector (v0.6.0)
| Tool | Description |
|------|-------------|
| `get_pool_details` | Detailed DB pool stats (MaxOpen, InUse, Idle, WaitCount) |
| `detect_pool_leaks` | Heuristic leak detection (high wait ratio, saturation) |
| `get_pool_wait_stats` | Connection acquire wait stats (avg, P95, max, timeout) |

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

The demo uses **Gin** + **GORM/SQLite** + **Redis**. Start Redis with Docker Compose first:

### Docker Compose

```yaml
# docker-compose.yml
services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    command: redis-server --save 60 1 --loglevel warning
```

```bash
docker compose up -d
```

### Start the app

```bash
export LLM_API_KEY=your-key
cd demo && go run main.go
# Open http://localhost:8080/agent
```

## Built With

[![ggcode](https://img.shields.io/badge/built%20with-ggcode-blue)](https://github.com/topcheer/ggcode)

This project was built using [ggcode](https://github.com/topcheer/ggcode) — an AI coding assistant for terminal-based development.

## License

MIT
