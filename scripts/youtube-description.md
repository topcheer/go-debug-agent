# YouTube Video Description

## Title

Go Debug Agent — AI-Powered In-Process Diagnostics for Go Applications (8 Inspectors / 25 Tools)

## Description

Chat with your LIVE Go application at runtime. The Go Debug Agent embeds directly into your app as middleware and gives an AI assistant access to 25 diagnostic tools across 8 inspectors — goroutines, memory & GC, HTTP requests, runtime info, system info, build info, database pools, and network.

No external agents. No attach-to-process. No separate monitoring stack. Just one import, one line of code, and you're chatting with your running app.

### What you'll see in this demo

**Section 1 — Go Runtime + Memory Deep Dive**
Memory stats, GC collections, allocation stats (mallocs/frees), detailed MemStats, and forcing a garbage collection — all through natural language.

**Section 2 — Goroutines + Build Info**
Goroutine count, state distribution (running/waiting/sleeping), stack traces grouped by similarity, Go version, and module dependencies.

**Section 3 — HTTP Request Tracking**
Recent requests, P50/P95/P99 latency statistics, slowest requests, and error requests (4xx/5xx).

**Section 4 — System Info + Environment**
Hostname, CPU count, GOMAXPROCS, disk usage, environment variables (with secret masking).

**Section 5 — Network + DNS**
Local addresses, network interfaces, DNS resolver info, and live DNS resolution timing.

**Section 6 — Comprehensive Debugging**
Multi-tool correlation: memory + goroutines + HTTP + system + build — all in one analysis.

### Quick Start

```go
package main

import (
    "net/http"
    agent "github.com/ggcode/debugagent"
)

func main() {
    mux := http.NewServeMux()
    // Your routes...
    mux.Handle("/agent", agent.Middleware(nil))
    http.ListenAndServe(":8080", mux)
}
```

Set your API key and run:
```bash
export LLM_API_KEY=your-key-here
go run .
```

Open `http://localhost:8080/agent` and start chatting with your app.

### Features

- 25 diagnostic tools across 8 inspectors
- Streaming AI responses with real-time tool call badges
- LLM-based context compression for long conversations
- SSE streaming with content/tool_start/tool_result/done/error events
- Works with any OpenAI-compatible LLM endpoint (Z.ai GLM-5.2 by default)
- Zero external dependencies (no Datadog, no Prometheus, no Grafana)
- Dark-themed chat UI built-in (single HTML page, no frontend framework)
- Markdown rendering with syntax-highlighted code blocks

### Inspector Coverage

| Inspector | Tools | What it inspects |
|-----------|-------|-----------------|
| Runtime | 6 | Memory, GC, goroutine dump, runtime info, GC stats, CPU profile |
| HTTP Requests | 4 | Recent requests, stats (P50/P95/P99), slow, errors |
| System | 4 | Process info, system info, disk usage, environment vars |
| Goroutines | 3 | Count, stacks grouped by similarity, state distribution |
| Memory Allocator | 2 | Alloc stats (mallocs/frees), detailed MemStats |
| Build Info | 2 | Build info (Go version, settings), module dependencies |
| Database | 2 | Connection pools, detailed DBStats |
| Network | 2 | Network stats/interfaces, DNS resolution |

### GitHub

https://github.com/ggcode/go-debug-agent

### Tags

#golang #godebugging #AI #Diagnostics #Observability #LLM #GLM #DeveloperTools #DevOps #ApplicationMonitoring #Go #AIOps #goroutines #runtime #SSE

## Chapters

00:00 Introduction
01:00 Go Runtime — Memory, GC, Allocation Stats
03:10 Goroutines — Count, States, Stack Traces
04:45 Build Info — Go Version, Module Dependencies
05:30 HTTP Request Tracking — Stats, Slow, Errors
07:15 System Info — CPU, Disk, Environment
08:30 Network + DNS Resolution
09:45 Comprehensive Multi-Tool Debugging
11:30 Summary + Quick Start Guide

---

## Thumbnail Text (for image)

Go Debug Agent
Chat with your LIVE app
25 tools / 8 inspectors

---

## Playlist

AI Debug Agents Collection
(Spring / .NET / Go / Node.js / Python / Ruby)

---

## Category

Science & Technology

## Language

English

## Visibility

Public

## Made for Kids

No
