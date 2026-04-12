package session

import (
	"github.com/xmh1011/glaude/internal/llm"
)

// BuildChain reconstructs a linear conversation from a DAG of entries.
// It finds the leaf entry (no children), walks ParentUUID to root, and
// returns the chain in chronological order. Only message entries (user/assistant)
// are included.
func BuildChain(entries []*Entry) []*Entry {
	if len(entries) == 0 {
		return nil
	}

	// Index entries by UUID
	byUUID := make(map[string]*Entry, len(entries))
	for _, e := range entries {
		if e.UUID != "" {
			byUUID[e.UUID] = e
		}
	}

	// Find leaf: an entry whose UUID is not referenced as any other entry's ParentUUID
	hasChild := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.ParentUUID != "" {
			hasChild[e.ParentUUID] = true
		}
	}

	// Among message entries, find the leaf (last entry with no children).
	// If all entries have children (cycle), fall back to the last message entry.
	var leaf *Entry
	var lastMsg *Entry
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if !isMessageEntry(e) {
			continue
		}
		if lastMsg == nil {
			lastMsg = e
		}
		if !hasChild[e.UUID] {
			leaf = e
			break
		}
	}
	if leaf == nil {
		leaf = lastMsg // fallback for cycles
	}
	if leaf == nil {
		return nil
	}

	// Walk backward from leaf to root
	var chain []*Entry
	seen := make(map[string]bool)
	current := leaf
	for current != nil {
		if seen[current.UUID] {
			break // cycle detection
		}
		seen[current.UUID] = true
		if isMessageEntry(current) {
			chain = append(chain, current)
		}
		if current.ParentUUID == "" {
			break
		}
		current = byUUID[current.ParentUUID]
	}

	// Reverse to chronological order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// ToMessages converts a chain of entries back into llm.Message slice
// suitable for Agent restoration.
func ToMessages(chain []*Entry) []llm.Message {
	msgs := make([]llm.Message, 0, len(chain))
	for _, e := range chain {
		if e.Message != nil {
			msgs = append(msgs, *e.Message)
		}
	}
	return msgs
}

// isMessageEntry returns true for entries that carry conversation messages.
func isMessageEntry(e *Entry) bool {
	return e.Type == "user" || e.Type == "assistant"
}
