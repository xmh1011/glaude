package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xmh1011/glaude/internal/config"
)

// FileStore implements Store using local Markdown files.
//
// Directive files are loaded from multiple tiers (lowest to highest priority):
//  1. ~/.glaude/GLAUDE.md              — user global directives
//  2. <project>/GLAUDE.md              — project-level (version controlled)
//  3. <project>/.glaude/GLAUDE.md      — project-level alternate location
//  4. <project>/GLAUDE.local.md        — project-local (gitignored, highest priority)
//
// All tiers are concatenated with section headers. Higher-priority content
// appears later so it takes precedence when the LLM interprets instructions.
type FileStore struct{}

// Load reads and merges directive files from all tiers.
func (f *FileStore) Load(projectRoot string) (string, error) {
	globalDir, err := config.GlobalDir()
	if err != nil {
		return "", fmt.Errorf("resolving global dir: %w", err)
	}

	sources := []struct {
		label string
		path  string
	}{
		{"User Global", filepath.Join(globalDir, "GLAUDE.md")},
		{"Project", filepath.Join(projectRoot, "GLAUDE.md")},
		{"Project (.glaude)", filepath.Join(projectRoot, ".glaude", "GLAUDE.md")},
		{"Project Local", filepath.Join(projectRoot, "GLAUDE.local.md")},
	}

	var sections []string
	for _, src := range sources {
		content, err := readFileIfExists(src.path)
		if err != nil {
			return "", fmt.Errorf("reading %s (%s): %w", src.label, src.path, err)
		}
		if content != "" {
			sections = append(sections, content)
		}
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n\n---\n\n"), nil
}

// Save writes content to the project-level GLAUDE.md file.
func (f *FileStore) Save(projectRoot string, content string) error {
	path := filepath.Join(projectRoot, "GLAUDE.md")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// readFileIfExists returns the file content or empty string if the file
// does not exist. Other errors are propagated.
func readFileIfExists(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
