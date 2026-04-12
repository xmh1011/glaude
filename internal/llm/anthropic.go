package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"glaude/internal/telemetry"
)

// AnthropicProvider implements Provider using the Anthropic Messages API.
type AnthropicProvider struct {
	client anthropic.Client
}

// NewAnthropicProvider creates a provider with the given API key.
// If apiKey is empty, the SDK falls back to the ANTHROPIC_API_KEY environment variable.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &AnthropicProvider{
		client: anthropic.NewClient(opts...),
	}
}

// Complete sends a completion request to the Anthropic Messages API.
func (p *AnthropicProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	params := anthropic.MessageNewParams{
		Model:     req.Model,
		MaxTokens: int64(req.MaxTokens),
		Messages:  toAnthropicMessages(req.Messages),
	}

	if req.System != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.System},
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = toAnthropicTools(req.Tools)
	}

	telemetry.Log.
		WithField("model", req.Model).
		WithField("msg_count", len(req.Messages)).
		Debug("anthropic: sending request")

	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic API: %w", err)
	}

	telemetry.Log.
		WithField("input_tokens", msg.Usage.InputTokens).
		WithField("output_tokens", msg.Usage.OutputTokens).
		WithField("stop_reason", msg.StopReason).
		Debug("anthropic: response received")

	return fromAnthropicMessage(msg), nil
}

// toAnthropicMessages converts our unified messages to Anthropic SDK format.
func toAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	out := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toAnthropicMessageParam(m))
	}
	return out
}

func toAnthropicMessageParam(m Message) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Content))
	for _, cb := range m.Content {
		switch cb.Type {
		case ContentText:
			blocks = append(blocks, anthropic.NewTextBlock(cb.Text))
		case ContentToolResult:
			blocks = append(blocks, anthropic.NewToolResultBlock(cb.ToolUseID, cb.Content, cb.IsError))
		}
	}

	return anthropic.MessageParam{
		Role:    anthropic.MessageParamRole(m.Role),
		Content: blocks,
	}
}

// fromAnthropicMessage converts an Anthropic SDK response to our unified type.
func fromAnthropicMessage(msg *anthropic.Message) *Response {
	blocks := make([]ContentBlock, 0, len(msg.Content))
	for _, cb := range msg.Content {
		switch cb.Type {
		case "text":
			blocks = append(blocks, ContentBlock{
				Type: ContentText,
				Text: cb.Text,
			})
		case "tool_use":
			raw, _ := json.Marshal(cb.Input)
			blocks = append(blocks, ContentBlock{
				Type:  ContentToolUse,
				ID:    cb.ID,
				Name:  cb.Name,
				Input: raw,
			})
		}
	}

	return &Response{
		ID:         msg.ID,
		Content:    blocks,
		StopReason: StopReason(msg.StopReason),
		Usage: Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}
}

// toAnthropicTools converts unified tool definitions to Anthropic SDK format.
func toAnthropicTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, t := range tools {
		schema := toAnthropicInputSchema(t.InputSchema)
		out = append(out, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.Opt(t.Description),
				InputSchema: schema,
			},
		})
	}
	return out
}

// toAnthropicInputSchema converts a raw JSON Schema to ToolInputSchemaParam.
func toAnthropicInputSchema(raw json.RawMessage) anthropic.ToolInputSchemaParam {
	var parsed struct {
		Properties map[string]interface{} `json:"properties"`
		Required   []string               `json:"required"`
	}
	json.Unmarshal(raw, &parsed)

	return anthropic.ToolInputSchemaParam{
		Properties: parsed.Properties,
		Required:   parsed.Required,
	}
}
