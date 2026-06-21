package debugagent

import (
	"sort"
	"strings"
)

var categoryMap = map[string]string{
	"memory":    "Memory & GC",
	"heap":      "Memory & GC",
	"gc":        "Memory & GC",
	"alloc":     "Memory & GC",
	"mem":       "Memory & GC",
	"goroutine": "Goroutines",
	"go":        "Goroutines",
	"process":   "Process Info",
	"system":    "System Info",
	"cpu":       "System Info",
	"disk":      "System Info",
	"uptime":    "System Info",
	"runtime":   "Runtime Info",
	"routes":    "Framework",
	"recent":    "HTTP Requests",
	"slow":      "HTTP Requests",
	"error":     "HTTP Requests",
	"request":   "HTTP Requests",
	"module":    "Module Info",
	"build":     "Build Info",
	"db":        "Database",
	"network":   "Network",
	"dns":       "Network",
	"env":       "Environment",
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
	name := strings.TrimPrefix(toolName, "get_")
	name = strings.TrimPrefix(name, "trigger_")
	for key, cat := range categoryMap {
		if strings.Contains(name, key) {
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
