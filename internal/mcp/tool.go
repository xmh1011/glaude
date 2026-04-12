package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
)

// Compile-time check: MCPTool must implement tool.Tool.
var _ tool.Tool = (*MCPTool)(nil)

// MCPTool adapts an MCP server tool to the standard tool.Tool interface.
// Tool names follow the convention: mcp__{serverName}__{toolName}
type MCPTool struct {
	client      *Client
	serverName  string
	toolName    string
	description string
	inputSchema json.RawMessage
}

// NewMCPTool creates a tool adapter for an MCP server tool.
func NewMCPTool(client *Client, info ToolInfo) *MCPTool {
	return &MCPTool{
		client:      client,
		serverName:  client.Name(),
		toolName:    info.Name,
		description: info.Description,
		inputSchema: info.InputSchema,
	}
}

// Name returns the namespaced tool name: mcp__{server}__{tool}
func (m *MCPTool) Name() string {
	return fmt.Sprintf("mcp__%s__%s", m.serverName, m.toolName)
}

// Description returns the tool description from the MCP server.
func (m *MCPTool) Description() string {
	return m.description
}

// InputSchema returns the JSON Schema from the MCP server.
func (m *MCPTool) InputSchema() json.RawMessage {
	return m.inputSchema
}

// IsReadOnly returns false — we can't know if the remote tool mutates state.
func (m *MCPTool) IsReadOnly() bool { return false }

// Execute calls the remote MCP tool and returns the text result.
func (m *MCPTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	result, err := m.client.CallTool(ctx, m.toolName, input)
	if err != nil {
		return "", fmt.Errorf("mcp tool %s: %w", m.Name(), err)
	}

	// Concatenate all text content blocks
	var b strings.Builder
	for i, c := range result.Content {
		if c.Type == "text" {
			if i > 0 && b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(c.Text)
		}
	}

	text := b.String()
	if result.IsError {
		return text, fmt.Errorf("mcp tool %s returned error", m.Name())
	}
	return text, nil
}

// ServerConfig describes an MCP server to connect to.
type ServerConfig struct {
	Name    string   `json:"name"`    // unique name for namespacing
	Command string   `json:"command"` // executable path
	Args    []string `json:"args"`    // command arguments
	Env     []string `json:"env"`     // environment variables
}

// Manager manages multiple MCP server connections and their tool registration.
type Manager struct {
	clients map[string]*Client
}

// NewManager creates an empty MCP manager.
func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

// Connect starts an MCP server subprocess, performs the handshake,
// and returns the discovered tools ready for registration.
func (m *Manager) Connect(ctx context.Context, cfg ServerConfig) ([]*MCPTool, error) {
	transport, err := NewStdioTransport(cfg.Command, cfg.Args, cfg.Env)
	if err != nil {
		return nil, fmt.Errorf("connect %q: %w", cfg.Name, err)
	}

	client := NewClient(cfg.Name, transport)

	if err := client.Initialize(ctx); err != nil {
		transport.Close()
		return nil, fmt.Errorf("initialize %q: %w", cfg.Name, err)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		transport.Close()
		return nil, fmt.Errorf("list tools %q: %w", cfg.Name, err)
	}

	m.clients[cfg.Name] = client

	mcpTools := make([]*MCPTool, 0, len(tools))
	for _, info := range tools {
		mcpTools = append(mcpTools, NewMCPTool(client, info))
	}

	telemetry.Log.
		WithField("server", cfg.Name).
		WithField("tools", len(mcpTools)).
		Info("mcp: server connected and tools discovered")

	return mcpTools, nil
}

// Close shuts down all connected MCP servers.
func (m *Manager) Close() {
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			telemetry.Log.
				WithField("server", name).
				WithField("error", err.Error()).
				Warn("mcp: error closing server")
		}
	}
	m.clients = make(map[string]*Client)
}

// Client returns the client for a specific server, or nil.
func (m *Manager) Client(name string) *Client {
	return m.clients[name]
}

// Clients returns all connected clients.
func (m *Manager) Clients() map[string]*Client {
	return m.clients
}
