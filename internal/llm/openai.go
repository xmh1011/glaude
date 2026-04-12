package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/shared"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// OpenAIProvider implements Provider using the OpenAI Chat Completions API
// via the official openai-go SDK. It can also be used with any
// OpenAI-compatible endpoint (e.g., Ollama) by setting a custom base URL.
type OpenAIProvider struct {
	client  openai.Client
	baseURL string // stored for logging/diagnostics only
}

// NewOpenAIProvider creates a provider for OpenAI-compatible APIs.
// If baseURL is empty, the SDK defaults to https://api.openai.com/v1/.
// If apiKey is empty, the SDK reads OPENAI_API_KEY from the environment.
func NewOpenAIProvider(apiKey, baseURL string) *OpenAIProvider {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &OpenAIProvider{
		client:  openai.NewClient(opts...),
		baseURL: baseURL,
	}
}

// Complete sends a chat completion request to the OpenAI-compatible API.
func (p *OpenAIProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	msgs := p.buildMessages(req)
	tools := p.buildTools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:               req.Model,
		Messages:            msgs,
		MaxCompletionTokens: openai.Int(int64(req.MaxTokens)),
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	telemetry.Log.
		WithField("model", req.Model).
		WithField("base_url", p.baseURL).
		WithField("msg_count", len(msgs)).
		Debug("openai: sending request")

	resp, err := p.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai API: %w", err)
	}

	return p.fromChatCompletion(resp), nil
}

// CompleteStream starts a streaming completion and delivers events via a channel.
func (p *OpenAIProvider) CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	msgs := p.buildMessages(req)
	tools := p.buildTools(req.Tools)

	params := openai.ChatCompletionNewParams{
		Model:               req.Model,
		Messages:            msgs,
		MaxCompletionTokens: openai.Int(int64(req.MaxTokens)),
	}
	if len(tools) > 0 {
		params.Tools = tools
	}

	telemetry.Log.
		WithField("model", req.Model).
		WithField("base_url", p.baseURL).
		WithField("msg_count", len(msgs)).
		Debug("openai: starting stream")

	stream := p.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		defer stream.Close()

		acc := openai.ChatCompletionAccumulator{}
		// Track which tool call indices we've already seen start for
		seenToolStarts := map[int64]bool{}

		for stream.Next() {
			chunk := stream.Current()
			acc.AddChunk(chunk)

			if len(chunk.Choices) == 0 {
				continue
			}
			choice := chunk.Choices[0]

			// Text delta
			if choice.Delta.Content != "" {
				ch <- StreamEvent{
					Type: EventTextDelta,
					Text: choice.Delta.Content,
				}
			}

			// Tool call deltas
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				if !seenToolStarts[idx] && (tc.ID != "" || tc.Function.Name != "") {
					seenToolStarts[idx] = true
					ch <- StreamEvent{
						Type:  EventToolUseStart,
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Index: int(idx),
					}
				}
				if tc.Function.Arguments != "" {
					ch <- StreamEvent{
						Type:      EventInputJSONDelta,
						InputJSON: tc.Function.Arguments,
						Index:     int(idx),
					}
				}
			}

			// Check if content just finished
			if _, ok := acc.JustFinishedContent(); ok {
				ch <- StreamEvent{
					Type:  EventContentBlockStop,
					Index: 0,
				}
			}

			// Check if a tool call just finished
			if tc, ok := acc.JustFinishedToolCall(); ok {
				ch <- StreamEvent{
					Type:  EventContentBlockStop,
					Index: tc.Index,
				}
			}

			// Finish reason
			if choice.FinishReason != "" {
				ch <- StreamEvent{
					Type:       EventMessageDelta,
					StopReason: mapFinishReason(string(choice.FinishReason)),
					Usage: Usage{
						InputTokens:  int(chunk.Usage.PromptTokens),
						OutputTokens: int(chunk.Usage.CompletionTokens),
					},
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Errorf("openai stream: %w", err),
			}
		}
	}()

	return ch, nil
}

// buildMessages converts our unified messages to OpenAI SDK message params.
func (p *OpenAIProvider) buildMessages(req *Request) []openai.ChatCompletionMessageParamUnion {
	var msgs []openai.ChatCompletionMessageParamUnion

	// System prompt
	if req.System != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}

	// Conversation messages
	for _, m := range req.Messages {
		msgs = append(msgs, toOpenAIMessages(m)...)
	}
	return msgs
}

// toOpenAIMessages converts a unified Message to one or more OpenAI SDK messages.
func toOpenAIMessages(m Message) []openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case RoleAssistant:
		return toOpenAIAssistantMessage(m)
	case RoleUser:
		return toOpenAIUserOrToolMessages(m)
	default:
		return nil
	}
}

// toOpenAIAssistantMessage converts an assistant message, preserving tool_calls.
func toOpenAIAssistantMessage(m Message) []openai.ChatCompletionMessageParamUnion {
	var textContent string
	var toolCalls []openai.ChatCompletionMessageToolCallParam

	for _, cb := range m.Content {
		switch cb.Type {
		case ContentText:
			if textContent != "" {
				textContent += "\n"
			}
			textContent += cb.Text
		case ContentToolUse:
			args := string(cb.Input)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, openai.ChatCompletionMessageToolCallParam{
				ID: cb.ID,
				Function: openai.ChatCompletionMessageToolCallFunctionParam{
					Name:      cb.Name,
					Arguments: args,
				},
			})
		}
	}

	msg := openai.ChatCompletionMessageParamUnion{
		OfAssistant: &openai.ChatCompletionAssistantMessageParam{},
	}
	if textContent != "" {
		msg.OfAssistant.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
			OfString: openai.String(textContent),
		}
	}
	if len(toolCalls) > 0 {
		msg.OfAssistant.ToolCalls = toolCalls
	}
	return []openai.ChatCompletionMessageParamUnion{msg}
}

// toOpenAIUserOrToolMessages converts a user message. If it contains tool results,
// each becomes a separate "tool" role message.
func toOpenAIUserOrToolMessages(m Message) []openai.ChatCompletionMessageParamUnion {
	var toolResults []openai.ChatCompletionMessageParamUnion
	var textParts []string

	for _, cb := range m.Content {
		switch cb.Type {
		case ContentToolResult:
			toolResults = append(toolResults, openai.ToolMessage(cb.Content, cb.ToolUseID))
		case ContentText:
			textParts = append(textParts, cb.Text)
		}
	}

	if len(toolResults) > 0 {
		return toolResults
	}

	combined := ""
	for i, p := range textParts {
		if i > 0 {
			combined += "\n"
		}
		combined += p
	}
	return []openai.ChatCompletionMessageParamUnion{openai.UserMessage(combined)}
}

// buildTools converts unified tool definitions to OpenAI SDK format.
func (p *OpenAIProvider) buildTools(tools []ToolDefinition) []openai.ChatCompletionToolParam {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openai.ChatCompletionToolParam, 0, len(tools))
	for _, t := range tools {
		// Parse InputSchema into FunctionParameters (map[string]any)
		var params shared.FunctionParameters
		if len(t.InputSchema) > 0 {
			_ = json.Unmarshal(t.InputSchema, &params)
		}
		out = append(out, openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

// fromChatCompletion converts an OpenAI SDK response to our unified Response.
func (p *OpenAIProvider) fromChatCompletion(resp *openai.ChatCompletion) *Response {
	if len(resp.Choices) == 0 {
		return &Response{ID: resp.ID, StopReason: StopEndTurn}
	}

	choice := resp.Choices[0]
	var blocks []ContentBlock

	// Text content
	if choice.Message.Content != "" {
		blocks = append(blocks, NewTextBlock(choice.Message.Content))
	}

	// Tool calls
	for _, tc := range choice.Message.ToolCalls {
		blocks = append(blocks, ContentBlock{
			Type:  ContentToolUse,
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}

	stopReason := mapFinishReason(string(choice.FinishReason))

	telemetry.Log.
		WithField("prompt_tokens", resp.Usage.PromptTokens).
		WithField("completion_tokens", resp.Usage.CompletionTokens).
		WithField("finish_reason", choice.FinishReason).
		Debug("openai: response received")

	return &Response{
		ID:         resp.ID,
		Content:    blocks,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  int(resp.Usage.PromptTokens),
			OutputTokens: int(resp.Usage.CompletionTokens),
		},
	}
}

// mapFinishReason converts OpenAI finish_reason to our StopReason.
func mapFinishReason(reason string) StopReason {
	switch reason {
	case "stop":
		return StopEndTurn
	case "tool_calls":
		return StopToolUse
	case "length":
		return StopMaxTokens
	default:
		return StopReason(reason)
	}
}
