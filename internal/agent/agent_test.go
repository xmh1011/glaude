package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/tool"
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
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", text)

	// Verify conversation history: user + assistant
	require.Len(t, a.Messages(), 2)
	assert.Equal(t, llm.RoleUser, a.Messages()[0].Role)
	assert.Equal(t, llm.RoleAssistant, a.Messages()[1].Role)

	// Verify usage tracking
	usage := a.TotalUsage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)

	// Verify the request sent to provider
	require.Len(t, mock.Requests, 1)
	assert.Equal(t, "mock-model", mock.Requests[0].Model)
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
	require.NoError(t, err)
	assert.Equal(t, "No tools available.", text)

	// Third message should be tool result with error
	toolResult := a.Messages()[2]
	assert.True(t, toolResult.Content[0].IsError, "tool result should be marked as error when no registry")
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
	require.NoError(t, err)
	assert.Contains(t, text, "hello from file")

	// Verify tool result is NOT an error
	toolResult := a.Messages()[2]
	assert.False(t, toolResult.Content[0].IsError, "tool result should NOT be error")
	assert.Contains(t, toolResult.Content[0].Content, "hello from file")
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
	assert.Equal(t, "partial output...", text)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max_tokens")
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
}
