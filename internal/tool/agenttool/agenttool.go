package agenttool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xmh1011/glaude/internal/compact"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
)

// AgentTool spawns an isolated sub-agent with its own context budget and
// message history. The sub-agent inherits the parent's LLM provider and
// tool registry but runs a completely independent conversation loop.
// Only the final text conclusion is returned — intermediate reasoning is discarded.
type AgentTool struct {
	Provider llm.Provider
	Model    string
	Registry *tool.Registry
}

// Input is the parsed input for the Agent tool.
type Input struct {
	Prompt      string `json:"prompt"`
	SubAgent    string `json:"subagent_type"`
	Description string `json:"description"`
}

func (a *AgentTool) Name() string { return "Agent" }

func (a *AgentTool) Description() string {
	return "Launches an isolated sub-agent with its own context and message history. " +
		"The sub-agent inherits the current tools but runs independently. " +
		"Only the final conclusion is returned."
}

func (a *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {"type": "string", "description": "The task for the sub-agent to perform"},
			"subagent_type": {"type": "string", "description": "The type of sub-agent (e.g. general-purpose, Explore)"},
			"description": {"type": "string", "description": "A short description of the sub-agent task"}
		},
		"required": ["prompt"]
	}`)
}

func (a *AgentTool) IsReadOnly() bool { return true }

func (a *AgentTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	desc := in.Description
	if desc == "" {
		desc = "sub-agent"
	}

	telemetry.Log.
		WithField("subagent_type", in.SubAgent).
		WithField("description", desc).
		Info("spawning sub-agent")

	// Build sub-agent system prompt — focused on the delegated task
	systemPrompt := "You are a sub-agent performing a specific task. " +
		"Complete the task thoroughly and return a clear, concise conclusion. " +
		"Do not ask follow-up questions."

	// Create an isolated sub-agent loop
	result, usage, err := a.runSubAgent(ctx, systemPrompt, in.Prompt)

	telemetry.Log.
		WithField("description", desc).
		WithField("input_tokens", usage.InputTokens).
		WithField("output_tokens", usage.OutputTokens).
		WithField("error", err).
		Info("sub-agent completed")

	if err != nil {
		return fmt.Sprintf("Sub-agent error: %v\n\nPartial result: %s", err, result), err
	}

	return result, nil
}

// runSubAgent executes an isolated agent loop with its own message history and budget.
func (a *AgentTool) runSubAgent(ctx context.Context, systemPrompt, prompt string) (string, llm.Usage, error) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock(prompt)}},
	}

	var totalUsage llm.Usage
	maxIterations := 20 // safety limit for sub-agents
	counter := compact.NewTokenCounter()
	budget := compact.NewBudget(compact.DefaultContextWindow/2, compact.ResponseReserve)

	tools := a.toolDefinitions()

	for i := 0; i < maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			return "", totalUsage, err
		}

		// MicroCompact to keep sub-agent context lean
		messages = compact.MicroCompact(messages)

		// Budget check
		budget.Update(counter, systemPrompt, tools, messages)
		if budget.NeedsCompact() {
			// Sub-agents don't get AutoCompact — just truncate early
			return textFromMessages(messages), totalUsage,
				fmt.Errorf("sub-agent context budget exhausted")
		}

		resp, err := a.Provider.Complete(ctx, &llm.Request{
			Model:     a.Model,
			System:    systemPrompt,
			Messages:  messages,
			MaxTokens: 4096,
			Tools:     tools,
		})
		if err != nil {
			return textFromMessages(messages), totalUsage, fmt.Errorf("sub-agent LLM: %w", err)
		}

		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens

		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

		switch resp.StopReason {
		case llm.StopEndTurn:
			return resp.TextContent(), totalUsage, nil

		case llm.StopToolUse:
			toolBlocks := resp.ToolUseBlocks()
			if len(toolBlocks) == 0 {
				return resp.TextContent(), totalUsage, nil
			}

			results := make([]llm.ContentBlock, 0, len(toolBlocks))
			for _, tb := range toolBlocks {
				result, isErr := a.ExecuteTool(ctx, tb)
				results = append(results, llm.NewToolResultBlock(tb.ID, result, isErr))
			}
			messages = append(messages, llm.Message{
				Role:    llm.RoleUser,
				Content: results,
			})

		case llm.StopMaxTokens:
			return resp.TextContent(), totalUsage,
				fmt.Errorf("sub-agent output truncated (max_tokens)")

		default:
			return resp.TextContent(), totalUsage,
				fmt.Errorf("sub-agent unexpected stop: %s", resp.StopReason)
		}
	}

	return textFromMessages(messages), totalUsage,
		fmt.Errorf("sub-agent reached max iterations (%d)", maxIterations)
}

// ExecuteTool dispatches to the Registry (exported for testing).
func (a *AgentTool) ExecuteTool(ctx context.Context, tb llm.ContentBlock) (string, bool) {
	if a.Registry == nil {
		return fmt.Sprintf("Error: tool %q not available", tb.Name), true
	}
	t := a.Registry.Get(tb.Name)
	if t == nil {
		return fmt.Sprintf("Error: tool %q not found", tb.Name), true
	}
	// Prevent sub-agent from spawning another sub-agent (no recursion)
	if tb.Name == "Agent" {
		return "Error: sub-agents cannot spawn further sub-agents", true
	}
	result, err := t.Execute(ctx, tb.Input)
	if err != nil {
		if result == "" {
			result = err.Error()
		}
		return result, true
	}
	return result, false
}

// toolDefinitions builds LLM tool defs, excluding AgentTool to prevent recursion.
func (a *AgentTool) toolDefinitions() []llm.ToolDefinition {
	if a.Registry == nil {
		return nil
	}
	tools := a.Registry.All()
	defs := make([]llm.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		if t.Name() == "Agent" {
			continue // exclude self
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.InputSchema(),
		})
	}
	return defs
}

// textFromMessages extracts the last assistant text from messages.
func textFromMessages(msgs []llm.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == llm.RoleAssistant {
			for _, b := range msgs[i].Content {
				if b.Type == llm.ContentText && b.Text != "" {
					return b.Text
				}
			}
		}
	}
	return ""
}
