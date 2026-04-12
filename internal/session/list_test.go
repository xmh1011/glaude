package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/llm"
)

func TestListSessions_Empty(t *testing.T) {
	sessions, err := ListSessions("/nonexistent/path/to/project")
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestListSessions_WithSessions(t *testing.T) {
	tmp := t.TempDir()
	cwd := "/test/project"

	// Override SessionDir for test by creating sessions directly in tmp
	sid1 := uuid.New().String()
	sid2 := uuid.New().String()

	s1 := &Store{dir: tmp, sessionID: sid1}
	s2 := &Store{dir: tmp, sessionID: sid2}

	// Write session 1
	require.NoError(t, s1.Append(&Entry{
		Type:      "user",
		UUID:      "u1",
		SessionID: sid1,
		CWD:       cwd,
		Timestamp: time.Now().Add(-time.Hour).Format(time.RFC3339),
		Message:   &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("older")}},
	}))
	require.NoError(t, s1.Append(&Entry{
		Type:      "last-prompt",
		UUID:      "lp1",
		SessionID: sid1,
		Timestamp: time.Now().Add(-time.Hour).Format(time.RFC3339),
		Text:      "older prompt",
	}))
	require.NoError(t, s1.Close())

	// Write session 2 (more recent)
	require.NoError(t, s2.Append(&Entry{
		Type:      "user",
		UUID:      "u2",
		SessionID: sid2,
		CWD:       cwd,
		Timestamp: time.Now().Format(time.RFC3339),
		Message:   &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("newer")}},
	}))
	require.NoError(t, s2.Append(&Entry{
		Type:      "title",
		UUID:      "t2",
		SessionID: sid2,
		Timestamp: time.Now().Format(time.RFC3339),
		Text:      "Session Two",
	}))
	require.NoError(t, s2.Close())

	// List from the tmp dir directly
	files, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	// Verify JSONL files exist
	for _, f := range files {
		assert.True(t, filepath.Ext(f.Name()) == ".jsonl")
	}
}

func TestMostRecentSession_NoSessions(t *testing.T) {
	info := MostRecentSession("/nonexistent/project")
	assert.Nil(t, info)
}

func TestExtractSessionInfo_Title(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.jsonl")

	content := `{"type":"user","uuid":"a","session_id":"sess-1","timestamp":"2024-01-15T10:00:00Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"title","uuid":"t","session_id":"sess-1","timestamp":"2024-01-15T10:00:05Z","text":"My Title"}
{"type":"last-prompt","uuid":"lp","session_id":"sess-1","timestamp":"2024-01-15T10:00:05Z","text":"hi"}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	info := extractSessionInfo(path)
	assert.Equal(t, "sess-1", info.ID)
	assert.Equal(t, "My Title", info.Title)
	assert.Equal(t, "hi", info.LastPrompt)
}
