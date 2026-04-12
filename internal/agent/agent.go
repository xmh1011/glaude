// Package agent implements the core agentic loop.
//
// The loop is intentionally simple: request -> parse stop_reason -> dispatch.
// All intelligence lives in the LLM; the framework just faithfully executes
// tools and feeds results back.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xmh1011/glaude/internal/compact"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/session"
	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
)

// Agent drives the LLM loop for a single session.
type Agent struct {
	provider      llm.Provider
	model         string
	systemPrompt  string
	messages      []llm.Message
	maxTokens     int
	totalUsage    llm.Usage
	registry      *tool.Registry
	budget        *compact.Budget
	counter       *compact.TokenCounter
	autoCompactor *compact.AutoCompactor
	gate          *permission.Gate
	session       *session.Store // nil = no persistence (tests, sub-agents)
	lastUUID      string         // UUID of the last recorded entry for parentUUID chaining
}

// New creates an Agent bound to the given provider and model.
// The registry may be nil if no tools are available (tools will return errors).
func New(provider llm.Provider, model, systemPrompt string, registry *tool.Registry) *Agent {
	return &Agent{
		provider:      provider,
		model:         model,
		systemPrompt:  systemPrompt,
		maxTokens:     4096,
		registry:      registry,
		budget:        compact.NewBudget(compact.DefaultContextWindow, compact.ResponseReserve),
		counter:       compact.NewTokenCounter(),
		autoCompactor: compact.NewAutoCompactor(provider, model),
		gate:          permission.NewGate(permission.NewChecker(), nil),
	}
}

// SetGate replaces the permission gate. Use this to wire in an interactive
// prompt callback from the UI layer.
func (a *Agent) SetGate(g *permission.Gate) {
	a.gate = g
}

// Gate returns the current permission gate.
func (a *Agent) Gate() *permission.Gate {
	return a.gate
}

// SetSession sets the session store for conversation persistence.
// Pass nil to disable persistence (default).
func (a *Agent) SetSession(s *session.Store) {
	a.session = s
}

// Session returns the current session store, or nil.
func (a *Agent) Session() *session.Store {
	return a.session
}

// RestoreFrom loads a saved conversation into the agent's message history.
// This is used by --continue and --resume to restore a previous session.
func (a *Agent) RestoreFrom(entries []*session.Entry) {
	a.messages = session.ToMessages(session.BuildChain(entries))
	if len(entries) > 0 {
		a.lastUUID = entries[len(entries)-1].UUID
	}
}

// Run executes the agent loop for a single user prompt.
// It returns the assistant's final text response.
func (a *Agent) Run(ctx context.Context, prompt string) (string, error) {
	userMsg := llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.NewTextBlock(prompt)},
	}
	a.messages = append(a.messages, userMsg)
	a.recordEntry("user", &userMsg)

	telemetry.Log.WithField("prompt_len", len(prompt)).Info("agent loop started")

	for iteration := 0; ; iteration++ {
		// Check cancellation before each API call
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Apply MicroCompact: clear old tool results each iteration
		a.messages = compact.MicroCompact(a.messages)

		// Update budget and check if AutoCompact is needed
		a.budget.Update(a.counter, a.systemPrompt, a.toolDefinitions(), a.messages)
		if a.budget.NeedsCompact() {
			telemetry.Log.
				WithField("used", a.budget.Used()).
				WithField("effective_window", a.budget.EffectiveWindow()).
				Info("triggering auto-compact")
			compacted, err := a.autoCompactor.Compact(ctx, a.messages)
			if err != nil {
				telemetry.Log.WithField("error", err.Error()).Warn("auto-compact failed")
			} else {
				a.messages = compacted
				a.budget.ResetCalibration() // API data no longer valid after compaction
			}
		}

		resp, err := a.provider.Complete(ctx, &llm.Request{
			Model:     a.model,
			System:    a.systemPrompt,
			Messages:  a.messages,
			MaxTokens: a.maxTokens,
			Tools:     a.toolDefinitions(),
		})
		if err != nil {
			return "", fmt.Errorf("iteration %d: %w", iteration, err)
		}

		// Track cumulative usage
		a.totalUsage.InputTokens += resp.Usage.InputTokens
		a.totalUsage.OutputTokens += resp.Usage.OutputTokens

		// Calibrate budget with API-reported input tokens for more accurate counting
		a.budget.CalibrateFromAPI(resp.Usage.InputTokens, len(a.messages))

		// Append assistant response to conversation history
		assistantMsg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		}
		a.messages = append(a.messages, assistantMsg)
		a.recordEntry("assistant", &assistantMsg)

		telemetry.Log.
			WithField("iteration", iteration).
			WithField("stop_reason", string(resp.StopReason)).
			WithField("output_tokens", resp.Usage.OutputTokens).
			Debug("loop iteration complete")

		switch resp.StopReason {
		case llm.StopEndTurn:
			// LLM decided the task is done
			return resp.TextContent(), nil

		case llm.StopToolUse:
			toolBlocks := resp.ToolUseBlocks()
			if len(toolBlocks) == 0 {
				return resp.TextContent(), nil
			}

			results := make([]llm.ContentBlock, 0, len(toolBlocks))
			for _, tb := range toolBlocks {
				result, isErr := a.executeTool(ctx, tb)
				results = append(results, llm.NewToolResultBlock(tb.ID, result, isErr))
			}
			toolResultMsg := llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			}
			a.messages = append(a.messages, toolResultMsg)
			a.recordEntry("user", &toolResultMsg)
			// Continue the loop: the LLM will see the result and decide next action

		case llm.StopMaxTokens:
			// Output was truncated; return what we have with an error
			return resp.TextContent(), fmt.Errorf("output truncated (max_tokens reached)")

		default:
			return resp.TextContent(), fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
		}
	}
}

// StreamCallback receives streaming events for real-time UI updates.
type StreamCallback func(event llm.StreamEvent)

// RunStream executes the agent loop with streaming output.
// Text deltas are delivered via callback in real-time. Tool execution
// still happens synchronously after the stream completes each turn.
// If the provider does not support streaming, it falls back to synchronous mode.
func (a *Agent) RunStream(ctx context.Context, prompt string, cb StreamCallback) (string, error) {
	userMsg := llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.NewTextBlock(prompt)},
	}
	a.messages = append(a.messages, userMsg)
	a.recordEntry("user", &userMsg)

	telemetry.Log.WithField("prompt_len", len(prompt)).Info("agent stream loop started")

	// Check if provider supports streaming
	streamProvider, canStream := a.provider.(llm.StreamingProvider)

	for iteration := 0; ; iteration++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		// Apply MicroCompact
		a.messages = compact.MicroCompact(a.messages)

		// Update budget and check if AutoCompact is needed
		a.budget.Update(a.counter, a.systemPrompt, a.toolDefinitions(), a.messages)
		if a.budget.NeedsCompact() {
			telemetry.Log.
				WithField("used", a.budget.Used()).
				WithField("effective_window", a.budget.EffectiveWindow()).
				Info("triggering auto-compact")
			compacted, err := a.autoCompactor.Compact(ctx, a.messages)
			if err != nil {
				telemetry.Log.WithField("error", err.Error()).Warn("auto-compact failed")
			} else {
				a.messages = compacted
				a.budget.ResetCalibration()
			}
		}

		req := &llm.Request{
			Model:     a.model,
			System:    a.systemPrompt,
			Messages:  a.messages,
			MaxTokens: a.maxTokens,
			Tools:     a.toolDefinitions(),
		}

		var resp *llm.Response
		var err error

		if canStream {
			resp, err = a.consumeStream(ctx, streamProvider, req, cb)
		} else {
			resp, err = a.provider.Complete(ctx, req)
		}
		if err != nil {
			return "", fmt.Errorf("iteration %d: %w", iteration, err)
		}

		// Track cumulative usage
		a.totalUsage.InputTokens += resp.Usage.InputTokens
		a.totalUsage.OutputTokens += resp.Usage.OutputTokens

		// Calibrate budget
		a.budget.CalibrateFromAPI(resp.Usage.InputTokens, len(a.messages))

		// Append assistant response
		assistantMsg := llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		}
		a.messages = append(a.messages, assistantMsg)
		a.recordEntry("assistant", &assistantMsg)

		telemetry.Log.
			WithField("iteration", iteration).
			WithField("stop_reason", string(resp.StopReason)).
			WithField("output_tokens", resp.Usage.OutputTokens).
			Debug("stream loop iteration complete")

		switch resp.StopReason {
		case llm.StopEndTurn:
			return resp.TextContent(), nil

		case llm.StopToolUse:
			toolBlocks := resp.ToolUseBlocks()
			if len(toolBlocks) == 0 {
				return resp.TextContent(), nil
			}

			results := make([]llm.ContentBlock, 0, len(toolBlocks))
			for _, tb := range toolBlocks {
				result, isErr := a.executeTool(ctx, tb)
				results = append(results, llm.NewToolResultBlock(tb.ID, result, isErr))
			}
			toolResultMsg := llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			}
			a.messages = append(a.messages, toolResultMsg)
			a.recordEntry("user", &toolResultMsg)

		case llm.StopMaxTokens:
			return resp.TextContent(), fmt.Errorf("output truncated (max_tokens reached)")

		default:
			return resp.TextContent(), fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
		}
	}
}

// toolBuilder accumulates streamed tool call fragments.
type toolBuilder struct {
	id      string
	name    string
	jsonBuf strings.Builder
	index   int
}

// consumeStream reads all events from a streaming completion channel and
// assembles them into a unified Response. Text deltas are forwarded to the
// callback in real-time; tool input fragments are buffered until complete.
func (a *Agent) consumeStream(
	ctx context.Context,
	sp llm.StreamingProvider,
	req *llm.Request,
	cb StreamCallback,
) (*llm.Response, error) {
	ch, err := sp.CompleteStream(ctx, req)
	if err != nil {
		return nil, err
	}

	var textAccum strings.Builder
	toolAccum := map[int]*toolBuilder{}
	var stopReason llm.StopReason
	var usage llm.Usage

	for event := range ch {
		switch event.Type {
		case llm.EventTextDelta:
			textAccum.WriteString(event.Text)
			if cb != nil {
				cb(event)
			}

		case llm.EventToolUseStart:
			toolAccum[event.Index] = &toolBuilder{
				id:    event.ID,
				name:  event.Name,
				index: event.Index,
			}
			if cb != nil {
				cb(event)
			}

		case llm.EventInputJSONDelta:
			if tb, ok := toolAccum[event.Index]; ok {
				tb.jsonBuf.WriteString(event.InputJSON)
			}

		case llm.EventContentBlockStop:
			// Nothing to do for text blocks; tool blocks will be assembled below

		case llm.EventMessageDelta:
			stopReason = event.StopReason
			usage = event.Usage

		case llm.EventError:
			return nil, event.Error
		}
	}

	// Assemble response content blocks
	var content []llm.ContentBlock

	if textAccum.Len() > 0 {
		content = append(content, llm.NewTextBlock(textAccum.String()))
	}

	for _, tb := range toolAccum {
		input := llm.SafeParseJSON([]byte(tb.jsonBuf.String()))
		content = append(content, llm.ContentBlock{
			Type:  llm.ContentToolUse,
			ID:    tb.id,
			Name:  tb.name,
			Input: input,
		})
	}

	return &llm.Response{
		Content:    content,
		StopReason: stopReason,
		Usage:      usage,
	}, nil
}

// executeTool dispatches a tool_use block to the Registry and returns the
// result text and whether it's an error.
func (a *Agent) executeTool(ctx context.Context, tb llm.ContentBlock) (string, bool) {
	if a.registry == nil {
		return fmt.Sprintf("Error: tool %q is not available (no registry)", tb.Name), true
	}

	t := a.registry.Get(tb.Name)
	if t == nil {
		return fmt.Sprintf("Error: tool %q not found", tb.Name), true
	}

	// Permission gate: check before executing
	if a.gate != nil {
		bashCmd := extractBashCommand(tb)
		result := a.gate.Evaluate(ctx, tb.Name, t.IsReadOnly(), bashCmd)

		telemetry.Log.
			WithField("tool", tb.Name).
			WithField("decision", result.Decision.String()).
			WithField("reason", result.Reason).
			Debug("permission check")

		if result.Decision == permission.Deny {
			msg := fmt.Sprintf("Permission denied: %s", result.Reason)
			telemetry.Log.WithField("tool", tb.Name).Info(msg)
			return msg, true
		}
	}

	telemetry.Log.
		WithField("tool", tb.Name).
		WithField("tool_use_id", tb.ID).
		Debug("executing tool")

	result, err := t.Execute(ctx, tb.Input)
	if err != nil {
		telemetry.Log.
			WithField("tool", tb.Name).
			WithField("error", err.Error()).
			Debug("tool execution error")
		if result == "" {
			result = err.Error()
		}
		return result, true
	}

	telemetry.Log.
		WithField("tool", tb.Name).
		WithField("result_len", len(result)).
		Debug("tool execution success")

	return result, false
}

// extractBashCommand extracts the "command" field from a Bash tool's input JSON.
func extractBashCommand(tb llm.ContentBlock) string {
	if tb.Name != "Bash" {
		return ""
	}
	var parsed struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(tb.Input, &parsed); err != nil {
		return ""
	}
	return parsed.Command
}

// TotalUsage returns the cumulative token usage across all iterations.
func (a *Agent) TotalUsage() llm.Usage {
	return a.totalUsage
}

// Messages returns the current conversation history.
func (a *Agent) Messages() []llm.Message {
	return a.messages
}

// Budget returns the current token budget state.
func (a *Agent) Budget() *compact.Budget {
	return a.budget
}

// recordEntry persists a message entry to the session store if available.
func (a *Agent) recordEntry(entryType string, msg *llm.Message) {
	if a.session == nil {
		return
	}
	id := uuid.New().String()
	cwd, _ := os.Getwd()
	entry := &session.Entry{
		Type:       entryType,
		UUID:       id,
		ParentUUID: a.lastUUID,
		SessionID:  a.session.SessionID(),
		CWD:        cwd,
		Timestamp:  time.Now().Format(time.RFC3339),
		Message:    msg,
	}
	if err := a.session.Append(entry); err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("failed to record session entry")
	}
	a.lastUUID = id
}

// RecordLastPrompt writes a "last-prompt" metadata entry for session listing.
func (a *Agent) RecordLastPrompt(prompt string) {
	if a.session == nil {
		return
	}
	cwd, _ := os.Getwd()
	entry := &session.Entry{
		Type:      "last-prompt",
		UUID:      uuid.New().String(),
		SessionID: a.session.SessionID(),
		CWD:       cwd,
		Timestamp: time.Now().Format(time.RFC3339),
		Text:      prompt,
	}
	_ = a.session.Append(entry)
}

// toolDefinitions converts Registry tools to LLM-ready definitions.
func (a *Agent) toolDefinitions() []llm.ToolDefinition {
	if a.registry == nil {
		return nil
	}
	tools := a.registry.All()
	defs := make([]llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return defs
}
