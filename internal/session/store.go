package session

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// Store is an append-only JSONL writer for a single session.
// The underlying file is created lazily on the first Append call.
type Store struct {
	dir       string // ~/.glaude/projects/{sanitized_cwd}
	sessionID string
	file      *os.File
	mu        sync.Mutex
}

// NewStore creates a Store for the given working directory and session ID.
// No file is created until the first Append.
func NewStore(cwd, sessionID string) *Store {
	return &Store{
		dir:       SessionDir(cwd),
		sessionID: sessionID,
	}
}

// SessionID returns the store's session identifier.
func (s *Store) SessionID() string {
	return s.sessionID
}

// Append serializes entry as a single JSON line and appends it to the JSONL file.
func (s *Store) Append(entry *Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureFile(); err != nil {
		return err
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}
	data = append(data, '\n')

	if _, err := s.file.Write(data); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}
	return s.file.Sync()
}

// Close closes the underlying file if open.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		err := s.file.Close()
		s.file = nil
		return err
	}
	return nil
}

// FilePath returns the full path to this session's JSONL file.
func (s *Store) FilePath() string {
	return filepath.Join(s.dir, s.sessionID+".jsonl")
}

// ensureFile lazily creates the directory and file on first write.
// Must be called with s.mu held.
func (s *Store) ensureFile() error {
	if s.file != nil {
		return nil
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create session dir: %w", err)
	}
	f, err := os.OpenFile(s.FilePath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open session file: %w", err)
	}
	s.file = f
	return nil
}

// LoadEntries reads all entries from a JSONL file.
func LoadEntries(path string) ([]*Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []*Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB per line
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue // skip malformed lines
		}
		entries = append(entries, &e)
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("scan session file: %w", err)
	}
	return entries, nil
}

// --- Path utilities ---

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// SanitizePath replaces non-alphanumeric characters with hyphens.
// Paths longer than 200 characters are truncated with a hash suffix.
func SanitizePath(path string) string {
	s := nonAlphaNum.ReplaceAllString(path, "-")
	if len(s) > 200 {
		h := sha256.Sum256([]byte(path))
		s = s[:192] + fmt.Sprintf("-%x", h[:4])
	}
	return s
}

// ProjectsDir returns the base directory for all session projects.
func ProjectsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".glaude", "projects"), nil
}

// SessionDir returns the project-specific session directory for the given cwd.
func SessionDir(cwd string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".glaude", "projects", SanitizePath(cwd))
}

// SessionFilePath returns the full path to a session's JSONL file.
func SessionFilePath(cwd, sessionID string) string {
	return filepath.Join(SessionDir(cwd), sessionID+".jsonl")
}
