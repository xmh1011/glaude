package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"glaude/internal/llm"
)

func TestRun_EndTurn(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("Hello, world!")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	a := New(mock, "mock-model", "You are helpful.")
	text, err := a.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hello, world!" {
		t.Fatalf("expected %q, got %q", "Hello, world!", text)
	}

	// Verify conversation history: user + assistant
	if len(a.Messages()) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(a.Messages()))
	}
	if a.Messages()[0].Role != llm.RoleUser {
		t.Fatalf("first message should be user, got %s", a.Messages()[0].Role)
	}
	if a.Messages()[1].Role != llm.RoleAssistant {
		t.Fatalf("second message should be assistant, got %s", a.Messages()[1].Role)
	}

	// Verify usage tracking
	usage := a.TotalUsage()
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Fatalf("expected usage {10, 5}, got {%d, %d}", usage.InputTokens, usage.OutputTokens)
	}

	// Verify the request sent to provider
	if len(mock.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(mock.Requests))
	}
	if mock.Requests[0].Model != "mock-model" {
		t.Fatalf("expected model %q, got %q", "mock-model", mock.Requests[0].Model)
	}
}

func TestRun_ToolUseLoop(t *testing.T) {
	// Simulate: model wants a tool -> gets error result -> responds with text
	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Let me check..."),
				{
					Type:  llm.ContentToolUse,
					ID:    "tool_01",
					Name:  "Bash",
					Input: json.RawMessage(`{"command":"ls"}`),
				},
			},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{InputTokens: 20, OutputTokens: 15},
		},
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("Tools aren't available yet.")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 50, OutputTokens: 10},
		},
	)

	a := New(mock, "mock-model", "test")
	text, err := a.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Tools aren't available yet." {
		t.Fatalf("expected final text, got %q", text)
	}

	// Should have: user, assistant(tool_use), user(tool_result), assistant(end_turn)
	if len(a.Messages()) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(a.Messages()))
	}

	// Third message should be tool result with error
	toolResult := a.Messages()[2]
	if toolResult.Role != llm.RoleUser {
		t.Fatalf("tool result should be user role, got %s", toolResult.Role)
	}
	if toolResult.Content[0].Type != llm.ContentToolResult {
		t.Fatalf("expected tool_result, got %s", toolResult.Content[0].Type)
	}
	if !toolResult.Content[0].IsError {
		t.Fatal("tool result should be marked as error")
	}

	// Cumulative usage
	usage := a.TotalUsage()
	if usage.InputTokens != 70 || usage.OutputTokens != 25 {
		t.Fatalf("expected usage {70, 25}, got {%d, %d}", usage.InputTokens, usage.OutputTokens)
	}
}

func TestRun_MaxTokens(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("partial output...")},
			StopReason: llm.StopMaxTokens,
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 4096},
		},
	)

	a := New(mock, "mock-model", "test")
	text, err := a.Run(context.Background(), "write a long essay")
	if text != "partial output..." {
		t.Fatalf("expected partial text, got %q", text)
	}
	if err == nil || !strings.Contains(err.Error(), "max_tokens") {
		t.Fatalf("expected max_tokens error, got: %v", err)
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("should not reach")},
			StopReason: llm.StopEndTurn,
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	a := New(mock, "mock-model", "test")
	_, err := a.Run(ctx, "hello")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected canceled error, got: %v", err)
	}
}
