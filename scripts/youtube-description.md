# YouTube Video Description

## Title

Go Debug Agent v0.5.0 — Security, Health, Scheduler, Error Tracking, WebSocket (65 Tools)

## Description

Chat with your LIVE Go application at runtime. The Go Debug Agent embeds directly into your app and gives an AI assistant access to 65 diagnostic tools across 24 inspectors.

v0.5.0 adds five new inspectors that bring production-grade observability directly into your AI debugging workflow.

NEW — Security Inspector
Inspect auth middleware configs with JWT secrets auto-masked. List active user sessions with IPs. Show password policies with min length, complexity, and expiry rules.

NEW — Health Inspector
Run registered health checks on demand. Get aggregated UP/DOWN/DEGRADED status. Deep-dive into individual components like database, Redis, and disk. Every check is wrapped in panic recovery.

NEW — Scheduler Inspector
Track background jobs and cron schedules. View schedule expressions, next/last run times, and execution history with success rates. Jobs auto-tracked via a ticker wrapper.

NEW — Error Tracking Inspector
Capture runtime errors and panics in a 50-entry ring buffer. Compute error rates per minute and top error types. Group similar errors by normalized patterns. Panics auto-captured via recovery middleware.

NEW — WebSocket Inspector
Monitor live WebSocket connections with session IDs and remote addresses. Track aggregate message stats and average sizes. List pub/sub rooms with subscriber counts.

The demo also showcases all previously released inspectors: runtime memory and GC, goroutine analysis, HTTP request tracking, Gin/Echo/Chi routes, database pools, GORM models, Redis info and latency, pprof profiles, logging buffers, custom metrics, file descriptors, and context trees.

Quick Start — one import, one line:

```go
import agent "github.com/ggcode/debugagent"
http.Handle("/agent/", agent.Middleware(nil))
```

Open localhost:8080/agent and chat with your app.

Features
- 65 tools across 24 inspectors
- Auto-masking of all secret fields
- Panic recovery on every health check and HTTP handler
- Ring buffer error capture with pattern analysis
- WebSocket lifecycle tracking with room support
- All inspectors defensive — nil checks and recover everywhere
- Optional deps via reflection — no hard imports
- Streaming AI responses with tool call badges
- Works with any OpenAI-compatible LLM

Inspector Coverage (65 tools / 24 inspectors)
Runtime 5, Goroutine 4, HTTP Tracker 4, System 4, pprof 5, Logging 4, Redis 3, Routes 3, Cache 3, Metrics 3, Security 3, Health 3, Error Tracking 3, WebSocket 3, Database 2, Build Info 2, Network 2, GORM 2, HTTP Client 2, File Descriptors 2, Scheduler 2, Context 1, Sync 1

GitHub
github.com/topcheer/go-debug-agent

Tags
#golang #security #healthcheck #websocket #errorhandling #cron #scheduler #godebugging #AI #Diagnostics #Goroutines #Redis #Gin #GORM #DeveloperTools #DevOps #Go #AIOps #Observability

## Chapters

00:00 Introduction
00:24 Go Runtime — Memory, GC, Allocations
03:06 Goroutines + Build Info
05:49 HTTP Requests + Gin Routes
08:31 Database (GORM) + Redis Pool
11:14 Logging + Cache Stats
13:57 Security — Auth, Sessions
16:39 Health Checks — DB, Redis, Disk
19:22 Scheduler + Error Tracking
22:04 Outbound HTTP + FD + Metrics
24:47 Comprehensive Multi-Tool Debugging

---

## Thumbnail Text (for image)

Go Debug Agent v0.5.0
Chat with your LIVE app
65 tools / 24 inspectors

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
