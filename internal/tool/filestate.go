package tool

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileState records the state of a file at the time it was last read or written.
// Modeled after Claude Code's readFileState / FileStateCache.
type FileState struct {
	Content   string // file content at time of read/write
	Timestamp int64  // file mtime (Unix milliseconds) at time of read/write
	Offset    int    // line offset used for the read (0 = full read / edit/write)
	Limit     int    // line limit used for the read (0 = full read / edit/write)
}

// FileStateCache tracks file read states to enable staleness detection.
// FileReadTool records state when reading; FileEditTool and FileWriteTool
// validate against it before mutating and update it afterward.
//
// This prevents edits/writes to files that have changed since the last read,
// catching TOCTOU races and stale model context.
type FileStateCache struct {
	mu    sync.RWMutex
	store map[string]*FileState
}

// NewFileStateCache creates an empty file state cache.
func NewFileStateCache() *FileStateCache {
	return &FileStateCache{
		store: make(map[string]*FileState),
	}
}

// Set records the file state for the given path.
func (c *FileStateCache) Set(path string, state *FileState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[normalizePath(path)] = state
}

// Get retrieves the last known file state for the given path.
// Returns nil if the file has not been recorded.
func (c *FileStateCache) Get(path string) *FileState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.store[normalizePath(path)]
}

// Has returns true if a state entry exists for the given path.
func (c *FileStateCache) Has(path string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.store[normalizePath(path)]
	return ok
}

// Delete removes the state entry for the given path.
func (c *FileStateCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, normalizePath(path))
}

// Clear removes all entries.
func (c *FileStateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = make(map[string]*FileState)
}

// CheckStaleness validates that the file at path has not been modified since
// the last recorded state. Returns:
//   - nil if the file is fresh (safe to edit)
//   - ErrFileNotRead if the file was never read
//   - ErrFileModified if the file has been modified since last read
func (c *FileStateCache) CheckStaleness(path string) error {
	state := c.Get(path)
	if state == nil {
		return ErrFileNotRead
	}

	info, err := os.Stat(path)
	if err != nil {
		// File may have been deleted
		return ErrFileModified
	}

	currentMtime := info.ModTime().UnixMilli()
	if currentMtime > state.Timestamp {
		// mtime is newer — but if it was a full read, compare content as fallback
		// (handles cases where mtime changes without content change, e.g. touch)
		if state.Offset == 0 && state.Limit == 0 {
			data, err := os.ReadFile(path)
			if err == nil && string(data) == state.Content {
				return nil // content unchanged despite mtime change
			}
		}
		return ErrFileModified
	}

	return nil
}

// GetFileMtime returns the file modification time in Unix milliseconds.
func GetFileMtime(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now().UnixMilli()
	}
	return info.ModTime().UnixMilli()
}

// normalizePath normalizes a file path for use as a cache key.
func normalizePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
