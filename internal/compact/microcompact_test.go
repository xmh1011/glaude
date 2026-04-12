package compact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"glaude/internal/llm"
)

func TestMicroCompact_ClearsOldResults(t *testing.T) {
	// Create messages with 8 tool use/result pairs (Read tool)
	messages := make([]llm.Message, 0)
	for i := 0; i < 8; i++ {
		toolID := string(rune('a' + i))
		// Assistant uses a tool
		messages = append(messages, llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{{
				Type:  llm.ContentToolUse,
				ID:    toolID,
				Name:  "Read",
				Input: json.RawMessage(`{"file_path":"/tmp/test.go"}`),
			}},
		})
		// User returns tool result
		messages = append(messages, llm.Message{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				llm.NewToolResultBlock(toolID, "file content "+toolID, false),
			},
		})
	}

	result := MicroCompact(messages)
	require.Len(t, result, len(messages))

	// Count cleared vs preserved results
	cleared := 0
	preserved := 0
	for _, msg := range result {
		for _, block := range msg.Content {
			if block.Type == llm.ContentToolResult {
				if block.Content == ClearedPlaceholder {
					cleared++
				} else {
					preserved++
				}
			}
		}
	}

	// Should clear 8 - 5 = 3 oldest results
	assert.Equal(t, 3, cleared)
	assert.Equal(t, PreserveRecentResults, preserved)
}

func TestMicroCompact_PreservesNonCompactableTools(t *testing.T) {
	messages := []llm.Message{
		{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{{
				Type:  llm.ContentToolUse,
				ID:    "agent-1",
				Name:  "AgentTool", // not compactable
				Input: json.RawMessage(`{}`),
			}},
		},
		{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				llm.NewToolResultBlock("agent-1", "agent result that should not be cleared", false),
			},
		},
	}

	result := MicroCompact(messages)
	assert.Equal(t, "agent result that should not be cleared", result[1].Content[0].Content)
}

func TestMicroCompact_TruncatesOversized(t *testing.T) {
	bigContent := strings.Repeat("x", MaxToolResultBytes+1000)
	messages := []llm.Message{
		{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{{
				Type:  llm.ContentToolUse,
				ID:    "big-1",
				Name:  "Bash",
				Input: json.RawMessage(`{"command":"cat huge.log"}`),
			}},
		},
		{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				llm.NewToolResultBlock("big-1", bigContent, false),
			},
		},
	}

	result := MicroCompact(messages)
	content := result[1].Content[0].Content
	assert.Less(t, len(content), len(bigContent))
	assert.True(t, strings.HasSuffix(content, TruncatedSuffix))
}

func TestMicroCompact_NoopWhenFewResults(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("hi")}},
	}

	result := MicroCompact(messages)
	assert.Equal(t, messages, result, "should return same slice when nothing to compact")
}

func TestMicroCompact_PreservesToolUseBlocks(t *testing.T) {
	messages := make([]llm.Message, 0)
	for i := 0; i < 8; i++ {
		toolID := string(rune('a' + i))
		messages = append(messages, llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{{
				Type:  llm.ContentToolUse,
				ID:    toolID,
				Name:  "Read",
				Input: json.RawMessage(`{"file_path":"/test"}`),
			}},
		})
		messages = append(messages, llm.Message{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				llm.NewToolResultBlock(toolID, "content", false),
			},
		})
	}

	result := MicroCompact(messages)

	// All tool_use blocks should be untouched
	for i, msg := range result {
		for j, block := range msg.Content {
			if block.Type == llm.ContentToolUse {
				assert.Equal(t, messages[i].Content[j], block, "tool_use blocks must not be modified")
			}
		}
	}
}

func TestMicroCompact_PreservesTextMessages(t *testing.T) {
	messages := []llm.Message{
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("user text")}},
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("assistant text")}},
	}

	// Add tool results to trigger compaction
	for i := 0; i < 8; i++ {
		toolID := string(rune('a' + i))
		messages = append(messages, llm.Message{
			Role: llm.RoleAssistant,
			Content: []llm.ContentBlock{{
				Type: llm.ContentToolUse, ID: toolID, Name: "Grep",
				Input: json.RawMessage(`{}`),
			}},
		})
		messages = append(messages, llm.Message{
			Role: llm.RoleUser,
			Content: []llm.ContentBlock{
				llm.NewToolResultBlock(toolID, "grep result", false),
			},
		})
	}

	result := MicroCompact(messages)
	// First two messages (text) should be unchanged
	assert.Equal(t, "user text", result[0].Content[0].Text)
	assert.Equal(t, "assistant text", result[1].Content[0].Text)
}
