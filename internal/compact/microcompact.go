package compact

import (
	"github.com/xmh1011/glaude/internal/llm"
)

// MicroCompact configuration.
const (
	// PreserveRecentResults is the number of recent tool results to keep intact.
	PreserveRecentResults = 5

	// MaxToolResultBytes is the maximum size of a tool result before truncation.
	MaxToolResultBytes = 100 * 1024 // 100KB

	// ClearedPlaceholder replaces cleared tool result content.
	ClearedPlaceholder = "[Old tool result content cleared]"

	// TruncatedSuffix is appended to truncated tool results.
	TruncatedSuffix = "\n... [output truncated, showing first 100KB]"
)

// compactableTools lists tools whose results can be safely cleared.
var compactableTools = map[string]bool{
	"Read":  true,
	"Bash":  true,
	"Grep":  true,
	"Glob":  true,
	"Edit":  true,
	"Write": true,
	"LS":    true,
}

// MicroCompact performs local-only compression on a message slice.
// It clears old tool results while preserving recent ones, and truncates
// oversized outputs. It never calls the LLM API.
//
// The function returns a new message slice — the input is not modified.
func MicroCompact(messages []llm.Message) []llm.Message {
	// First pass: find indices of all tool_result blocks that are compactable
	type resultInfo struct {
		msgIdx   int
		blockIdx int
		toolName string
	}
	var compactable []resultInfo

	// Build a map from tool_use_id to tool name
	toolNames := make(map[string]string)
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == llm.ContentToolUse {
				toolNames[block.ID] = block.Name
			}
		}
	}

	for mi, msg := range messages {
		for bi, block := range msg.Content {
			if block.Type == llm.ContentToolResult {
				name := toolNames[block.ToolUseID]
				if compactableTools[name] {
					compactable = append(compactable, resultInfo{
						msgIdx:   mi,
						blockIdx: bi,
						toolName: name,
					})
				}
			}
		}
	}

	// Determine which results to clear (all except the most recent N)
	clearCount := len(compactable) - PreserveRecentResults
	if clearCount <= 0 {
		// Nothing to compact, but still check for oversized results
		return truncateOversized(messages)
	}

	toClear := make(map[[2]int]bool)
	for i := 0; i < clearCount; i++ {
		ri := compactable[i]
		toClear[[2]int{ri.msgIdx, ri.blockIdx}] = true
	}

	// Build new message slice with cleared results
	result := make([]llm.Message, len(messages))
	for mi, msg := range messages {
		newBlocks := make([]llm.ContentBlock, len(msg.Content))
		for bi, block := range msg.Content {
			if toClear[[2]int{mi, bi}] {
				newBlocks[bi] = llm.ContentBlock{
					Type:      llm.ContentToolResult,
					ToolUseID: block.ToolUseID,
					Content:   ClearedPlaceholder,
					IsError:   block.IsError,
				}
			} else if block.Type == llm.ContentToolResult && len(block.Content) > MaxToolResultBytes {
				newBlocks[bi] = llm.ContentBlock{
					Type:      llm.ContentToolResult,
					ToolUseID: block.ToolUseID,
					Content:   block.Content[:MaxToolResultBytes] + TruncatedSuffix,
					IsError:   block.IsError,
				}
			} else {
				newBlocks[bi] = block
			}
		}
		result[mi] = llm.Message{
			Role:    msg.Role,
			Content: newBlocks,
		}
	}

	return result
}

// truncateOversized returns a copy with oversized tool results truncated.
func truncateOversized(messages []llm.Message) []llm.Message {
	needsCopy := false
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.Type == llm.ContentToolResult && len(block.Content) > MaxToolResultBytes {
				needsCopy = true
				break
			}
		}
		if needsCopy {
			break
		}
	}

	if !needsCopy {
		return messages
	}

	result := make([]llm.Message, len(messages))
	for mi, msg := range messages {
		newBlocks := make([]llm.ContentBlock, len(msg.Content))
		for bi, block := range msg.Content {
			if block.Type == llm.ContentToolResult && len(block.Content) > MaxToolResultBytes {
				newBlocks[bi] = llm.ContentBlock{
					Type:      llm.ContentToolResult,
					ToolUseID: block.ToolUseID,
					Content:   block.Content[:MaxToolResultBytes] + TruncatedSuffix,
					IsError:   block.IsError,
				}
			} else {
				newBlocks[bi] = block
			}
		}
		result[mi] = llm.Message{
			Role:    msg.Role,
			Content: newBlocks,
		}
	}
	return result
}
