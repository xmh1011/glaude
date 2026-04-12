// Package session implements JSONL-based conversation persistence.
//
// Each conversation is stored as an append-only JSONL file with one Entry
// per line. Entries form a DAG via UUID/ParentUUID links, enabling fork
// and resume semantics.
package session

import (
	"time"

	"github.com/xmh1011/glaude/internal/llm"
)

// Entry is a single line in the session JSONL file.
// The Type field discriminates what kind of entry this is.
type Entry struct {
	Type       string       `json:"type"`                  // "user","assistant","summary","title","last-prompt","tag"
	UUID       string       `json:"uuid"`                  // unique message ID
	ParentUUID string       `json:"parent_uuid,omitempty"` // DAG parent link
	SessionID  string       `json:"session_id"`
	CWD        string       `json:"cwd"`
	Timestamp  string       `json:"timestamp"` // RFC3339
	Message    *llm.Message `json:"message,omitempty"`
	Text       string       `json:"text,omitempty"` // for title/summary/tag/last-prompt
}

// SessionInfo holds metadata about a saved session, extracted from JSONL
// head/tail without loading the full file.
type SessionInfo struct {
	ID         string
	Title      string
	LastPrompt string
	Timestamp  time.Time
	Path       string // full path to the JSONL file
}
