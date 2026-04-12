// Package mcp implements the Model Context Protocol (MCP) client.
//
// MCP is a JSON-RPC 2.0 based protocol that allows connecting to external
// tool servers. This package supports two transport modes:
//   - Stdio: communicates with a subprocess via stdin/stdout
//   - SSE: communicates with an HTTP server via Server-Sent Events
//
// The client handles the initialize handshake, tools/list discovery, and
// tools/call invocation. Discovered tools are exposed through the standard
// tool.Tool interface for transparent integration with the Agent's Registry.
package mcp

import (
	"encoding/json"
)

// --- JSON-RPC 2.0 types ---

const jsonRPCVersion = "2.0"

// Request is a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError represents a JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

// Notification is a JSON-RPC 2.0 notification (no id, no response expected).
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// --- MCP protocol types ---

// InitializeParams is sent during the initialize handshake.
type InitializeParams struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ClientCaps `json:"capabilities"`
	ClientInfo      ClientInfo `json:"clientInfo"`
}

// ClientCaps declares what the client supports.
type ClientCaps struct{}

// ClientInfo identifies the client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is returned by the server during the initialize handshake.
type InitializeResult struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Capabilities    ServerCaps `json:"capabilities"`
	ServerInfo      ServerInfo `json:"serverInfo"`
}

// ServerCaps declares what the server supports.
type ServerCaps struct {
	Tools *ToolsCap `json:"tools,omitempty"`
}

// ToolsCap indicates the server has tools.
type ToolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ToolInfo describes a tool returned by tools/list.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ToolsListResult is the response to tools/list.
type ToolsListResult struct {
	Tools []ToolInfo `json:"tools"`
}

// ToolCallParams is sent to tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolCallResult is the response to tools/call.
type ToolCallResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ToolContent is a content block in a tool call result.
type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
