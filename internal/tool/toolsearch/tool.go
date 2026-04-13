// Package toolsearch implements the ToolSearch tool for finding tools in the registry.
package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xmh1011/glaude/internal/tool"
)

const defaultMaxResults = 10

// Tool searches the registry for tools by name or keyword.
type Tool struct {
	Registry *tool.Registry
}

// Input is the parsed input for the ToolSearch tool.
type Input struct {
	Query      string `json:"query"`
	MaxResults *int   `json:"max_results,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "ToolSearch" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Searches for tools in the registry by name or keyword. Use 'select:<name>' for exact tool lookup."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Search query or 'select:<tool_name>' for exact lookup"},
			"max_results": {"type": "integer", "description": "Maximum number of results to return", "default": 10}
		},
		"required": ["query"]
	}`)
}

// IsReadOnly returns true since this only reads registry state.
func (t *Tool) IsReadOnly() bool { return true }

// Execute searches for tools matching the query.
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

	maxResults := defaultMaxResults
	if in.MaxResults != nil && *in.MaxResults > 0 {
		maxResults = *in.MaxResults
	}

	// Handle "select:<name>" for exact lookup.
	if strings.HasPrefix(in.Query, "select:") {
		name := strings.TrimPrefix(in.Query, "select:")
		name = strings.TrimSpace(name)
		found := t.Registry.Get(name)
		if found == nil {
			return fmt.Sprintf("No tool named %q found.", name), nil
		}
		return formatToolDetail(found), nil
	}

	// Keyword search across name and description.
	query := strings.ToLower(in.Query)
	var results []tool.Tool
	for _, tl := range t.Registry.All() {
		score := matchScore(tl, query)
		if score > 0 {
			results = append(results, tl)
		}
	}

	if len(results) == 0 {
		return fmt.Sprintf("No tools matching %q found.", in.Query), nil
	}

	if len(results) > maxResults {
		results = results[:maxResults]
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d tool(s) matching %q:\n\n", len(results), in.Query)
	for _, tl := range results {
		fmt.Fprintf(&b, "- **%s**: %s\n", tl.Name(), truncate(tl.Description(), 100))
	}
	return b.String(), nil
}

func matchScore(t tool.Tool, query string) int {
	name := strings.ToLower(t.Name())
	desc := strings.ToLower(t.Description())

	score := 0
	if strings.Contains(name, query) {
		score += 10
	}
	if strings.Contains(desc, query) {
		score += 5
	}
	return score
}

func formatToolDetail(t tool.Tool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**\n", t.Name())
	fmt.Fprintf(&b, "Read-only: %v\n", t.IsReadOnly())
	fmt.Fprintf(&b, "Description: %s\n", t.Description())
	fmt.Fprintf(&b, "Input Schema:\n```json\n%s\n```\n", string(t.InputSchema()))
	return b.String()
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
