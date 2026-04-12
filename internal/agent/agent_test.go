package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"glaude/internal/llm"
	"glaude/internal/tool"
)

func TestRun_EndTurn(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("Hello, world!")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	a := New(mock, "mock-model", "You are helpful.", nil)
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

func TestRun_ToolUseNoRegistry(t *testing.T) {
	// Without a registry, tool_use returns an error result
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
			Content:    []llm.ContentBlock{llm.NewTextBlock("No tools available.")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 50, OutputTokens: 10},
		},
	)

	a := New(mock, "mock-model", "test", nil)
	text, err := a.Run(context.Background(), "list files")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "No tools available." {
		t.Fatalf("expected final text, got %q", text)
	}

	// Third message should be tool result with error
	toolResult := a.Messages()[2]
	if !toolResult.Content[0].IsError {
		t.Fatal("tool result should be marked as error when no registry")
	}
}

func TestRun_ToolUseWithRegistry(t *testing.T) {
	// Create a temp file to read via FileReadTool
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "hello.txt")
	os.WriteFile(testFile, []byte("hello from file\n"), 0644)

	// LLM calls Read tool -> gets file content -> responds with final text
	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Let me read that file."),
				{
					Type:  llm.ContentToolUse,
					ID:    "tool_01",
					Name:  "Read",
					Input: json.RawMessage(`{"file_path":"` + testFile + `"}`),
				},
			},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{InputTokens: 20, OutputTokens: 15},
		},
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("The file contains: hello from file")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 60, OutputTokens: 12},
		},
	)

	reg := tool.NewRegistry()
	reg.Register(&tool.FileReadTool{})

	a := New(mock, "mock-model", "test", reg)
	text, err := a.Run(context.Background(), "read the file")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "hello from file") {
		t.Fatalf("expected final text to reference file content, got %q", text)
	}

	// Verify tool result is NOT an error
	toolResult := a.Messages()[2]
	if toolResult.Content[0].IsError {
		t.Fatalf("tool result should NOT be error, content: %s", toolResult.Content[0].Content)
	}
	if !strings.Contains(toolResult.Content[0].Content, "hello from file") {
		t.Fatalf("tool result should contain file content, got: %s", toolResult.Content[0].Content)
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

	a := New(mock, "mock-model", "test", nil)
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

	a := New(mock, "mock-model", "test", nil)
	_, err := a.Run(ctx, "hello")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("expected canceled error, got: %v", err)
	}
}
