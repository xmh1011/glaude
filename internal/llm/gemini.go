package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/genai"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// GeminiProvider implements Provider and StreamingProvider using the Google
// Gemini API via the official genai SDK.
type GeminiProvider struct {
	client *genai.Client
}

// NewGeminiProvider creates a provider for Google Gemini models.
// If apiKey is empty, the SDK reads GEMINI_API_KEY from the environment.
func NewGeminiProvider(ctx context.Context, apiKey string) (*GeminiProvider, error) {
	cfg := &genai.ClientConfig{
		Backend: genai.BackendGeminiAPI,
	}
	if apiKey != "" {
		cfg.APIKey = apiKey
	}
	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("genai.NewClient: %w", err)
	}
	return &GeminiProvider{client: client}, nil
}

// Complete sends a non-streaming completion request to the Gemini API.
func (p *GeminiProvider) Complete(ctx context.Context, req *Request) (*Response, error) {
	contents, nameMap := toGeminiContents(req.Messages)
	config := buildGeminiConfig(req)

	telemetry.Log.
		WithField("model", req.Model).
		WithField("msg_count", len(contents)).
		Debug("gemini: sending request")

	resp, err := p.client.Models.GenerateContent(ctx, req.Model, contents, config)
	if err != nil {
		return nil, fmt.Errorf("gemini API: %w", err)
	}

	return fromGeminiResponse(resp, nameMap), nil
}

// CompleteStream starts a streaming completion and delivers events via a channel.
func (p *GeminiProvider) CompleteStream(ctx context.Context, req *Request) (<-chan StreamEvent, error) {
	contents, nameMap := toGeminiContents(req.Messages)
	config := buildGeminiConfig(req)

	telemetry.Log.
		WithField("model", req.Model).
		WithField("msg_count", len(contents)).
		Debug("gemini: starting stream")

	stream := p.client.Models.GenerateContentStream(ctx, req.Model, contents, config)

	ch := make(chan StreamEvent, 64)
	go func() {
		defer close(ch)
		emitGeminiStream(stream, nameMap, ch)
	}()

	return ch, nil
}

// buildGeminiConfig converts a unified Request into Gemini's GenerateContentConfig.
func buildGeminiConfig(req *Request) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(req.MaxTokens),
	}
	if req.System != "" {
		config.SystemInstruction = genai.NewContentFromText(req.System, "user")
	}
	if tools := toGeminiTools(req.Tools); len(tools) > 0 {
		config.Tools = tools
	}
	return config
}

// toGeminiContents converts unified messages to Gemini Content slices.
// It also builds a toolUseID→toolName map for resolving tool results.
func toGeminiContents(msgs []Message) ([]*genai.Content, map[string]string) {
	nameMap := buildToolNameMap(msgs)
	var contents []*genai.Content

	for _, m := range msgs {
		switch m.Role {
		case RoleAssistant:
			if c := toGeminiAssistantContent(m); c != nil {
				contents = append(contents, c)
			}
		case RoleUser:
			contents = append(contents, toGeminiUserContents(m, nameMap)...)
		}
	}
	return contents, nameMap
}

// toGeminiAssistantContent converts an assistant message to a Gemini Content.
func toGeminiAssistantContent(m Message) *genai.Content {
	var parts []*genai.Part
	for _, cb := range m.Content {
		switch cb.Type {
		case ContentText:
			if cb.Text != "" {
				parts = append(parts, &genai.Part{Text: cb.Text})
			}
		case ContentToolUse:
			args := argsToMap(cb.Input)
			parts = append(parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: cb.Name,
					Args: args,
				},
			})
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return &genai.Content{Role: "model", Parts: parts}
}

// toGeminiUserContents converts a user message to one or more Gemini Contents.
// Text goes into a "user" content; tool results go into separate "user" contents.
func toGeminiUserContents(m Message, nameMap map[string]string) []*genai.Content {
	var textParts []*genai.Part
	var toolContents []*genai.Content

	for _, cb := range m.Content {
		switch cb.Type {
		case ContentText:
			if cb.Text != "" {
				textParts = append(textParts, &genai.Part{Text: cb.Text})
			}
		case ContentToolResult:
			name := nameMap[cb.ToolUseID]
			if name == "" {
				name = "unknown"
			}
			resp := map[string]any{"output": cb.Content}
			if cb.IsError {
				resp = map[string]any{"error": cb.Content}
			}
			toolContents = append(toolContents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{{
					FunctionResponse: &genai.FunctionResponse{
						Name:     name,
						Response: resp,
					},
				}},
			})
		}
	}

	var result []*genai.Content
	if len(textParts) > 0 {
		result = append(result, &genai.Content{
			Role:  "user",
			Parts: textParts,
		})
	}
	result = append(result, toolContents...)
	return result
}

// toGeminiTools converts unified tool definitions to Gemini Tool format.
func toGeminiTools(tools []ToolDefinition) []*genai.Tool {
	if len(tools) == 0 {
		return nil
	}
	var decls []*genai.FunctionDeclaration
	for _, t := range tools {
		decl := &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
		}
		// Use ParametersJsonSchema for raw JSON schema passthrough
		if len(t.InputSchema) > 0 {
			var schema any
			if err := json.Unmarshal(t.InputSchema, &schema); err == nil {
				decl.ParametersJsonSchema = schema
			}
		}
		decls = append(decls, decl)
	}
	return []*genai.Tool{{FunctionDeclarations: decls}}
}

// fromGeminiResponse converts a Gemini response to our unified Response.
func fromGeminiResponse(resp *genai.GenerateContentResponse, nameMap map[string]string) *Response {
	if resp == nil || len(resp.Candidates) == 0 {
		return &Response{ID: safeResponseID(resp), StopReason: StopEndTurn}
	}

	candidate := resp.Candidates[0]
	blocks := extractGeminiBlocks(candidate, nameMap)
	stopReason := mapGeminiFinishReason(candidate.FinishReason, blocks)
	usage := extractGeminiUsage(resp.UsageMetadata)

	telemetry.Log.
		WithField("prompt_tokens", usage.InputTokens).
		WithField("completion_tokens", usage.OutputTokens).
		WithField("finish_reason", string(candidate.FinishReason)).
		Debug("gemini: response received")

	return &Response{
		ID:         safeResponseID(resp),
		Content:    blocks,
		StopReason: stopReason,
		Usage:      usage,
	}
}

// extractGeminiBlocks converts Gemini candidate parts to unified ContentBlocks.
func extractGeminiBlocks(candidate *genai.Candidate, nameMap map[string]string) []ContentBlock {
	if candidate.Content == nil {
		return nil
	}

	var blocks []ContentBlock
	callIdx := len(nameMap) // counter for synthetic IDs
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			blocks = append(blocks, NewTextBlock(part.Text))
		}
		if part.FunctionCall != nil {
			fc := part.FunctionCall
			id := fc.ID
			if id == "" {
				callIdx++
				id = fmt.Sprintf("gemini_call_%d", callIdx)
			}
			argsJSON, _ := json.Marshal(fc.Args)
			if len(argsJSON) == 0 {
				argsJSON = json.RawMessage(`{}`)
			}
			blocks = append(blocks, ContentBlock{
				Type:  ContentToolUse,
				ID:    id,
				Name:  fc.Name,
				Input: argsJSON,
			})
		}
	}
	return blocks
}

// emitGeminiStream processes a Gemini streaming iterator and sends events.
func emitGeminiStream(
	stream func(func(*genai.GenerateContentResponse, error) bool),
	nameMap map[string]string,
	ch chan<- StreamEvent,
) {
	callIdx := len(nameMap)
	blockIndex := 0

	stream(func(resp *genai.GenerateContentResponse, err error) bool {
		if err != nil {
			ch <- StreamEvent{
				Type:  EventError,
				Error: fmt.Errorf("gemini stream: %w", err),
			}
			return false
		}

		if resp == nil || len(resp.Candidates) == 0 {
			return true
		}

		candidate := resp.Candidates[0]
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					ch <- StreamEvent{
						Type:  EventTextDelta,
						Text:  part.Text,
						Index: blockIndex,
					}
				}
				if part.FunctionCall != nil {
					fc := part.FunctionCall
					id := fc.ID
					if id == "" {
						callIdx++
						id = fmt.Sprintf("gemini_call_%d", callIdx)
					}
					blockIndex++
					ch <- StreamEvent{
						Type:  EventToolUseStart,
						ID:    id,
						Name:  fc.Name,
						Index: blockIndex,
					}
					argsJSON, _ := json.Marshal(fc.Args)
					if len(argsJSON) > 0 {
						ch <- StreamEvent{
							Type:      EventInputJSONDelta,
							InputJSON: string(argsJSON),
							Index:     blockIndex,
						}
					}
					ch <- StreamEvent{
						Type:  EventContentBlockStop,
						Index: blockIndex,
					}
				}
			}
		}

		// Finish reason
		if candidate.FinishReason != "" {
			usage := extractGeminiUsage(resp.UsageMetadata)
			ch <- StreamEvent{
				Type:       EventMessageDelta,
				StopReason: mapGeminiFinishReason(candidate.FinishReason, nil),
				Usage:      usage,
			}
		}

		return true
	})
}

// mapGeminiFinishReason converts a Gemini FinishReason to our StopReason.
// If blocks contain function calls, the stop reason is StopToolUse regardless
// of the declared finish reason.
func mapGeminiFinishReason(reason genai.FinishReason, blocks []ContentBlock) StopReason {
	// If any block is a tool call, treat as tool_use stop
	for _, b := range blocks {
		if b.Type == ContentToolUse {
			return StopToolUse
		}
	}
	switch reason {
	case genai.FinishReasonStop:
		return StopEndTurn
	case genai.FinishReasonMaxTokens:
		return StopMaxTokens
	default:
		return StopEndTurn
	}
}

// buildToolNameMap scans messages to build a toolUseID→toolName mapping.
// This is needed because Gemini's FunctionResponse requires a function name,
// not an ID.
func buildToolNameMap(msgs []Message) map[string]string {
	m := make(map[string]string)
	for _, msg := range msgs {
		if msg.Role != RoleAssistant {
			continue
		}
		for _, cb := range msg.Content {
			if cb.Type == ContentToolUse && cb.ID != "" {
				m[cb.ID] = cb.Name
			}
		}
	}
	return m
}

// argsToMap unmarshals JSON-encoded tool arguments to map[string]any.
func argsToMap(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// extractGeminiUsage extracts token usage from Gemini response metadata.
func extractGeminiUsage(meta *genai.GenerateContentResponseUsageMetadata) Usage {
	if meta == nil {
		return Usage{}
	}
	return Usage{
		InputTokens:  int(meta.PromptTokenCount),
		OutputTokens: int(meta.CandidatesTokenCount),
	}
}

// safeResponseID extracts the response ID, returning empty string if nil.
func safeResponseID(resp *genai.GenerateContentResponse) string {
	if resp == nil {
		return ""
	}
	return resp.ResponseID
}
