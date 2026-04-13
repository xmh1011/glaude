// Package mcpreslist implements the ListMcpResources tool.
package mcpreslist

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/mcp"
)

// Tool lists resources from connected MCP servers.
type Tool struct {
	Manager *mcp.Manager
}

// Input is the parsed input for the ListMcpResources tool.
type Input struct {
	Server string `json:"server,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "ListMcpResources" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Lists resources available from connected MCP servers. Optionally filter by server name."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {"type": "string", "description": "Optional server name to filter by"}
		}
	}`)
}

// IsReadOnly returns true since this only reads information.
func (t *Tool) IsReadOnly() bool { return true }

// Execute lists resources from MCP servers.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	clients := t.Manager.Clients()
	if len(clients) == 0 {
		return "No MCP servers connected.", nil
	}

	var b strings.Builder
	found := false

	for name, client := range clients {
		if in.Server != "" && name != in.Server {
			continue
		}
		found = true

		resources, err := client.ListResources(ctx)
		if err != nil {
			fmt.Fprintf(&b, "Server %q: error listing resources: %v\n", name, err)
			continue
		}

		if len(resources) == 0 {
			fmt.Fprintf(&b, "Server %q: no resources\n", name)
			continue
		}

		fmt.Fprintf(&b, "Server %q (%d resources):\n", name, len(resources))
		for _, r := range resources {
			fmt.Fprintf(&b, "  - %s (%s)", r.URI, r.Name)
			if r.Description != "" {
				fmt.Fprintf(&b, ": %s", r.Description)
			}
			b.WriteString("\n")
		}
	}

	if !found && in.Server != "" {
		return fmt.Sprintf("Server %q not found.", in.Server), nil
	}

	return b.String(), nil
}
