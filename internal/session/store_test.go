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

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/Users/test/code/myproject", "-Users-test-code-myproject"},
		{"simple", "simple"},
		{"/a/b/c", "-a-b-c"},
	}
	for _, tt := range tests {
		got := SanitizePath(tt.input)
		assert.Equal(t, tt.want, got, "SanitizePath(%q)", tt.input)
	}
}

func TestSanitizePath_LongPath(t *testing.T) {
	long := "/" + string(make([]byte, 250)) // 251 chars after sanitization
	got := SanitizePath(long)
	assert.LessOrEqual(t, len(got), 201, "sanitized path should be truncated")
	assert.Contains(t, got, "-") // has hash suffix
}

func TestStore_LazyFileCreation(t *testing.T) {
	tmp := t.TempDir()
	s := &Store{dir: tmp, sessionID: "test-session"}

	// File should not exist yet
	_, err := os.Stat(s.FilePath())
	assert.True(t, os.IsNotExist(err))

	// After Append, file should exist
	err = s.Append(&Entry{
		Type:      "user",
		UUID:      uuid.New().String(),
		SessionID: "test-session",
		Timestamp: time.Now().Format(time.RFC3339),
		Message:   &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("hello")}},
	})
	require.NoError(t, err)

	_, err = os.Stat(s.FilePath())
	assert.NoError(t, err)
	require.NoError(t, s.Close())
}

func TestStore_AppendAndLoad(t *testing.T) {
	tmp := t.TempDir()
	sid := "sess-" + uuid.New().String()
	s := &Store{dir: tmp, sessionID: sid}

	id1 := uuid.New().String()
	id2 := uuid.New().String()

	require.NoError(t, s.Append(&Entry{
		Type:      "user",
		UUID:      id1,
		SessionID: sid,
		CWD:       "/test",
		Timestamp: time.Now().Format(time.RFC3339),
		Message:   &llm.Message{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.NewTextBlock("Hi")}},
	}))
	require.NoError(t, s.Append(&Entry{
		Type:       "assistant",
		UUID:       id2,
		ParentUUID: id1,
		SessionID:  sid,
		CWD:        "/test",
		Timestamp:  time.Now().Format(time.RFC3339),
		Message:    &llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.NewTextBlock("Hello!")}},
	}))
	require.NoError(t, s.Close())

	// Load and verify
	entries, err := LoadEntries(s.FilePath())
	require.NoError(t, err)
	require.Len(t, entries, 2)

	assert.Equal(t, "user", entries[0].Type)
	assert.Equal(t, id1, entries[0].UUID)
	assert.Equal(t, "", entries[0].ParentUUID)

	assert.Equal(t, "assistant", entries[1].Type)
	assert.Equal(t, id2, entries[1].UUID)
	assert.Equal(t, id1, entries[1].ParentUUID)
}

func TestStore_AppendMetadata(t *testing.T) {
	tmp := t.TempDir()
	s := &Store{dir: tmp, sessionID: "meta-test"}

	require.NoError(t, s.Append(&Entry{
		Type:      "title",
		UUID:      uuid.New().String(),
		SessionID: "meta-test",
		Timestamp: time.Now().Format(time.RFC3339),
		Text:      "My Session Title",
	}))
	require.NoError(t, s.Close())

	entries, err := LoadEntries(s.FilePath())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "title", entries[0].Type)
	assert.Equal(t, "My Session Title", entries[0].Text)
}

func TestLoadEntries_NotFound(t *testing.T) {
	_, err := LoadEntries("/nonexistent/path.jsonl")
	assert.Error(t, err)
}

func TestLoadEntries_MalformedLines(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.jsonl")
	content := `{"type":"user","uuid":"a","session_id":"s","timestamp":"t"}
invalid json line
{"type":"assistant","uuid":"b","parent_uuid":"a","session_id":"s","timestamp":"t"}
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	entries, err := LoadEntries(path)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "malformed line should be skipped")
}

func TestSessionDir_PathStructure(t *testing.T) {
	dir := SessionDir("/Users/test/code/myproject")
	assert.Contains(t, dir, ".glaude")
	assert.Contains(t, dir, "projects")
	assert.Contains(t, dir, "-Users-test-code-myproject")
}
