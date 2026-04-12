// Package agent implements the core agentic loop.
//
// The loop is intentionally simple: request -> parse stop_reason -> dispatch.
// All intelligence lives in the LLM; the framework just faithfully executes
// tools and feeds results back.
package agent

import (
	"context"
	"fmt"

	"glaude/internal/compact"
	"glaude/internal/llm"
	"glaude/internal/telemetry"
	"glaude/internal/tool"
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
	}
}

// Run executes the agent loop for a single user prompt.
// It returns the assistant's final text response.
func (a *Agent) Run(ctx context.Context, prompt string) (string, error) {
	a.messages = append(a.messages, llm.Message{
		Role:    llm.RoleUser,
		Content: []llm.ContentBlock{llm.NewTextBlock(prompt)},
	})

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

		// Append assistant response to conversation history
		a.messages = append(a.messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

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
			a.messages = append(a.messages, llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			})
			// Continue the loop: the LLM will see the result and decide next action

		case llm.StopMaxTokens:
			// Output was truncated; return what we have with an error
			return resp.TextContent(), fmt.Errorf("output truncated (max_tokens reached)")

		default:
			return resp.TextContent(), fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
		}
	}
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
