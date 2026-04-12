package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/hook"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/tool"
	"github.com/xmh1011/glaude/internal/tool/fileread"
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
	reg.Register(&fileread.Tool{})

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

func TestRunStream_EndTurn(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("Hello, stream!")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 10, OutputTokens: 5},
		},
	)

	a := New(mock, "mock-model", "You are helpful.", nil)

	var deltas []string
	cb := func(event llm.StreamEvent) {
		if event.Type == llm.EventTextDelta {
			deltas = append(deltas, event.Text)
		}
	}

	text, err := a.RunStream(context.Background(), "Hi", cb)
	require.NoError(t, err)
	assert.Equal(t, "Hello, stream!", text)

	// Verify callback was called with text deltas
	require.Len(t, deltas, 1)
	assert.Equal(t, "Hello, stream!", deltas[0])

	// Verify usage tracking
	usage := a.TotalUsage()
	assert.Equal(t, 10, usage.InputTokens)
	assert.Equal(t, 5, usage.OutputTokens)
}

func TestRunStream_ToolUse(t *testing.T) {
	// Create a temp file to read via FileReadTool
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "stream.txt")
	os.WriteFile(testFile, []byte("streamed file content\n"), 0644)

	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Let me read that."),
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
			Content:    []llm.ContentBlock{llm.NewTextBlock("File contains: streamed file content")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 60, OutputTokens: 12},
		},
	)

	reg := tool.NewRegistry()
	reg.Register(&fileread.Tool{})

	a := New(mock, "mock-model", "test", reg)

	var toolStarts []string
	cb := func(event llm.StreamEvent) {
		if event.Type == llm.EventToolUseStart {
			toolStarts = append(toolStarts, event.Name)
		}
	}

	text, err := a.RunStream(context.Background(), "read the file", cb)
	require.NoError(t, err)
	assert.Contains(t, text, "streamed file content")

	// Verify tool start callback was called
	require.Len(t, toolStarts, 1)
	assert.Equal(t, "Read", toolStarts[0])
}

func TestRunStream_NilCallback(t *testing.T) {
	mock := llm.NewMockProvider(
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("OK")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 5, OutputTokens: 1},
		},
	)

	a := New(mock, "mock-model", "test", nil)
	text, err := a.RunStream(context.Background(), "Hi", nil)
	require.NoError(t, err)
	assert.Equal(t, "OK", text)
}

// ---------------------------------------------------------------------------
// Hook integration tests
// ---------------------------------------------------------------------------

func TestExecuteTool_HookDeny(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}

	// Setup: LLM calls Bash tool, but PreToolUse hook denies it.
	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Let me run a command."),
				{
					Type:  llm.ContentToolUse,
					ID:    "tool_01",
					Name:  "Read",
					Input: json.RawMessage(`{"file_path":"/tmp/nonexistent"}`),
				},
			},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{InputTokens: 20, OutputTokens: 15},
		},
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("OK, denied.")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 50, OutputTokens: 10},
		},
	)

	reg := tool.NewRegistry()
	reg.Register(&fileread.Tool{})

	hookEngine := &hook.Engine{}
	hookEngine.SetConfig(hook.HookConfig{
		hook.PreToolUse: {
			{
				Matcher: "*",
				Hooks: []hook.HookEntry{
					{Type: "command", Command: `echo '{"decision":"deny","message":"not allowed"}'`},
				},
			},
		},
	})

	a := New(mock, "mock-model", "test", reg)
	a.SetHookEngine(hookEngine)

	text, err := a.Run(context.Background(), "read a file")
	require.NoError(t, err)
	assert.Equal(t, "OK, denied.", text)

	// The tool result message should contain the denial.
	toolResult := a.Messages()[2]
	assert.True(t, toolResult.Content[0].IsError)
	assert.Contains(t, toolResult.Content[0].Content, "denied")
}

func TestExecuteTool_HookAllow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}

	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "hook-allow.txt")
	os.WriteFile(testFile, []byte("hook allowed content\n"), 0644)

	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Reading file."),
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
			Content:    []llm.ContentBlock{llm.NewTextBlock("Got it.")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 60, OutputTokens: 12},
		},
	)

	reg := tool.NewRegistry()
	reg.Register(&fileread.Tool{})

	hookEngine := &hook.Engine{}
	hookEngine.SetConfig(hook.HookConfig{
		hook.PreToolUse: {
			{
				Matcher: "Read",
				Hooks: []hook.HookEntry{
					{Type: "command", Command: `echo '{"decision":"allow"}'`},
				},
			},
		},
	})

	a := New(mock, "mock-model", "test", reg)
	a.SetHookEngine(hookEngine)

	text, err := a.Run(context.Background(), "read the file")
	require.NoError(t, err)
	assert.Equal(t, "Got it.", text)

	// Verify the tool executed successfully (not denied).
	toolResult := a.Messages()[2]
	assert.False(t, toolResult.Content[0].IsError)
	assert.Contains(t, toolResult.Content[0].Content, "hook allowed content")
}

func TestExecuteTool_HookBlockingError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}

	mock := llm.NewMockProvider(
		&llm.Response{
			Content: []llm.ContentBlock{
				llm.NewTextBlock("Trying."),
				{
					Type:  llm.ContentToolUse,
					ID:    "tool_01",
					Name:  "Read",
					Input: json.RawMessage(`{"file_path":"/tmp/x"}`),
				},
			},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{InputTokens: 20, OutputTokens: 15},
		},
		&llm.Response{
			Content:    []llm.ContentBlock{llm.NewTextBlock("Blocked.")},
			StopReason: llm.StopEndTurn,
			Usage:      llm.Usage{InputTokens: 50, OutputTokens: 10},
		},
	)

	reg := tool.NewRegistry()
	reg.Register(&fileread.Tool{})

	hookEngine := &hook.Engine{}
	hookEngine.SetConfig(hook.HookConfig{
		hook.PreToolUse: {
			{
				Matcher: "*",
				Hooks: []hook.HookEntry{
					{Type: "command", Command: `echo "security violation" >&2; exit 2`},
				},
			},
		},
	})

	a := New(mock, "mock-model", "test", reg)
	a.SetHookEngine(hookEngine)

	text, err := a.Run(context.Background(), "try it")
	require.NoError(t, err)
	assert.Equal(t, "Blocked.", text)

	toolResult := a.Messages()[2]
	assert.True(t, toolResult.Content[0].IsError)
	assert.Contains(t, toolResult.Content[0].Content, "denied by hook")
}

func TestAgent_SetHookEngine_Nil(t *testing.T) {
	a := New(llm.NewMockProvider(), "model", "sys", nil)
	a.SetHookEngine(nil)
	assert.Nil(t, a.HookEngine())
}
