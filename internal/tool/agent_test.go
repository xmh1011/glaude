package tool

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

// mockSubAgentProvider returns a simple text response on first call.
type mockSubAgentProvider struct {
	calls    int
	response string
}

func (m *mockSubAgentProvider) Complete(_ context.Context, req *llm.Request) (*llm.Response, error) {
	m.calls++
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock(m.response)},
		StopReason: llm.StopEndTurn,
		Usage:      llm.Usage{InputTokens: 100, OutputTokens: 50},
	}, nil
}

// mockToolUseProvider returns a tool_use on first call, then end_turn on second.
type mockToolUseProvider struct {
	calls int
}

func (m *mockToolUseProvider) Complete(_ context.Context, req *llm.Request) (*llm.Response, error) {
	m.calls++
	if m.calls == 1 {
		return &llm.Response{
			Content: []llm.ContentBlock{
				{
					Type:  llm.ContentToolUse,
					ID:    "tu_1",
					Name:  "Read",
					Input: json.RawMessage(`{"file_path": "/tmp/test.txt"}`),
				},
			},
			StopReason: llm.StopToolUse,
			Usage:      llm.Usage{InputTokens: 100, OutputTokens: 50},
		}, nil
	}
	return &llm.Response{
		Content:    []llm.ContentBlock{llm.NewTextBlock("Done reading file")},
		StopReason: llm.StopEndTurn,
		Usage:      llm.Usage{InputTokens: 200, OutputTokens: 100},
	}, nil
}

func TestAgentTool_Name(t *testing.T) {
	a := &AgentTool{}
	assert.Equal(t, "Agent", a.Name())
}

func TestAgentTool_IsReadOnly(t *testing.T) {
	a := &AgentTool{}
	assert.True(t, a.IsReadOnly())
}

func TestAgentTool_Execute_BasicConclusion(t *testing.T) {
	provider := &mockSubAgentProvider{response: "The answer is 42."}
	a := &AgentTool{
		Provider: provider,
		Model:    "test-model",
	}

	input, _ := json.Marshal(agentToolInput{
		Prompt:      "What is the meaning of life?",
		Description: "test sub-agent",
	})

	result, err := a.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "The answer is 42.", result)
	assert.Equal(t, 1, provider.calls)
}

func TestAgentTool_Execute_EmptyPrompt(t *testing.T) {
	a := &AgentTool{Provider: &mockSubAgentProvider{}}
	input, _ := json.Marshal(agentToolInput{})
	_, err := a.Execute(context.Background(), input)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt is required")
}

func TestAgentTool_Execute_WithToolUse(t *testing.T) {
	provider := &mockToolUseProvider{}
	reg := NewRegistry()
	reg.Register(&FileReadTool{})

	a := &AgentTool{
		Provider: provider,
		Model:    "test-model",
		Registry: reg,
	}

	input, _ := json.Marshal(agentToolInput{
		Prompt: "Read a file for me",
	})

	result, err := a.Execute(context.Background(), input)
	// FileReadTool will fail since /tmp/test.txt doesn't exist, but the sub-agent
	// should still complete with the second response
	require.NoError(t, err)
	assert.Equal(t, "Done reading file", result)
	assert.Equal(t, 2, provider.calls)
}

func TestAgentTool_Execute_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := &AgentTool{Provider: &mockSubAgentProvider{}}
	input, _ := json.Marshal(agentToolInput{Prompt: "test"})
	_, err := a.Execute(ctx, input)
	assert.Error(t, err)
}

func TestAgentTool_BlocksRecursion(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&AgentTool{})
	a := &AgentTool{Registry: reg}
	result, isErr := a.executeTool(context.Background(), llm.ContentBlock{
		Type:  llm.ContentToolUse,
		Name:  "Agent",
		Input: json.RawMessage(`{"prompt":"nested"}`),
	})
	assert.True(t, isErr)
	assert.Contains(t, result, "cannot spawn further sub-agents")
}

func TestAgentTool_ExcludesSelfFromToolDefs(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&FileReadTool{})
	reg.Register(&AgentTool{})

	a := &AgentTool{Registry: reg}
	defs := a.toolDefinitions()

	names := make([]string, len(defs))
	for i, d := range defs {
		names[i] = d.Name
	}
	assert.Contains(t, names, "Read")
	assert.NotContains(t, names, "Agent")
}

func TestTextFromMessages(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("world")}},
	}
	assert.Equal(t, "world", textFromMessages(msgs))
	assert.Equal(t, "", textFromMessages(nil))
}
