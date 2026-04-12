// Package llm defines the provider-agnostic message model for LLM communication.
//
// All types here form an anti-corruption layer (ACL) that shields the rest
// of glaude from vendor-specific API differences (Anthropic, OpenAI, Ollama).
package llm

import "encoding/json"

// Role represents the message sender.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentType identifies the kind of content block.
type ContentType string

const (
	ContentText       ContentType = "text"
	ContentToolUse    ContentType = "tool_use"
	ContentToolResult ContentType = "tool_result"
)

// ContentBlock is a unified representation of message content.
// Only the fields relevant to the block's Type are populated.
type ContentBlock struct {
	Type ContentType `json:"type"`

	// Text block fields
	Text string `json:"text,omitempty"`

	// ToolUse block fields
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// ToolResult block fields
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// NewTextBlock creates a text content block.
func NewTextBlock(text string) ContentBlock {
	return ContentBlock{Type: ContentText, Text: text}
}

// NewToolResultBlock creates a tool result content block.
func NewToolResultBlock(toolUseID, content string, isError bool) ContentBlock {
	return ContentBlock{
		Type:      ContentToolResult,
		ToolUseID: toolUseID,
		Content:   content,
		IsError:   isError,
	}
}

// Message represents a single message in the conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
)

// Usage tracks token consumption for a single API call.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Request encapsulates a provider-agnostic completion request.
type Request struct {
	Model     string
	System    string
	Messages  []Message
	MaxTokens int
	// Tools will be added in Phase 2.
}

// Response encapsulates a provider-agnostic completion response.
type Response struct {
	ID         string
	Content    []ContentBlock
	StopReason StopReason
	Usage      Usage
}

// TextContent extracts and concatenates all text blocks from the response.
func (r *Response) TextContent() string {
	var result string
	for _, b := range r.Content {
		if b.Type == ContentText {
			if result != "" {
				result += "\n"
			}
			result += b.Text
		}
	}
	return result
}

// ToolUseBlocks returns all tool_use blocks from the response.
func (r *Response) ToolUseBlocks() []ContentBlock {
	var blocks []ContentBlock
	for _, b := range r.Content {
		if b.Type == ContentToolUse {
			blocks = append(blocks, b)
		}
	}
	return blocks
}
