package debugagent

import (
	"os"
)

// getenv is a wrapper for testing.
var getenv = os.Getenv

// ToolFunc is the signature for a debug tool function.
// It receives a map of arguments and returns any JSON-serializable result.
type ToolFunc func(args map[string]any) (any, error)

// ToolParam describes a tool parameter.
type ToolParam struct {
	Type        string
	Description string
	Required    bool
}

// ToolDefinition represents a registered debug tool.
type ToolDefinition struct {
	Name        string
	Description string
	Func        ToolFunc
	Params      map[string]ToolParam
}

// Schema returns the OpenAI function-calling JSON schema for this tool.
func (t *ToolDefinition) Schema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for pname, pmeta := range t.Params {
		props[pname] = map[string]any{
			"type":        pmeta.Type,
			"description": pmeta.Description,
		}
		if pmeta.Required {
			required = append(required, pname)
		}
	}
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"parameters": map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
		},
	}
}

// --- Global Registry ---

var registry = &ToolRegistry{}

type ToolRegistry struct {
	tools map[string]*ToolDefinition
}

func init() {
	registry.tools = make(map[string]*ToolDefinition)
}

// RegisterTool registers a debug tool in the global registry.
func RegisterTool(name, description string, params map[string]ToolParam, fn ToolFunc) {
	registry.tools[name] = &ToolDefinition{
		Name:        name,
		Description: description,
		Func:        fn,
		Params:      params,
	}
}

// AllSchemas returns schemas for all registered tools.
func AllSchemas() []map[string]any {
	result := make([]map[string]any, 0, len(registry.tools))
	for _, t := range registry.tools {
		result = append(result, t.Schema())
	}
	return result
}

// ExecuteTool runs a registered tool by name.
func ExecuteTool(name string, args map[string]any) any {
	tool, ok := registry.tools[name]
	if !ok {
		return map[string]any{"error": "Unknown tool: " + name}
	}
	result, err := tool.Func(args)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	return result
}

// ToolNames returns all registered tool names.
func ToolNames() []string {
	names := make([]string, 0, len(registry.tools))
	for name := range registry.tools {
		names = append(names, name)
	}
	return names
}
