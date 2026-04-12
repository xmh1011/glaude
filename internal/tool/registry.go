package tool

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Registry holds all available tools, indexed by name.
// Tools are registered at startup and remain immutable during a session.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Panics on duplicate names to
// catch wiring bugs at startup rather than at runtime.
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; exists {
		panic(fmt.Sprintf("tool: duplicate registration for %q", name))
	}
	r.tools[name] = t
}

// Get returns the tool with the given name, or nil if not found.
func (r *Registry) Get(name string) Tool {
	return r.tools[name]
}

// All returns all registered tools sorted by name.
// Deterministic ordering ensures stable prompt cache behavior.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name() < out[j].Name()
	})
	return out
}

// ToolDefinition is the JSON structure sent to the LLM for each tool.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// Definitions returns the LLM-ready tool definitions for all registered tools.
func (r *Registry) Definitions() []ToolDefinition {
	tools := r.All()
	defs := make([]ToolDefinition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return defs
}
