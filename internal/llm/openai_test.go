package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProvider_Complete_TextResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "chat/completions")
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer test-key")

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-123",
			"choices": []map[string]interface{}{{
				"message":       map[string]interface{}{"role": "assistant", "content": "Hello!"},
				"finish_reason": "stop",
				"index":         0,
			}},
			"usage":   map[string]interface{}{"prompt_tokens": 20, "completion_tokens": 5, "total_tokens": 25},
			"model":   "gpt-4",
			"object":  "chat.completion",
			"created": 1234567890,
		})
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL)
	resp, err := p.Complete(context.Background(), &Request{
		Model:     "gpt-4",
		System:    "You are helpful.",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})

	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, StopEndTurn, resp.StopReason)
	assert.Equal(t, 20, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "Hello!", resp.Content[0].Text)
}

func TestOpenAIProvider_Complete_ToolCallResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "chatcmpl-456",
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"role": "assistant",
					"tool_calls": []map[string]interface{}{{
						"id":   "call_1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "Read",
							"arguments": `{"file_path":"/tmp/test.txt"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
				"index":         0,
			}},
			"usage":   map[string]interface{}{"prompt_tokens": 30, "completion_tokens": 15, "total_tokens": 45},
			"model":   "gpt-4",
			"object":  "chat.completion",
			"created": 1234567890,
		})
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL)
	resp, err := p.Complete(context.Background(), &Request{
		Model:     "gpt-4",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("read file")}}},
		MaxTokens: 1024,
		Tools: []ToolDefinition{{
			Name:        "Read",
			Description: "Read a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"file_path":{"type":"string"}}}`),
		}},
	})

	require.NoError(t, err)
	assert.Equal(t, StopToolUse, resp.StopReason)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, ContentToolUse, resp.Content[0].Type)
	assert.Equal(t, "call_1", resp.Content[0].ID)
	assert.Equal(t, "Read", resp.Content[0].Name)
}

func TestOpenAIProvider_Complete_ErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProvider("test-key", server.URL)
	_, err := p.Complete(context.Background(), &Request{
		Model:     "gpt-4",
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{NewTextBlock("Hi")}}},
		MaxTokens: 1024,
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestToOpenAIMessages_ToolResult(t *testing.T) {
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			NewToolResultBlock("call_1", "file contents", false),
			NewToolResultBlock("call_2", "error", true),
		},
	}

	msgs := toOpenAIMessages(msg)
	require.Len(t, msgs, 2)
	// Both should be OfTool messages
	assert.NotNil(t, msgs[0].OfTool)
	assert.Equal(t, "call_1", msgs[0].OfTool.ToolCallID)
	assert.NotNil(t, msgs[1].OfTool)
	assert.Equal(t, "call_2", msgs[1].OfTool.ToolCallID)
}

func TestToOpenAIMessages_AssistantWithToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			NewTextBlock("Let me check..."),
			{
				Type:  ContentToolUse,
				ID:    "call_1",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"ls"}`),
			},
		},
	}

	msgs := toOpenAIMessages(msg)
	require.Len(t, msgs, 1)
	assert.NotNil(t, msgs[0].OfAssistant)
	require.Len(t, msgs[0].OfAssistant.ToolCalls, 1)
	assert.Equal(t, "Bash", msgs[0].OfAssistant.ToolCalls[0].Function.Name)
}

func TestMapFinishReason(t *testing.T) {
	assert.Equal(t, StopEndTurn, mapFinishReason("stop"))
	assert.Equal(t, StopToolUse, mapFinishReason("tool_calls"))
	assert.Equal(t, StopMaxTokens, mapFinishReason("length"))
	assert.Equal(t, StopReason("unknown"), mapFinishReason("unknown"))
}
