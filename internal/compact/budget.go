// Package compact implements context budget management and compression strategies.
//
// It provides:
//   - Token counting via tiktoken-go for local estimation
//   - Context budget tracking (system prompt, tools, messages, response reserve)
//   - MicroCompact: local-only strategy to clear old tool results
//   - AutoCompact: LLM-driven summarization when context is nearly full
package compact

import (
	"encoding/json"
	"strings"

	tiktoken "github.com/pkoukk/tiktoken-go"

	"github.com/xmh1011/glaude/internal/llm"
)

// Default budget constants.
const (
	// DefaultContextWindow is the typical context window size for Claude models.
	DefaultContextWindow = 200000

	// ResponseReserve is the token budget reserved for model output.
	ResponseReserve = 16000

	// AutoCompactBuffer is the buffer below the effective window that triggers auto-compact.
	AutoCompactBuffer = 13000

	// WarningThreshold is the buffer for showing a warning indicator.
	WarningThreshold = 20000
)

// TokenCounter estimates token counts using tiktoken.
// It falls back to a simple heuristic if tiktoken initialization fails.
type TokenCounter struct {
	enc *tiktoken.Tiktoken
}

// NewTokenCounter creates a counter using the cl100k_base encoding (suitable for Claude).
func NewTokenCounter() *TokenCounter {
	enc, err := tiktoken.EncodingForModel("gpt-4")
	if err != nil {
		// Fallback: cl100k_base is the closest available encoding
		enc, _ = tiktoken.GetEncoding("cl100k_base")
	}
	return &TokenCounter{enc: enc}
}

// Count returns the estimated token count for a string.
func (c *TokenCounter) Count(text string) int {
	if c.enc == nil {
		// Rough heuristic fallback: ~4 chars per token for English
		return len(text) / 4
	}
	return len(c.enc.Encode(text, nil, nil))
}

// CountMessages estimates the total token count for a slice of messages.
func (c *TokenCounter) CountMessages(messages []llm.Message) int {
	total := 0
	for _, msg := range messages {
		// Per-message overhead (~4 tokens for role, delimiters)
		total += 4
		for _, block := range msg.Content {
			switch block.Type {
			case llm.ContentText:
				total += c.Count(block.Text)
			case llm.ContentToolUse:
				total += c.Count(block.Name)
				if block.Input != nil {
					total += c.Count(string(block.Input))
				}
			case llm.ContentToolResult:
				total += c.Count(block.Content)
			}
		}
	}
	return total
}

// CountToolDefinitions estimates token count for tool definitions.
func (c *TokenCounter) CountToolDefinitions(tools []llm.ToolDefinition) int {
	total := 0
	for _, t := range tools {
		total += c.Count(t.Name)
		total += c.Count(t.Description)
		if t.InputSchema != nil {
			total += c.Count(string(t.InputSchema))
		}
	}
	return total
}

// Budget tracks token allocation across context components.
// It supports two counting modes:
//   - Local estimation via TokenCounter (always available)
//   - API-calibrated counting: uses the last API response's input_tokens as a
//     more accurate baseline, only estimating tokens for newly added messages.
//     This matches Claude Code's tokens.ts approach.
type Budget struct {
	ContextWindow int // total window size
	SystemPrompt  int // tokens used by system prompt
	Tools         int // tokens used by tool definitions
	Messages      int // tokens used by conversation messages
	Reserved      int // tokens reserved for model response

	// API-calibrated counting state
	lastAPIInputTokens int // input_tokens from the most recent API response
	lastMessageCount   int // number of messages when lastAPIInputTokens was recorded
}

// NewBudget creates a Budget with the given context window and reserve.
func NewBudget(contextWindow, reserve int) *Budget {
	return &Budget{
		ContextWindow: contextWindow,
		Reserved:      reserve,
	}
}

// EffectiveWindow returns the usable context window (total - response reserve).
func (b *Budget) EffectiveWindow() int {
	return b.ContextWindow - b.Reserved
}

// Used returns total tokens used across all components.
func (b *Budget) Used() int {
	return b.SystemPrompt + b.Tools + b.Messages
}

// Available returns remaining tokens for messages.
func (b *Budget) Available() int {
	avail := b.EffectiveWindow() - b.Used()
	if avail < 0 {
		return 0
	}
	return avail
}

// UsagePercent returns the percentage of effective window used.
func (b *Budget) UsagePercent() float64 {
	eff := b.EffectiveWindow()
	if eff <= 0 {
		return 100.0
	}
	return float64(b.Used()) / float64(eff) * 100.0
}

// NeedsCompact returns true if usage exceeds the auto-compact threshold.
func (b *Budget) NeedsCompact() bool {
	return b.Used() > b.EffectiveWindow()-AutoCompactBuffer
}

// NeedsWarning returns true if usage exceeds the warning threshold.
func (b *Budget) NeedsWarning() bool {
	return b.Used() > b.EffectiveWindow()-WarningThreshold
}

// Update recalculates all budget components using the given counter.
// If API usage data has been calibrated via CalibrateFromAPI, it uses the
// more accurate API-reported count for existing messages and only estimates
// tokens for newly added messages since the last API call.
func (b *Budget) Update(counter *TokenCounter, systemPrompt string, tools []llm.ToolDefinition, messages []llm.Message) {
	b.SystemPrompt = counter.Count(systemPrompt)
	b.Tools = counter.CountToolDefinitions(tools)

	if b.lastAPIInputTokens > 0 && b.lastMessageCount > 0 && len(messages) >= b.lastMessageCount {
		// Use API-reported input_tokens as the baseline for messages seen so far.
		// The API input_tokens includes system prompt + tools + messages, so
		// subtract system + tools to get message-only count.
		apiMessageTokens := b.lastAPIInputTokens - b.SystemPrompt - b.Tools
		if apiMessageTokens < 0 {
			apiMessageTokens = 0
		}

		// Only estimate tokens for messages added after the last API call
		newMessages := messages[b.lastMessageCount:]
		newTokens := counter.CountMessages(newMessages)
		b.Messages = apiMessageTokens + newTokens
	} else {
		// Pure local estimation (no API data yet)
		b.Messages = counter.CountMessages(messages)
	}
}

// CalibrateFromAPI records the input_tokens from the most recent API response.
// This provides a more accurate baseline for budget calculations, as the API
// knows the exact token count including any model-specific overhead.
func (b *Budget) CalibrateFromAPI(inputTokens int, messageCount int) {
	b.lastAPIInputTokens = inputTokens
	b.lastMessageCount = messageCount
}

// ResetCalibration clears the API calibration data (e.g., after auto-compact).
func (b *Budget) ResetCalibration() {
	b.lastAPIInputTokens = 0
	b.lastMessageCount = 0
}

// estimateBlockTokens returns a rough token estimate for a content block.
func estimateBlockTokens(block llm.ContentBlock) int {
	switch block.Type {
	case llm.ContentText:
		return len(block.Text) / 4
	case llm.ContentToolUse:
		n := len(block.Name)/4 + len(block.Input)/4
		if n < 10 {
			return 10
		}
		return n
	case llm.ContentToolResult:
		return len(block.Content) / 4
	default:
		return 0
	}
}

// FormatBudgetBar returns a text-based budget indicator.
func FormatBudgetBar(b *Budget) string {
	pct := b.UsagePercent()
	var sb strings.Builder
	sb.WriteString("[")

	barWidth := 20
	filled := int(pct / 100.0 * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	sb.WriteString(strings.Repeat("█", filled))
	sb.WriteString(strings.Repeat("░", barWidth-filled))
	sb.WriteString("]")

	return sb.String()
}

// contentBlockJSON returns a rough JSON-like string for estimation.
func contentBlockJSON(block llm.ContentBlock) string {
	data, _ := json.Marshal(block)
	return string(data)
}
