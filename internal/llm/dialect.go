package llm

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// DialectFixer is a Provider decorator that repairs malformed responses
// from open-source models (e.g., Ollama). It fixes invalid JSON in
// tool_use input fields and filters empty tool calls.
type DialectFixer struct {
	inner Provider
}

// NewDialectFixer wraps a Provider with response repair logic.
func NewDialectFixer(inner Provider) *DialectFixer {
	return &DialectFixer{inner: inner}
}

// Complete delegates to the inner provider, then repairs the response.
func (d *DialectFixer) Complete(ctx context.Context, req *Request) (*Response, error) {
	resp, err := d.inner.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	d.fixResponse(resp)
	return resp, nil
}

// fixResponse repairs malformed content blocks in-place.
func (d *DialectFixer) fixResponse(resp *Response) {
	var cleaned []ContentBlock
	for i := range resp.Content {
		cb := &resp.Content[i]
		if cb.Type == ContentToolUse {
			// Filter out empty tool calls (no name)
			if cb.Name == "" {
				telemetry.Log.Debug("dialect: filtering empty tool_call")
				continue
			}
			// Repair malformed JSON input
			if len(cb.Input) > 0 && !json.Valid(cb.Input) {
				fixed := SafeParseJSON(cb.Input)
				telemetry.Log.
					WithField("tool", cb.Name).
					WithField("original_len", len(cb.Input)).
					Debug("dialect: repaired tool input JSON")
				cb.Input = fixed
			}
			// Ensure input is not empty
			if len(cb.Input) == 0 {
				cb.Input = json.RawMessage(`{}`)
			}
		}
		cleaned = append(cleaned, *cb)
	}
	resp.Content = cleaned
}

// SafeParseJSON attempts to parse raw bytes as JSON.
// If parsing fails, it applies basic repairs:
//   - Strip UTF-8 BOM
//   - Remove trailing commas before } or ]
//   - Complete unclosed braces/brackets
//
// Returns valid JSON or `{}` as a last resort.
func SafeParseJSON(raw []byte) json.RawMessage {
	// Fast path: already valid
	if json.Valid(raw) {
		return json.RawMessage(raw)
	}

	// Strip BOM
	data := bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if json.Valid(data) {
		return json.RawMessage(data)
	}

	// Trim whitespace
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return json.RawMessage(`{}`)
	}

	// Remove trailing commas before } or ]
	data = removeTrailingCommas(data)
	if json.Valid(data) {
		return json.RawMessage(data)
	}

	// Try to close unclosed braces/brackets
	data = closeUnclosed(data)
	if json.Valid(data) {
		return json.RawMessage(data)
	}

	// Last resort
	return json.RawMessage(`{}`)
}

// removeTrailingCommas removes commas that appear before } or ].
func removeTrailingCommas(data []byte) []byte {
	result := make([]byte, 0, len(data))
	inString := false
	escape := false
	for i := 0; i < len(data); i++ {
		b := data[i]
		if escape {
			result = append(result, b)
			escape = false
			continue
		}
		if b == '\\' && inString {
			result = append(result, b)
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			result = append(result, b)
			continue
		}
		if inString {
			result = append(result, b)
			continue
		}
		if b == ',' {
			// Look ahead for } or ] (skipping whitespace)
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue // skip this comma
			}
		}
		result = append(result, b)
	}
	return result
}

// closeUnclosed counts unmatched { and [ and appends the corresponding closers.
func closeUnclosed(data []byte) []byte {
	var stack []byte
	inString := false
	escape := false
	for _, b := range data {
		if escape {
			escape = false
			continue
		}
		if b == '\\' && inString {
			escape = true
			continue
		}
		if b == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch b {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			if len(stack) > 0 && stack[len(stack)-1] == b {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) == 0 {
		return data
	}
	result := make([]byte, len(data), len(data)+len(stack))
	copy(result, data)
	// Close in reverse order
	for i := len(stack) - 1; i >= 0; i-- {
		result = append(result, stack[i])
	}
	return result
}
