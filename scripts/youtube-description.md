# YouTube Video Description

## Title

Go Debug Agent — AI-Powered In-Process Diagnostics (36 Tools / 12 Inspectors)

## Description

Chat with your LIVE Go application at runtime. The Go Debug Agent embeds directly into your app and gives an AI assistant access to 36 diagnostic tools across 12 inspectors — runtime memory, goroutines, database pools, build info, network, HTTP requests, system, Redis, Gin routes, GORM models, pprof profiles, and context trees.

No external agents. No attach-to-process. No separate monitoring stack. Just one import, one line of code, and you're chatting with your running app.

### What you'll see in this demo

**Section 1 — Go Runtime Deep Dive**
Memory stats, allocation tracking, GC pause times, forcing a garbage collection, and runtime info — all through natural language.

**Section 2 — Goroutines + Context**
Counting goroutines, dumping stack traces grouped by similarity, detecting leaked goroutines, and inspecting the context tree.

**Section 3 — HTTP Requests + Gin Routes**
Discovering all Gin routes with handlers, analyzing recent HTTP traffic, identifying slow and error requests.

**Section 4 — Database + GORM**
Inspecting database/sql connection pool stats, listing GORM models and table names, checking migration status.

**Section 5 — Redis Inspector**
Server info, keyspace scan with pattern matching, connected client stats and slow log.

**Section 6 — pprof Profiles**
Goroutine profile, heap allocation profile, and CPU profile snapshot via runtime/pprof.

**Section 7 — Comprehensive Debugging**
Multi-tool correlation: memory + GC + goroutines + Redis + GORM + routes + requests — all in one analysis.

### Quick Start

```go
package main

import (
    "net/http"
    agent "github.com/topcheer/go-debug-agent"
)

func main() {
    http.Handle("/agent/", agent.Middleware(nil))
    http.ListenAndServe(":8080", nil)
}
```

Open `http://localhost:8080/agent` and start chatting with your app.

### Features

- 36 diagnostic tools across 12 inspectors
- Streaming AI responses with real-time tool call badges
- LLM-based context compression for long conversations
- Custom tool registration via RegisterTool()
- Works with any OpenAI-compatible LLM endpoint
- Zero external dependencies (no Datadog, no Grafana, no APM)
- Dark-themed chat UI built-in (single HTML page, no frontend framework)

### Inspector Coverage

| Inspector | Tools | What it inspects |
|-----------|-------|-----------------|
| Runtime | 6 | Memory, alloc, GC stats |
| Goroutine | 6 | Count, stacks, states, dumps, leaks |
| Database | 2 | Connection pool stats |
| Build Info | 2 | Go version, module deps |
| Network | 2 | Addresses, DNS |
| HTTP Tracker | 4 | Requests, slow, errors, stats |
| System | 4 | Process, disk, env vars |
| Redis | 3 | Server info, keys, client stats |
| Gin Routes | 1 | Route discovery with handlers |
| GORM | 2 | Models, migrations |
| pprof | 3 | Goroutine, heap, CPU profiles |
| Context | 1 | Active context tree |

### GitHub

github.com/topcheer/go-debug-agent

### Tags

#golang #godebugging #AI #Diagnostics #Goroutines #Redis #Gin #GORM #pprof #LLM #GLM #DeveloperTools #DevOps #ApplicationMonitoring #Go #AIOps #Observability

## Chapters

00:00 Introduction
00:22 Go Runtime — Memory, GC, Allocations
02:21 Goroutines + Build Info
04:20 HTTP Requests + Gin Routes
06:20 Database + GORM Models
08:19 Redis + Network
10:18 Comprehensive Multi-Tool Debugging

---

## Thumbnail Text (for image)

Go Debug Agent
Chat with your LIVE app
36 tools / 12 inspectors

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
