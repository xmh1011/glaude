package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ListSessions scans the session directory for the given cwd and returns
// metadata for each session, sorted by timestamp descending (most recent first).
func ListSessions(cwd string) ([]SessionInfo, error) {
	dir := SessionDir(cwd)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []SessionInfo
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		info := extractSessionInfo(path)
		if info.ID == "" {
			continue
		}
		sessions = append(sessions, info)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Timestamp.After(sessions[j].Timestamp)
	})
	return sessions, nil
}

// extractSessionInfo reads head and tail of a JSONL file (up to 64KB each)
// to extract session metadata without loading the full file.
func extractSessionInfo(path string) SessionInfo {
	id := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	info := SessionInfo{ID: id, Path: path}

	f, err := os.Open(path)
	if err != nil {
		return info
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return info
	}
	info.Timestamp = stat.ModTime()

	const maxRead = 64 * 1024 // 64KB

	// Read head
	head := make([]byte, maxRead)
	n, _ := f.Read(head)
	head = head[:n]

	// Read tail if file is larger than head
	var tail []byte
	if stat.Size() > int64(maxRead) {
		tailOffset := stat.Size() - int64(maxRead)
		if tailOffset < 0 {
			tailOffset = 0
		}
		tail = make([]byte, maxRead)
		n, _ = f.ReadAt(tail, tailOffset)
		tail = tail[:n]
	}

	// Parse combined head+tail for metadata
	combined := string(head)
	if len(tail) > 0 {
		combined += "\n" + string(tail)
	}
	parseMetadataLines(combined, &info)
	return info
}

// parseMetadataLines extracts metadata from JSONL lines.
func parseMetadataLines(data string, info *SessionInfo) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}

		var entry struct {
			Type      string `json:"type"`
			SessionID string `json:"session_id"`
			Timestamp string `json:"timestamp"`
			Text      string `json:"text"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.SessionID != "" && info.ID == strings.TrimSuffix(filepath.Base(info.Path), ".jsonl") {
			info.ID = entry.SessionID
		}

		switch entry.Type {
		case "title":
			info.Title = entry.Text
		case "last-prompt":
			info.LastPrompt = entry.Text
		}

		if entry.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339, entry.Timestamp); err == nil {
				info.Timestamp = t
			}
		}
	}
}

// MostRecentSession returns the most recent session for the given cwd, or nil.
func MostRecentSession(cwd string) *SessionInfo {
	sessions, err := ListSessions(cwd)
	if err != nil || len(sessions) == 0 {
		return nil
	}
	return &sessions[0]
}

// readHeadAndTail reads up to maxBytes from the head and tail of a file.
func readHeadAndTail(f *os.File, maxBytes int64) ([]byte, error) {
	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}

	if stat.Size() <= maxBytes {
		return io.ReadAll(f)
	}

	head := make([]byte, maxBytes/2)
	n, _ := f.Read(head)
	head = head[:n]

	tail := make([]byte, maxBytes/2)
	tailOffset := stat.Size() - maxBytes/2
	n, _ = f.ReadAt(tail, tailOffset)
	tail = tail[:n]

	result := make([]byte, 0, len(head)+1+len(tail))
	result = append(result, head...)
	result = append(result, '\n')
	result = append(result, tail...)
	return result, nil
}
