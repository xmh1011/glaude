// Package hook implements a lifecycle hook engine that allows users to run
// external scripts at key points in the agent loop (e.g. before/after tool
// execution, session start/stop).
//
// Hooks are configured in .glaude.json under the "hooks" key and communicate
// via JSON over stdin/stdout with a simple exit-code protocol:
//
//	0 = success, 1 = non-blocking error, 2 = blocking error.
package hook

import "encoding/json"

// Event represents a lifecycle event type.
type Event string

const (
	// PreToolUse fires before each tool execution. Hooks may approve, deny,
	// or modify the tool input.
	PreToolUse Event = "PreToolUse"
	// PostToolUse fires after each tool execution. Hooks receive the result
	// and may inject additional context.
	PostToolUse Event = "PostToolUse"
	// SessionStart fires once when the agent session begins.
	SessionStart Event = "SessionStart"
	// Stop fires when the agent is about to end its turn.
	Stop Event = "Stop"
)

// HookEntry is a single hook command configuration.
type HookEntry struct {
	Type    string `json:"type"`              // v1 only supports "command"
	Command string `json:"command"`           // shell command to execute
	Timeout int    `json:"timeout,omitempty"` // milliseconds, default 10000
}

// HookGroup binds a matcher pattern to a list of hooks.
type HookGroup struct {
	Matcher string      `json:"matcher"` // tool name pattern: "Bash", "Write|Edit", "*"
	Hooks   []HookEntry `json:"hooks"`
}

// HookConfig is the top-level hooks configuration, keyed by Event.
type HookConfig map[Event][]HookGroup

// HookInput is the JSON payload passed to hook commands via stdin.
type HookInput struct {
	SessionID  string          `json:"session_id"`
	Event      Event           `json:"event"`
	CWD        string          `json:"cwd"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolInput  json.RawMessage `json:"tool_input,omitempty"`
	ToolResult string          `json:"tool_result,omitempty"` // PostToolUse only
	IsError    bool            `json:"is_error,omitempty"`    // PostToolUse only
}

// HookOutput is the JSON payload a hook command may write to stdout.
type HookOutput struct {
	Decision     string          `json:"decision,omitempty"`      // "allow" or "deny"
	UpdatedInput json.RawMessage `json:"updatedInput,omitempty"`  // modified tool input
	Continue     *bool           `json:"continue,omitempty"`      // false = stop the session
	Message      string          `json:"message,omitempty"`       // informational message
}

// Decision represents an aggregated permission decision from hooks.
type Decision int

const (
	// None means no hook expressed an opinion.
	None Decision = iota
	// Allow means at least one hook explicitly allowed the action.
	Allow
	// Deny means at least one hook denied the action (overrides Allow).
	Deny
)

// String returns a human-readable representation of a Decision.
func (d Decision) String() string {
	switch d {
	case None:
		return "none"
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	default:
		return "unknown"
	}
}

// HookResult aggregates all matching hook execution results for a single event.
type HookResult struct {
	Decision     Decision        // aggregated permission decision
	UpdatedInput json.RawMessage // last updatedInput from hooks (if any)
	StopSession  bool            // true if any hook set continue=false
	Messages     []string        // informational messages from hooks
	Errors       []error         // non-blocking errors from hooks
	Blocked      bool            // true if any hook returned exit code 2
}
