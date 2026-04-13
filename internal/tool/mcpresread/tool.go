// Package mcpresread implements the ReadMcpResource tool.
package mcpresread

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/mcp"
)

// Tool reads a specific resource from an MCP server.
type Tool struct {
	Manager *mcp.Manager
}

// Input is the parsed input for the ReadMcpResource tool.
type Input struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "ReadMcpResource" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Reads a specific resource from an MCP server by server name and resource URI."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {"type": "string", "description": "The MCP server name"},
			"uri": {"type": "string", "description": "The resource URI to read"}
		},
		"required": ["server", "uri"]
	}`)
}

// IsReadOnly returns true since this only reads data.
func (t *Tool) IsReadOnly() bool { return true }

// Execute reads a resource from the specified MCP server.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Server == "" {
		return "", fmt.Errorf("server is required")
	}
	if in.URI == "" {
		return "", fmt.Errorf("uri is required")
	}

	client := t.Manager.Client(in.Server)
	if client == nil {
		return fmt.Sprintf("MCP server %q not found.", in.Server), nil
	}

	result, err := client.ReadResource(ctx, in.URI)
	if err != nil {
		return "", fmt.Errorf("read resource: %w", err)
	}

	var b strings.Builder
	for _, c := range result.Contents {
		if c.Text != "" {
			b.WriteString(c.Text)
		} else if c.Blob != "" {
			fmt.Fprintf(&b, "[binary data, %d bytes, mime: %s]", len(c.Blob), c.MimeType)
		}
	}

	if b.Len() == 0 {
		return "Resource returned empty content.", nil
	}
	return b.String(), nil
}
