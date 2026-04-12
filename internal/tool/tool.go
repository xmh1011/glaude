// Package tool defines the unified tool interface and execution contract.
//
// Every tool in glaude implements the Tool interface. The Agent dispatches
// tool_use blocks by name through a Registry, which holds all available tools.
// Tools are self-describing: they provide their own JSON Schema and prompt
// for LLM consumption.
package tool

import (
	"context"
	"encoding/json"
)

// Tool is the unified interface for all agent tools.
// Each tool is self-contained: it declares its schema (for LLM cognition),
// behavioral flags (for the permission system), and execution logic.
type Tool interface {
	// Name returns the tool's unique identifier (e.g. "Bash", "Read").
	Name() string

	// Description returns the LLM-facing description of what this tool does.
	Description() string

	// InputSchema returns the JSON Schema describing the tool's expected input.
	// This is sent to the LLM so it knows how to call the tool.
	InputSchema() json.RawMessage

	// IsReadOnly returns true if the tool never modifies files or system state.
	// Read-only tools may skip permission checks in certain security modes.
	IsReadOnly() bool

	// Execute runs the tool with the given JSON input and returns a text result.
	// The result is fed back to the LLM as a tool_result content block.
	// Errors are returned as the result string with isError=true by the caller.
	Execute(ctx context.Context, input json.RawMessage) (string, error)
}
