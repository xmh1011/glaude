// Package agent implements the core agentic loop.
//
// The loop is intentionally simple: request -> parse stop_reason -> dispatch.
// All intelligence lives in the LLM; the framework just faithfully executes
// tools and feeds results back.
package agent

import (
	"context"
	"fmt"

	"glaude/internal/llm"
	"glaude/internal/telemetry"
)

// Agent drives the LLM loop for a single session.
type Agent struct {
	provider     llm.Provider
	model        string
	systemPrompt string
	messages     []llm.Message
	maxTokens    int
	totalUsage   llm.Usage
}

// New creates an Agent bound to the given provider and model.
func New(provider llm.Provider, model, systemPrompt string) *Agent {
	return &Agent{
		provider:     provider,
		model:        model,
		systemPrompt: systemPrompt,
		maxTokens:    4096,
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

		resp, err := a.provider.Complete(ctx, &llm.Request{
			Model:     a.model,
			System:    a.systemPrompt,
			Messages:  a.messages,
			MaxTokens: a.maxTokens,
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
			// Execute tools and feed results back (Phase 2 will implement real tools).
			// For now, return error results so the loop can continue or the caller
			// can extract the text portion.
			toolBlocks := resp.ToolUseBlocks()
			if len(toolBlocks) == 0 {
				return resp.TextContent(), nil
			}

			results := make([]llm.ContentBlock, 0, len(toolBlocks))
			for _, tb := range toolBlocks {
				results = append(results, llm.NewToolResultBlock(
					tb.ID,
					fmt.Sprintf("Error: tool %q is not yet implemented", tb.Name),
					true,
				))
			}
			a.messages = append(a.messages, llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			})
			// Continue the loop: the LLM will see the error and adapt

		case llm.StopMaxTokens:
			// Output was truncated; return what we have with an error
			return resp.TextContent(), fmt.Errorf("output truncated (max_tokens reached)")

		default:
			return resp.TextContent(), fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
		}
	}
}

// TotalUsage returns the cumulative token usage across all iterations.
func (a *Agent) TotalUsage() llm.Usage {
	return a.totalUsage
}

// Messages returns the current conversation history.
func (a *Agent) Messages() []llm.Message {
	return a.messages
}
