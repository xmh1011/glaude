// Package tool defines the unified tool interface and execution contract.
//
// Every tool in glaude implements the Tool interface. The Agent dispatches
// tool_use blocks by name through a Registry, which holds all available tools.
// Tools are self-describing: they provide their own JSON Schema and prompt
// for LLM consumption.
//
// Individual tool implementations live in sub-packages (e.g. bash,
// fileread) following the same pattern as Claude Code's src/tools/.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for file staleness detection.
var (
	// ErrFileNotRead is returned when an edit/write targets a file that
	// hasn't been read yet in this session.
	ErrFileNotRead = errors.New("file has not been read yet; read it first before editing")

	// ErrFileModified is returned when the file has been modified on disk
	// since the last recorded read/edit.
	ErrFileModified = errors.New("file has been modified since last read; read it again before editing")
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

// DefaultExcludes are directories always excluded from glob/grep results.
// Shared by GlobTool and GrepTool.
var DefaultExcludes = []string{
	".git",
	"node_modules",
	"vendor",
	"__pycache__",
	".DS_Store",
}

// TruncateLines limits output to maxLines, appending a truncation notice.
func TruncateLines(s string, maxLines int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, fmt.Sprintf("\n(results truncated to %d lines)", maxLines))
	}
	return strings.Join(lines, "\n")
}
