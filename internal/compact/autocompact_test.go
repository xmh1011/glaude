package compact

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"glaude/internal/llm"
)

// mockCompactProvider is a mock LLM provider for testing AutoCompact.
type mockCompactProvider struct {
	response string
	err      error
	calls    int
}

func (p *mockCompactProvider) Complete(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock(p.response)},
		StopReason: llm.StopEndTurn,
	}, nil
}

func TestAutoCompact_BasicSummarization(t *testing.T) {
	provider := &mockCompactProvider{response: "Summary: user asked to fix a bug"}
	ac := NewAutoCompactor(provider, "test-model")

	messages := buildConversation(10)
	result, err := ac.Compact(context.Background(), messages)
	require.NoError(t, err)

	// Should be shorter than original
	assert.Less(t, len(result), len(messages))
	// First message should be the summary
	assert.Contains(t, result[0].Content[0].Text, "Summary")
	assert.Contains(t, result[0].Content[0].Text, "compressed")
	// Provider should have been called once
	assert.Equal(t, 1, provider.calls)
}

func TestAutoCompact_TooFewMessages(t *testing.T) {
	provider := &mockCompactProvider{response: "summary"}
	ac := NewAutoCompactor(provider, "test-model")

	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("hi")}},
	}

	result, err := ac.Compact(context.Background(), messages)
	require.NoError(t, err)
	assert.Equal(t, messages, result, "should not compact with < 4 messages")
	assert.Equal(t, 0, provider.calls, "should not call LLM")
}

func TestAutoCompact_CircuitBreaker(t *testing.T) {
	provider := &mockCompactProvider{err: fmt.Errorf("API error")}
	ac := NewAutoCompactor(provider, "test-model")

	messages := buildConversation(12)

	// Fail 3 times
	for i := 0; i < MaxConsecutiveFailures; i++ {
		_, err := ac.Compact(context.Background(), messages)
		assert.Error(t, err)
	}

	assert.Equal(t, MaxConsecutiveFailures, ac.ConsecutiveFailures())

	// Circuit breaker should trip
	_, err := ac.Compact(context.Background(), messages)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
	// Should NOT have called provider again (3 calls total, not 4)
	assert.Equal(t, MaxConsecutiveFailures, provider.calls)
}

func TestAutoCompact_ResetFailures(t *testing.T) {
	provider := &mockCompactProvider{err: fmt.Errorf("API error")}
	ac := NewAutoCompactor(provider, "test-model")

	messages := buildConversation(12)
	ac.Compact(context.Background(), messages)
	assert.Equal(t, 1, ac.ConsecutiveFailures())

	// Reset and succeed
	ac.ResetFailures()
	provider.err = nil
	provider.response = "recovered summary"

	result, err := ac.Compact(context.Background(), messages)
	require.NoError(t, err)
	assert.Less(t, len(result), len(messages))
	assert.Equal(t, 0, ac.ConsecutiveFailures())
}

func TestAutoCompact_PreservesRecentMessages(t *testing.T) {
	provider := &mockCompactProvider{response: "conversation summary"}
	ac := NewAutoCompactor(provider, "test-model")

	messages := buildConversation(12)
	lastMsg := messages[len(messages)-1]

	result, err := ac.Compact(context.Background(), messages)
	require.NoError(t, err)

	// The last message should be preserved
	assert.Equal(t, lastMsg.Content[0].Text, result[len(result)-1].Content[0].Text)
}

func TestAdjustSplitForToolPairs(t *testing.T) {
	t.Run("moves split back to include tool_use for preserved tool_result", func(t *testing.T) {
		messages := []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("start")}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
				Type: llm.ContentToolUse, ID: "t1", Name: "Read",
			}}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.NewToolResultBlock("t1", "result", false),
			}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("done")}},
		}

		// Split at index 2 would preserve tool_result but not tool_use
		adjusted := adjustSplitForToolPairs(messages, 2)
		assert.LessOrEqual(t, adjusted, 1, "should move split to include tool_use")
	})

	t.Run("no adjustment needed when pairs are intact", func(t *testing.T) {
		messages := []llm.Message{
			{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("old")}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("old reply")}},
			{Role: llm.RoleAssistant, Content: []llm.ContentBlock{{
				Type: llm.ContentToolUse, ID: "t1", Name: "Read",
			}}},
			{Role: llm.RoleUser, Content: []llm.ContentBlock{
				llm.NewToolResultBlock("t1", "result", false),
			}},
		}

		adjusted := adjustSplitForToolPairs(messages, 2)
		assert.Equal(t, 2, adjusted, "no adjustment needed")
	})
}

// buildConversation creates a test conversation with N exchanges.
func buildConversation(n int) []llm.Message {
	messages := make([]llm.Message, 0, n*2)
	for i := 0; i < n; i++ {
		messages = append(messages,
			llm.Message{
				Role:    llm.RoleUser,
				Content: []llm.ContentBlock{llm.NewTextBlock(fmt.Sprintf("user message %d", i))},
			},
			llm.Message{
				Role:    llm.RoleAssistant,
				Content: []llm.ContentBlock{llm.NewTextBlock(fmt.Sprintf("assistant response %d", i))},
			},
		)
	}
	return messages
}
