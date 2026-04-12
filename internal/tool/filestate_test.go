package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStateCache_SetGet(t *testing.T) {
	cache := NewFileStateCache()

	cache.Set("/tmp/test.txt", &FileState{
		Content:   "hello",
		Timestamp: time.Now().UnixMilli(),
	})

	state := cache.Get("/tmp/test.txt")
	require.NotNil(t, state)
	assert.Equal(t, "hello", state.Content)
}

func TestFileStateCache_Has(t *testing.T) {
	cache := NewFileStateCache()
	assert.False(t, cache.Has("/tmp/nope.txt"))

	cache.Set("/tmp/nope.txt", &FileState{Content: "x"})
	assert.True(t, cache.Has("/tmp/nope.txt"))
}

func TestFileStateCache_Delete(t *testing.T) {
	cache := NewFileStateCache()
	cache.Set("/tmp/x.txt", &FileState{Content: "x"})
	cache.Delete("/tmp/x.txt")
	assert.Nil(t, cache.Get("/tmp/x.txt"))
}

func TestFileStateCache_Clear(t *testing.T) {
	cache := NewFileStateCache()
	cache.Set("/tmp/a.txt", &FileState{Content: "a"})
	cache.Set("/tmp/b.txt", &FileState{Content: "b"})
	cache.Clear()
	assert.Nil(t, cache.Get("/tmp/a.txt"))
	assert.Nil(t, cache.Get("/tmp/b.txt"))
}

func TestFileStateCache_CheckStaleness_NotRead(t *testing.T) {
	cache := NewFileStateCache()
	err := cache.CheckStaleness("/tmp/not_read.txt")
	assert.ErrorIs(t, err, ErrFileNotRead)
}

func TestFileStateCache_CheckStaleness_Fresh(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	cache := NewFileStateCache()
	cache.Set(path, &FileState{
		Content:   "hello",
		Timestamp: GetFileMtime(path),
	})

	err := cache.CheckStaleness(path)
	assert.NoError(t, err)
}

func TestFileStateCache_CheckStaleness_Modified(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	cache := NewFileStateCache()
	cache.Set(path, &FileState{
		Content:   "hello",
		Timestamp: GetFileMtime(path) - 1000, // pretend read was before current mtime
	})

	// Modify the file content
	os.WriteFile(path, []byte("world"), 0644)

	err := cache.CheckStaleness(path)
	assert.ErrorIs(t, err, ErrFileModified)
}

func TestFileStateCache_CheckStaleness_MtimeChangedContentSame(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	// Record with old timestamp but matching content (full read: offset=0, limit=0)
	cache := NewFileStateCache()
	cache.Set(path, &FileState{
		Content:   "hello",
		Timestamp: GetFileMtime(path) - 1000, // old timestamp
		Offset:    0,
		Limit:     0,
	})

	// Touch the file (mtime changes but content stays same)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(path, []byte("hello"), 0644)

	// Should pass because content is identical
	err := cache.CheckStaleness(path)
	assert.NoError(t, err)
}

func TestFileStateCache_CheckStaleness_Deleted(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	cache := NewFileStateCache()
	cache.Set(path, &FileState{
		Content:   "hello",
		Timestamp: GetFileMtime(path),
	})

	os.Remove(path)
	err := cache.CheckStaleness(path)
	assert.ErrorIs(t, err, ErrFileModified)
}
