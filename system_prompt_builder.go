package debugagent

import (
	"sort"
	"strings"
)

var categoryMap = map[string]string{
	// Memory & GC
	"memory":    "Memory & GC",
	"heap":      "Memory & GC",
	"gc":        "Memory & GC",
	"alloc":     "Memory & GC",
	"mem":       "Memory & GC",
	"leak":      "Memory & GC",
	"snapshots": "Memory & Snapshots",
	"snapshot":  "Memory & Snapshots",
	"compare":   "Memory & Snapshots",
	// Goroutines & Threads
	"goroutine":  "Goroutines",
	"go":         "Goroutines",
	"thread":     "Goroutines",
	"lock":       "Locks & Mutex",
	"mutex":      "Locks & Mutex",
	"contention": "Locks & Mutex",
	"deadlock":   "Locks & Mutex",
	"sync":       "Locks & Mutex",
	"waitgroup":  "Locks & Mutex",
	// Process & Runtime
	"process":   "Process Info",
	"runtime":   "Runtime Info",
	"system":    "System Info",
	"cpu":       "System Info",
	"disk":      "System Info",
	"uptime":    "System Info",
	"pprof":     "Profiling",
	"profile":   "Profiling",
	"start":     "Profiling",
	"stop":      "Profiling",
	"top":       "Profiling",
	// Framework
	"routes":    "Framework",
	"gin":       "Framework",
	"echo":      "Framework",
	"mux":       "Framework",
	"chi":       "Framework",
	// HTTP
	"recent":     "HTTP Requests",
	"slow":       "HTTP Requests",
	"request":    "HTTP Requests",
	"http":       "HTTP Requests",
	"outbound":   "HTTP Requests",
	"client":     "HTTP Requests",
	// Database
	"db":         "Database",
	"gorm":       "Database",
	"sql":        "Database",
	"migration":  "Database Migration",
	"pending":    "Database Migration",
	// Network
	"network":   "Network",
	"dns":       "Network",
	"tcp":       "Network",
	"conn":      "Network",
	"websocket": "WebSocket",
	"ws":        "WebSocket",
	// Module & Build
	"module":    "Module Info",
	"build":     "Build & Deployment",
	"deployment": "Build & Deployment",
	"version":   "Build & Deployment",
	// Configuration
	"config":    "Configuration",
	"env":       "Configuration",
	"environment": "Configuration",
	// Cache
	"cache":     "Cache",
	"redis":     "Redis",
	// Health & Security
	"health":    "Health Checks",
	"security":  "Security",
	"auth":      "Security",
	"cors":      "Security",
	// Error Tracking
	"error":     "Error Tracking",
	"warning":   "Error Tracking",
	// Feature Flags
	"feature":   "Feature Flags",
	"flag":      "Feature Flags",
	"evaluate":  "Feature Flags",
	// Endpoint Testing
	"test":      "Endpoint Testing",
	"batch":     "Endpoint Testing",
	"endpoint":  "Endpoint Testing",
	"coverage":  "Endpoint Testing",
	// Connection Pool
	"pool":      "Connection Pool",
	// File Descriptors
	"fd":        "File Descriptors",
	"handle":    "File Descriptors",
	"open":      "File Descriptors",
	// Metrics
	"metric":    "Metrics",
	"counter":   "Metrics",
	"gauge":     "Metrics",
	// Service Registry
	"registered": "Service Registry",
	"service":    "Service Registry",
	"registry":   "Service Registry",
	"dependencies": "Service Registry",
	// Context
	"context":   "Context & Lifecycle",
	"scheduler": "Scheduler",
	// Logging
	"log":       "Logging",
}

// SystemPromptBuilder generates the system prompt dynamically from registered tools.
type SystemPromptBuilder struct{}

func NewSystemPromptBuilder() *SystemPromptBuilder {
	return &SystemPromptBuilder{}
}

func (b *SystemPromptBuilder) Build() string {
	categories := b.categorizeTools()

	var sb strings.Builder
	sb.WriteString("You are an expert Go runtime debugging assistant.\n")
	sb.WriteString("You are running INSIDE the developer's Go application and have direct access\n")
	sb.WriteString("to its runtime state through diagnostic tools.\n\n")
	sb.WriteString("## Your Capabilities\n")
	sb.WriteString("You can call tools to inspect the live application. Here are ALL available tools,\n")
	sb.WriteString("grouped by category:\n\n")

	for _, category := range sortedKeys(categories) {
		tools := categories[category]
		sb.WriteString("**")
		sb.WriteString(category)
		sb.WriteString("**\n")
		for _, t := range tools {
			sb.WriteString("- `")
			sb.WriteString(t.name)
			sb.WriteString("`: ")
			sb.WriteString(truncateDesc(t.desc))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Workflow\n")
	sb.WriteString("1. Understand the developer's problem description\n")
	sb.WriteString("2. Proactively call the most relevant tools to gather diagnostic data\n")
	sb.WriteString("3. Analyze the collected data to identify root causes\n")
	sb.WriteString("4. Provide clear, actionable solutions with data evidence\n\n")
	sb.WriteString("## Guidelines\n")
	sb.WriteString("- Be proactive: gather data with tools before answering\n")
	sb.WriteString("- Always present data in a readable format (tables, bullet points)\n")
	sb.WriteString("- Respond in the same language the developer uses\n")
	sb.WriteString("- When you find a problem, explain the root cause and give concrete fix suggestions\n")
	sb.WriteString("- You can call multiple tools in parallel if they are independent\n")

	return sb.String()
}

type toolEntry struct {
	name string
	desc string
}

func (b *SystemPromptBuilder) categorizeTools() map[string][]toolEntry {
	categories := map[string][]toolEntry{}
	for _, schema := range AllSchemas() {
		fn := schema["function"].(map[string]any)
		name := fn["name"].(string)
		desc := fn["description"].(string)
		category := extractCategory(name)
		categories[category] = append(categories[category], toolEntry{name, desc})
	}
	return categories
}

func extractCategory(toolName string) string {
	// Remove common verb prefixes
	name := strings.TrimPrefix(toolName, "get_")
	name = strings.TrimPrefix(name, "trigger_")
	name = strings.TrimPrefix(name, "list_")
	name = strings.TrimPrefix(name, "start_")
	name = strings.TrimPrefix(name, "stop_")
	name = strings.TrimPrefix(name, "take_")
	name = strings.TrimPrefix(name, "compare_")
	name = strings.TrimPrefix(name, "test_")
	name = strings.TrimPrefix(name, "batch_")
	name = strings.TrimPrefix(name, "evaluate_")

	// Try keyword containment on the stripped name
	for key, cat := range categoryMap {
		if strings.Contains(name, key) {
			return cat
		}
	}

	// Fallback: try on the full original tool name
	for key, cat := range categoryMap {
		if strings.Contains(toolName, key) {
			return cat
		}
	}

	return "Other Tools"
}

func truncateDesc(desc string) string {
	idx := strings.Index(desc, ".")
	if idx > 0 && idx < 150 {
		return desc[:idx+1]
	}
	if len(desc) > 120 {
		return desc[:117] + "..."
	}
	return desc
}

func sortedKeys(m map[string][]toolEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
