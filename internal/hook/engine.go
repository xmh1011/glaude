package hook

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// Engine dispatches lifecycle hooks based on configuration.
// It is safe to call methods on a nil *Engine (they are no-ops).
type Engine struct {
	config    HookConfig
	sessionID string
}

// NewEngine creates an Engine that reads hook configuration from viper's
// "hooks" key and snapshots it. sessionID is embedded in every HookInput.
func NewEngine(sessionID string) *Engine {
	e := &Engine{sessionID: sessionID}
	e.config = loadConfig()
	return e
}

// HasHooks reports whether any hooks are configured for the given event.
func (e *Engine) HasHooks(event Event) bool {
	if e == nil {
		return false
	}
	groups, ok := e.config[event]
	return ok && len(groups) > 0
}

// SetConfig replaces the hook configuration. This is primarily useful for
// testing, where configuration is injected directly rather than read from viper.
func (e *Engine) SetConfig(cfg HookConfig) {
	e.config = cfg
}

// Dispatch finds all hooks matching the event and tool name, executes them,
// and returns an aggregated result.
func (e *Engine) Dispatch(ctx context.Context, event Event, input *HookInput) *HookResult {
	if e == nil {
		return &HookResult{}
	}

	groups, ok := e.config[event]
	if !ok || len(groups) == 0 {
		return &HookResult{}
	}

	// Fill in common fields.
	input.SessionID = e.sessionID
	input.Event = event

	// Collect matching hooks.
	var entries []HookEntry
	for _, g := range groups {
		if matchTool(g.Matcher, input.ToolName) {
			entries = append(entries, g.Hooks...)
		}
	}

	if len(entries) == 0 {
		return &HookResult{}
	}

	// Execute each hook and aggregate results.
	var outputs []*HookOutput
	var errors []error
	var blocked bool

	for _, entry := range entries {
		out, err := runCommand(ctx, entry, input)
		if err != nil {
			if _, ok := err.(*blockingError); ok {
				blocked = true
				errors = append(errors, err)
				telemetry.Log.WithField("command", entry.Command).Warn("hook blocking error")
			} else {
				errors = append(errors, err)
				telemetry.Log.WithField("command", entry.Command).
					WithField("error", err.Error()).
					Debug("hook non-blocking error")
			}
			continue
		}
		outputs = append(outputs, out)
	}

	return aggregate(outputs, errors, blocked)
}

// loadConfig reads hook configuration from viper.
func loadConfig() HookConfig {
	raw := viper.Get("hooks")
	if raw == nil {
		return HookConfig{}
	}

	// viper returns interface{} — roundtrip through JSON to unmarshal into our type.
	data, err := json.Marshal(raw)
	if err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("failed to marshal hooks config")
		return HookConfig{}
	}

	var cfg HookConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("failed to unmarshal hooks config")
		return HookConfig{}
	}
	return cfg
}

// matchTool checks whether a tool name matches a pattern.
// Supported patterns:
//   - "" or "*"      → matches everything
//   - "Bash"         → exact match
//   - "Write|Edit"   → pipe-separated exact matches
//   - "*.go"         → filepath.Match glob
func matchTool(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}

	// Pipe-separated list: "Write|Edit|Bash"
	if strings.Contains(pattern, "|") {
		for _, p := range strings.Split(pattern, "|") {
			if strings.TrimSpace(p) == name {
				return true
			}
		}
		return false
	}

	// Exact match (most common case).
	if pattern == name {
		return true
	}

	// Glob match (e.g. "File*").
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		return false
	}
	return matched
}

// aggregate merges all hook outputs and errors into a single HookResult.
// Aggregation rules:
//   - deny > allow (strictest wins)
//   - updatedInput: last one wins
//   - messages: concatenated
//   - any continue=false → StopSession=true
func aggregate(outputs []*HookOutput, errors []error, blocked bool) *HookResult {
	result := &HookResult{
		Errors:  errors,
		Blocked: blocked,
	}

	if blocked {
		result.Decision = Deny
	}

	for _, out := range outputs {
		// Decision aggregation: deny > allow.
		switch strings.ToLower(out.Decision) {
		case "deny":
			result.Decision = Deny
		case "allow":
			if result.Decision != Deny {
				result.Decision = Allow
			}
		}

		// UpdatedInput: last one wins.
		if len(out.UpdatedInput) > 0 {
			result.UpdatedInput = out.UpdatedInput
		}

		// Continue: any false stops the session.
		if out.Continue != nil && !*out.Continue {
			result.StopSession = true
		}

		// Messages: append non-empty.
		if out.Message != "" {
			result.Messages = append(result.Messages, out.Message)
		}
	}

	return result
}
