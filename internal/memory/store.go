// Package memory implements persistent memory for cross-session knowledge.
//
// Memory is loaded from Markdown directive files (GLAUDE.md) at multiple
// tiers and merged by priority. The MemoryStore interface allows future
// backends (vector DB, etc.) to replace the file-based implementation.
package memory

// Store abstracts persistent memory read/write.
// The initial implementation is file-based (Markdown). The interface is
// the extension point for future backends.
type Store interface {
	// Load returns merged memory content from all applicable tiers
	// for the given project root directory.
	Load(projectRoot string) (string, error)

	// Save writes a memory entry to the project-level memory file.
	Save(projectRoot string, content string) error
}
