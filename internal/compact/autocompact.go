package compact

import (
	"context"
	"fmt"
	"strings"

	"glaude/internal/llm"
)

// AutoCompact configuration.
const (
	// MaxConsecutiveFailures is the circuit breaker threshold.
	MaxConsecutiveFailures = 3
)

// compactPrompt is the system prompt for generating conversation summaries.
const compactPrompt = `You are a conversation summarizer. Your task is to create a structured summary of the conversation that preserves all essential information while being as concise as possible.

Generate a summary with these sections:

1. **Primary Request**: What the user originally asked for
2. **Key Technical Concepts**: Important technical details, patterns, or decisions
3. **Files and Code**: Specific files modified or discussed, with key code snippets
4. **Errors and Fixes**: Any errors encountered and how they were resolved
5. **Current State**: What has been accomplished so far
6. **Pending Work**: What still needs to be done
7. **Next Step**: The immediate next action to take

Be precise and include specific names, paths, and values. Do not be vague.`

// AutoCompactor performs LLM-driven conversation summarization.
type AutoCompactor struct {
	provider           llm.Provider
	model              string
	counter            *TokenCounter
	consecutiveFailures int
}

// NewAutoCompactor creates an AutoCompactor bound to the given provider.
func NewAutoCompactor(provider llm.Provider, model string) *AutoCompactor {
	return &AutoCompactor{
		provider: provider,
		model:    model,
		counter:  NewTokenCounter(),
	}
}

// Compact summarizes older messages and returns a compacted message slice.
// It preserves the most recent messages and replaces older ones with a summary.
// Returns the original messages unchanged if compaction fails or is unnecessary.
func (ac *AutoCompactor) Compact(ctx context.Context, messages []llm.Message) ([]llm.Message, error) {
	if ac.consecutiveFailures >= MaxConsecutiveFailures {
		return messages, fmt.Errorf("auto-compact circuit breaker: %d consecutive failures", ac.consecutiveFailures)
	}

	if len(messages) < 4 {
		return messages, nil
	}

	// Find a split point: preserve the last ~30% of messages but at least 4
	preserveCount := len(messages) / 3
	if preserveCount < 4 {
		preserveCount = 4
	}
	if preserveCount >= len(messages) {
		return messages, nil
	}

	splitIdx := len(messages) - preserveCount

	// Adjust split to not break tool_use/tool_result pairs
	splitIdx = adjustSplitForToolPairs(messages, splitIdx)
	if splitIdx <= 0 {
		return messages, nil
	}

	toSummarize := messages[:splitIdx]
	toPreserve := messages[splitIdx:]

	// Build the summarization request
	summary, err := ac.generateSummary(ctx, toSummarize)
	if err != nil {
		ac.consecutiveFailures++
		return messages, fmt.Errorf("auto-compact: %w", err)
	}

	ac.consecutiveFailures = 0

	// Build compacted message slice
	compacted := make([]llm.Message, 0, 1+len(toPreserve))

	// Insert summary as a system-like user message
	compacted = append(compacted, llm.Message{
		Role: llm.RoleUser,
		Content: []llm.ContentBlock{
			llm.NewTextBlock(fmt.Sprintf("[Conversation summary - %d messages compressed]\n\n%s", len(toSummarize), summary)),
		},
	})

	compacted = append(compacted, toPreserve...)

	return compacted, nil
}

// generateSummary calls the LLM to produce a conversation summary.
func (ac *AutoCompactor) generateSummary(ctx context.Context, messages []llm.Message) (string, error) {
	// Build a text representation of messages to summarize
	var sb strings.Builder
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]\n", msg.Role))
		for _, block := range msg.Content {
			switch block.Type {
			case llm.ContentText:
				sb.WriteString(block.Text)
				sb.WriteString("\n")
			case llm.ContentToolUse:
				sb.WriteString(fmt.Sprintf("Tool call: %s\n", block.Name))
			case llm.ContentToolResult:
				content := block.Content
				if len(content) > 2000 {
					content = content[:2000] + "...(truncated)"
				}
				sb.WriteString(fmt.Sprintf("Tool result: %s\n", content))
			}
		}
		sb.WriteString("\n")
	}

	resp, err := ac.provider.Complete(ctx, &llm.Request{
		Model:  ac.model,
		System: compactPrompt,
		Messages: []llm.Message{
			{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.NewTextBlock("Please summarize this conversation:\n\n" + sb.String())},
			},
		},
		MaxTokens: 4096,
	})
	if err != nil {
		return "", err
	}

	return resp.TextContent(), nil
}

// adjustSplitForToolPairs ensures the split point doesn't break tool_use/tool_result pairs.
// It moves the split forward until no tool_use in the preserved part is missing
// its tool_result in the summarized part (or vice versa).
func adjustSplitForToolPairs(messages []llm.Message, splitIdx int) int {
	// Collect tool_use IDs in the preserved (post-split) messages
	preservedToolUseIDs := make(map[string]bool)
	preservedToolResultIDs := make(map[string]bool)

	for i := splitIdx; i < len(messages); i++ {
		for _, block := range messages[i].Content {
			if block.Type == llm.ContentToolUse {
				preservedToolUseIDs[block.ID] = true
			}
			if block.Type == llm.ContentToolResult {
				preservedToolResultIDs[block.ToolUseID] = true
			}
		}
	}

	// Check if any tool_result in preserved part references a tool_use in summarized part
	for id := range preservedToolResultIDs {
		if !preservedToolUseIDs[id] {
			// Need to include the tool_use — move split back
			for i := splitIdx - 1; i >= 0; i-- {
				for _, block := range messages[i].Content {
					if block.Type == llm.ContentToolUse && block.ID == id {
						splitIdx = i
						goto done
					}
				}
			}
		}
	}
done:

	// Also check if any tool_use in preserved part has its result in summarized part
	for id := range preservedToolUseIDs {
		if !preservedToolResultIDs[id] {
			// Move split forward to include both
			for i := splitIdx; i < len(messages); i++ {
				for _, block := range messages[i].Content {
					if block.Type == llm.ContentToolResult && block.ToolUseID == id {
						// Already in preserved, ok
						goto done2
					}
				}
			}
			// Result not found in preserved — it's in summarized, that's ok
		}
	}
done2:

	return splitIdx
}

// ConsecutiveFailures returns the current failure count (for testing/monitoring).
func (ac *AutoCompactor) ConsecutiveFailures() int {
	return ac.consecutiveFailures
}

// ResetFailures resets the circuit breaker counter.
func (ac *AutoCompactor) ResetFailures() {
	ac.consecutiveFailures = 0
}
