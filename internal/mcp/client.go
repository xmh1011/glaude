package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// Client manages the lifecycle of an MCP server connection.
// It handles the initialize handshake and provides methods for
// discovering and calling tools.
type Client struct {
	name      string // server name for namespacing tools
	transport Transport
	server    *InitializeResult
}

// NewClient creates a Client wrapping the given transport.
// The name is used to namespace tools as mcp__{name}__{toolName}.
func NewClient(name string, transport Transport) *Client {
	return &Client{
		name:      name,
		transport: transport,
	}
}

// Initialize performs the MCP initialize handshake.
// This must be called before any other operations.
func (c *Client) Initialize(ctx context.Context) error {
	params := InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    ClientCaps{},
		ClientInfo: ClientInfo{
			Name:    "glaude",
			Version: "dev",
		},
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("marshal init params: %w", err)
	}

	resp, err := c.transport.Send(ctx, &Request{
		Method: "initialize",
		Params: paramsJSON,
	})
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse init result: %w", err)
	}
	c.server = &result

	telemetry.Log.
		WithField("server", result.ServerInfo.Name).
		WithField("version", result.ServerInfo.Version).
		WithField("protocol", result.ProtocolVersion).
		Info("mcp: initialized")

	// Send initialized notification
	if err := c.transport.Notify(ctx, &Notification{Method: "notifications/initialized"}); err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("mcp: failed to send initialized notification")
	}

	return nil
}

// ListTools calls tools/list to discover available tools.
func (c *Client) ListTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := c.transport.Send(ctx, &Request{
		Method: "tools/list",
		Params: json.RawMessage(`{}`),
	})
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}

	var result ToolsListResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list: %w", err)
	}

	telemetry.Log.
		WithField("server", c.name).
		WithField("tool_count", len(result.Tools)).
		Info("mcp: tools discovered")

	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (*ToolCallResult, error) {
	params := ToolCallParams{
		Name:      name,
		Arguments: args,
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("marshal call params: %w", err)
	}

	resp, err := c.transport.Send(ctx, &Request{
		Method: "tools/call",
		Params: paramsJSON,
	})
	if err != nil {
		return nil, fmt.Errorf("tools/call %q: %w", name, err)
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/call result: %w", err)
	}

	return &result, nil
}

// ServerName returns the connected server's name.
func (c *Client) ServerName() string {
	if c.server != nil {
		return c.server.ServerInfo.Name
	}
	return c.name
}

// Name returns the client's configured name used for tool namespacing.
func (c *Client) Name() string {
	return c.name
}

// Close shuts down the transport.
func (c *Client) Close() error {
	return c.transport.Close()
}
