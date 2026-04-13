// Package websearch implements the WebSearch tool for searching the web.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/llm"
)

// Tool searches the web by delegating to the LLM with a search prompt.
type Tool struct {
	Provider llm.Provider
	Model    string
}

// Input is the parsed input for the WebSearch tool.
type Input struct {
	Query          string   `json:"query"`
	AllowedDomains []string `json:"allowed_domains,omitempty"`
	BlockedDomains []string `json:"blocked_domains,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "WebSearch" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Searches the web for information. Returns formatted search results. Use this when you need current information not in your training data."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "The search query"},
			"allowed_domains": {"type": "array", "items": {"type": "string"}, "description": "Only include results from these domains"},
			"blocked_domains": {"type": "array", "items": {"type": "string"}, "description": "Exclude results from these domains"}
		},
		"required": ["query"]
	}`)
}

// IsReadOnly returns true since this only reads information.
func (t *Tool) IsReadOnly() bool { return true }

// Execute performs a web search by querying the LLM.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	prompt := buildSearchPrompt(in)

	req := &llm.Request{
		Model:     t.Model,
		MaxTokens: 1024,
		System:    "You are a helpful search assistant. Provide concise, factual search results.",
		Messages: []llm.Message{
			{Role: "user", Content: []llm.ContentBlock{llm.NewTextBlock(prompt)}},
		},
	}

	resp, err := t.Provider.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("web search: %w", err)
	}

	var text strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}

	if text.Len() == 0 {
		return "No results found.", nil
	}
	return text.String(), nil
}

func buildSearchPrompt(in Input) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Search the web for: %s\n", in.Query)

	if len(in.AllowedDomains) > 0 {
		fmt.Fprintf(&b, "Only include results from: %s\n", strings.Join(in.AllowedDomains, ", "))
	}
	if len(in.BlockedDomains) > 0 {
		fmt.Fprintf(&b, "Exclude results from: %s\n", strings.Join(in.BlockedDomains, ", "))
	}

	b.WriteString("\nProvide a concise summary of the top results with relevant information.")
	return b.String()
}
