package llm

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockProvider_CompleteStream_TextResponse(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content:    []ContentBlock{NewTextBlock("Hello world")},
			StopReason: StopEndTurn,
			Usage:      Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	ch, err := mock.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// Expect: text_delta, content_block_stop, message_delta
	require.Len(t, events, 3)
	assert.Equal(t, EventTextDelta, events[0].Type)
	assert.Equal(t, "Hello world", events[0].Text)
	assert.Equal(t, EventContentBlockStop, events[1].Type)
	assert.Equal(t, EventMessageDelta, events[2].Type)
	assert.Equal(t, StopEndTurn, events[2].StopReason)
	assert.Equal(t, 5, events[2].Usage.OutputTokens)
}

func TestMockProvider_CompleteStream_ToolUseResponse(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content: []ContentBlock{
				NewTextBlock("Let me check..."),
				{
					Type:  ContentToolUse,
					ID:    "call_1",
					Name:  "Bash",
					Input: json.RawMessage(`{"command":"ls"}`),
				},
			},
			StopReason: StopToolUse,
			Usage:      Usage{InputTokens: 20, OutputTokens: 10},
		},
	)

	ch, err := mock.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("list files")}}},
		MaxTokens: 1024,
	})
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	// Expect: text_delta, stop, tool_start, json_delta, stop, message_delta
	require.Len(t, events, 6)

	assert.Equal(t, EventTextDelta, events[0].Type)
	assert.Equal(t, "Let me check...", events[0].Text)

	assert.Equal(t, EventContentBlockStop, events[1].Type)

	assert.Equal(t, EventToolUseStart, events[2].Type)
	assert.Equal(t, "call_1", events[2].ID)
	assert.Equal(t, "Bash", events[2].Name)

	assert.Equal(t, EventInputJSONDelta, events[3].Type)
	assert.Equal(t, `{"command":"ls"}`, events[3].InputJSON)

	assert.Equal(t, EventContentBlockStop, events[4].Type)

	assert.Equal(t, EventMessageDelta, events[5].Type)
	assert.Equal(t, StopToolUse, events[5].StopReason)
}

func TestMockProvider_CompleteStream_CancelledContext(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content:    []ContentBlock{NewTextBlock("Hello")},
			StopReason: StopEndTurn,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := mock.CompleteStream(ctx, &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	assert.Error(t, err)
}

func TestMockProvider_CompleteStream_Exhausted(t *testing.T) {
	mock := NewMockProvider() // no responses

	_, err := mock.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no more responses")
}

func TestMockStreamEvents(t *testing.T) {
	mock := &MockStreamEvents{
		Events: []StreamEvent{
			{Type: EventTextDelta, Text: "Hello"},
			{Type: EventTextDelta, Text: " world"},
			{Type: EventContentBlockStop, Index: 0},
			{Type: EventMessageDelta, StopReason: StopEndTurn},
		},
	}

	ch, err := mock.CompleteStream(context.Background(), &Request{})
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	require.Len(t, events, 4)
	assert.Equal(t, EventTextDelta, events[0].Type)
	assert.Equal(t, "Hello", events[0].Text)
	assert.Equal(t, EventTextDelta, events[1].Type)
	assert.Equal(t, " world", events[1].Text)
}

func TestStreamingProvider_InterfaceCheck(t *testing.T) {
	// Verify that MockProvider implements StreamingProvider
	var _ StreamingProvider = (*MockProvider)(nil)
	var _ StreamingProvider = (*MockStreamEvents)(nil)
}

func TestRetryProvider_CompleteStream_Passthrough(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content:    []ContentBlock{NewTextBlock("streamed")},
			StopReason: StopEndTurn,
			Usage:      Usage{InputTokens: 5, OutputTokens: 3},
		},
	)

	retry := NewRetryProvider(mock, DefaultRetryConfig())
	ch, err := retry.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	require.True(t, len(events) > 0)
	assert.Equal(t, EventTextDelta, events[0].Type)
	assert.Equal(t, "streamed", events[0].Text)
}

func TestDialectFixer_CompleteStream_Passthrough(t *testing.T) {
	mock := NewMockProvider(
		&Response{
			Content:    []ContentBlock{NewTextBlock("fixed")},
			StopReason: StopEndTurn,
		},
	)

	fixer := NewDialectFixer(mock)
	ch, err := fixer.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	require.NoError(t, err)

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}

	require.True(t, len(events) > 0)
	assert.Equal(t, EventTextDelta, events[0].Type)
}

func TestDialectFixer_CompleteStream_NonStreamingInner(t *testing.T) {
	// dialectErrorProvider does not implement StreamingProvider
	inner := &dialectErrorProvider{err: nil}
	fixer := NewDialectFixer(inner)
	_, err := fixer.CompleteStream(context.Background(), &Request{
		Model:     "test",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not support streaming")
}
