package llm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestBuildToolNameMap(t *testing.T) {
	msgs := []Message{
		{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: ContentToolUse, ID: "call_1", Name: "Bash"},
				{Type: ContentToolUse, ID: "call_2", Name: "Read"},
				{Type: ContentText, Text: "hello"},
			},
		},
		{
			Role: RoleUser,
			Content: []ContentBlock{
				{Type: ContentToolResult, ToolUseID: "call_1", Content: "ok"},
			},
		},
	}

	m := buildToolNameMap(msgs)
	assert.Equal(t, "Bash", m["call_1"])
	assert.Equal(t, "Read", m["call_2"])
	assert.Empty(t, m["nonexistent"])
}

func TestBuildToolNameMap_Empty(t *testing.T) {
	m := buildToolNameMap(nil)
	assert.Empty(t, m)
}

func TestToGeminiAssistantContent_Text(t *testing.T) {
	m := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			NewTextBlock("hello world"),
		},
	}
	c := toGeminiAssistantContent(m)
	require.NotNil(t, c)
	assert.Equal(t, "model", c.Role)
	require.Len(t, c.Parts, 1)
	assert.Equal(t, "hello world", c.Parts[0].Text)
}

func TestToGeminiAssistantContent_FunctionCall(t *testing.T) {
	m := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			{
				Type:  ContentToolUse,
				ID:    "call_1",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls"}`),
			},
		},
	}
	c := toGeminiAssistantContent(m)
	require.NotNil(t, c)
	assert.Equal(t, "model", c.Role)
	require.Len(t, c.Parts, 1)
	assert.NotNil(t, c.Parts[0].FunctionCall)
	assert.Equal(t, "Bash", c.Parts[0].FunctionCall.Name)
	assert.Equal(t, "ls", c.Parts[0].FunctionCall.Args["command"])
}

func TestToGeminiAssistantContent_Empty(t *testing.T) {
	m := Message{Role: RoleAssistant, Content: nil}
	c := toGeminiAssistantContent(m)
	assert.Nil(t, c)
}

func TestToGeminiUserContents_Text(t *testing.T) {
	m := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			NewTextBlock("hi there"),
		},
	}
	nameMap := map[string]string{}
	contents := toGeminiUserContents(m, nameMap)
	require.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0].Role)
	require.Len(t, contents[0].Parts, 1)
	assert.Equal(t, "hi there", contents[0].Parts[0].Text)
}

func TestToGeminiUserContents_ToolResult(t *testing.T) {
	m := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentToolResult, ToolUseID: "call_1", Content: "file list"},
		},
	}
	nameMap := map[string]string{"call_1": "Bash"}
	contents := toGeminiUserContents(m, nameMap)
	require.Len(t, contents, 1)
	assert.Equal(t, "user", contents[0].Role)
	require.Len(t, contents[0].Parts, 1)

	fr := contents[0].Parts[0].FunctionResponse
	require.NotNil(t, fr)
	assert.Equal(t, "Bash", fr.Name)
	assert.Equal(t, "file list", fr.Response["output"])
}

func TestToGeminiUserContents_ToolResultError(t *testing.T) {
	m := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentToolResult, ToolUseID: "call_1", Content: "permission denied", IsError: true},
		},
	}
	nameMap := map[string]string{"call_1": "Bash"}
	contents := toGeminiUserContents(m, nameMap)
	require.Len(t, contents, 1)

	fr := contents[0].Parts[0].FunctionResponse
	require.NotNil(t, fr)
	assert.Equal(t, "permission denied", fr.Response["error"])
	assert.Nil(t, fr.Response["output"])
}

func TestToGeminiUserContents_UnknownToolName(t *testing.T) {
	m := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentToolResult, ToolUseID: "missing_id", Content: "ok"},
		},
	}
	nameMap := map[string]string{}
	contents := toGeminiUserContents(m, nameMap)
	require.Len(t, contents, 1)

	fr := contents[0].Parts[0].FunctionResponse
	assert.Equal(t, "unknown", fr.Name)
}

func TestToGeminiTools(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "Bash",
			Description: "Execute a command",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`),
		},
		{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		},
	}

	result := toGeminiTools(tools)
	require.Len(t, result, 1)
	require.Len(t, result[0].FunctionDeclarations, 2)
	assert.Equal(t, "Bash", result[0].FunctionDeclarations[0].Name)
	assert.Equal(t, "Execute a command", result[0].FunctionDeclarations[0].Description)
	assert.NotNil(t, result[0].FunctionDeclarations[0].ParametersJsonSchema)
	assert.Equal(t, "Read", result[0].FunctionDeclarations[1].Name)
}

func TestToGeminiTools_Empty(t *testing.T) {
	assert.Nil(t, toGeminiTools(nil))
}

func TestFromGeminiResponse_TextOnly(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		ResponseID: "resp-1",
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: "Hello!"}},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	result := fromGeminiResponse(resp, nil)
	assert.Equal(t, "resp-1", result.ID)
	assert.Equal(t, StopEndTurn, result.StopReason)
	require.Len(t, result.Content, 1)
	assert.Equal(t, ContentText, result.Content[0].Type)
	assert.Equal(t, "Hello!", result.Content[0].Text)
	assert.Equal(t, 10, result.Usage.InputTokens)
	assert.Equal(t, 5, result.Usage.OutputTokens)
}

func TestFromGeminiResponse_FunctionCall(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Role: "model",
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								Name: "Bash",
								Args: map[string]any{"command": "pwd"},
							},
						},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result := fromGeminiResponse(resp, nil)
	assert.Equal(t, StopToolUse, result.StopReason)
	require.Len(t, result.Content, 1)
	assert.Equal(t, ContentToolUse, result.Content[0].Type)
	assert.Equal(t, "Bash", result.Content[0].Name)
	assert.Contains(t, string(result.Content[0].Input), "pwd")
	// Synthetic ID should be generated
	assert.Contains(t, result.Content[0].ID, "gemini_call_")
}

func TestFromGeminiResponse_NilResponse(t *testing.T) {
	result := fromGeminiResponse(nil, nil)
	assert.Equal(t, StopEndTurn, result.StopReason)
	assert.Empty(t, result.Content)
}

func TestFromGeminiResponse_NoCandidates(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		ResponseID: "resp-empty",
	}
	result := fromGeminiResponse(resp, nil)
	assert.Equal(t, StopEndTurn, result.StopReason)
}

func TestMapGeminiFinishReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   genai.FinishReason
		blocks   []ContentBlock
		expected StopReason
	}{
		{
			name:     "stop",
			reason:   genai.FinishReasonStop,
			expected: StopEndTurn,
		},
		{
			name:     "max_tokens",
			reason:   genai.FinishReasonMaxTokens,
			expected: StopMaxTokens,
		},
		{
			name:     "unspecified defaults to end_turn",
			reason:   genai.FinishReasonUnspecified,
			expected: StopEndTurn,
		},
		{
			name:     "safety defaults to end_turn",
			reason:   genai.FinishReasonSafety,
			expected: StopEndTurn,
		},
		{
			name:   "tool_use inferred from blocks",
			reason: genai.FinishReasonStop,
			blocks: []ContentBlock{
				{Type: ContentToolUse, Name: "Bash"},
			},
			expected: StopToolUse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapGeminiFinishReason(tt.reason, tt.blocks)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestArgsToMap(t *testing.T) {
	t.Run("valid json", func(t *testing.T) {
		m := argsToMap(json.RawMessage(`{"key":"value"}`))
		assert.Equal(t, "value", m["key"])
	})

	t.Run("empty", func(t *testing.T) {
		m := argsToMap(nil)
		assert.Nil(t, m)
	})

	t.Run("invalid json", func(t *testing.T) {
		m := argsToMap(json.RawMessage(`not json`))
		assert.Nil(t, m)
	})
}

func TestExtractGeminiUsage(t *testing.T) {
	t.Run("with metadata", func(t *testing.T) {
		meta := &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 50,
		}
		u := extractGeminiUsage(meta)
		assert.Equal(t, 100, u.InputTokens)
		assert.Equal(t, 50, u.OutputTokens)
	})

	t.Run("nil metadata", func(t *testing.T) {
		u := extractGeminiUsage(nil)
		assert.Equal(t, 0, u.InputTokens)
		assert.Equal(t, 0, u.OutputTokens)
	})
}

func TestBuildGeminiConfig(t *testing.T) {
	tools := []ToolDefinition{
		{Name: "Bash", Description: "run cmd", InputSchema: json.RawMessage(`{"type":"object"}`)},
	}
	req := &Request{
		Model:     "gemini-2.5-flash",
		System:    "You are helpful.",
		MaxTokens: 4096,
		Tools:     tools,
	}

	cfg := buildGeminiConfig(req)
	assert.Equal(t, int32(4096), cfg.MaxOutputTokens)
	assert.NotNil(t, cfg.SystemInstruction)
	assert.Len(t, cfg.Tools, 1)
}

func TestBuildGeminiConfig_NoSystem(t *testing.T) {
	req := &Request{
		Model:     "gemini-2.5-flash",
		MaxTokens: 1024,
	}

	cfg := buildGeminiConfig(req)
	assert.Nil(t, cfg.SystemInstruction)
	assert.Nil(t, cfg.Tools)
}
