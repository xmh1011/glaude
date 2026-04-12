package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/xmh1011/glaude/internal/config"
)

// maxIncludeDepth prevents infinite recursion from @include directives.
const maxIncludeDepth = 5

// FileStore implements Store using local Markdown files.
//
// Directive files are loaded from multiple tiers (lowest to highest priority),
// modeled after Claude Code's claudemd.ts:
//
//  1. /etc/glaude/GLAUDE.md + /etc/glaude/.glaude/rules/*.md  — managed (enterprise)
//  2. ~/.glaude/GLAUDE.md + ~/.glaude/rules/*.md               — user global
//  3. <project>/GLAUDE.md, .glaude/GLAUDE.md, .glaude/rules/*.md — project
//  4. <project>/GLAUDE.local.md                                  — project local (gitignored)
//
// All tiers are concatenated with path metadata. Higher-priority content
// appears later so it takes precedence when the LLM interprets instructions.
type FileStore struct{}

// directiveFile represents a loaded directive with metadata.
type directiveFile struct {
	Path    string // source file path
	Tier    string // tier label for display
	Content string // file content
}

// Load reads and merges directive files from all tiers.
func (f *FileStore) Load(projectRoot string) (string, error) {
	globalDir, err := config.GlobalDir()
	if err != nil {
		return "", fmt.Errorf("resolving global dir: %w", err)
	}

	processedPaths := make(map[string]bool)
	var files []directiveFile

	// Tier 1: Managed (enterprise) — /etc/glaude/ (Linux/macOS)
	managedDir := managedConfigDir()
	if managedDir != "" {
		managed, err := loadTier(managedDir, "Managed", processedPaths)
		if err != nil {
			return "", err
		}
		files = append(files, managed...)
	}

	// Tier 2: User Global — ~/.glaude/
	userFiles, err := loadTier(globalDir, "User", processedPaths)
	if err != nil {
		return "", err
	}
	files = append(files, userFiles...)

	// Tier 3: Project — <projectRoot>/
	projectFiles, err := loadTier(projectRoot, "Project", processedPaths)
	if err != nil {
		return "", err
	}
	files = append(files, projectFiles...)

	// Tier 4: Project Local — <projectRoot>/GLAUDE.local.md
	localPath := filepath.Join(projectRoot, "GLAUDE.local.md")
	localContent, err := loadFileWithIncludes(localPath, processedPaths, 0)
	if err != nil {
		return "", fmt.Errorf("reading Project Local (%s): %w", localPath, err)
	}
	if localContent != "" {
		files = append(files, directiveFile{
			Path:    localPath,
			Tier:    "Project Local",
			Content: localContent,
		})
	}

	if len(files) == 0 {
		return "", nil
	}

	// Format output with path metadata
	var sections []string
	for _, f := range files {
		sections = append(sections, f.Content)
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

// loadTier loads all directive files from a directory tier:
//   - <dir>/GLAUDE.md
//   - <dir>/.glaude/GLAUDE.md
//   - <dir>/.glaude/rules/*.md (sorted alphabetically)
func loadTier(dir, tierLabel string, processedPaths map[string]bool) ([]directiveFile, error) {
	var files []directiveFile

	// Main GLAUDE.md
	mainPath := filepath.Join(dir, "GLAUDE.md")
	content, err := loadFileWithIncludes(mainPath, processedPaths, 0)
	if err != nil {
		return nil, fmt.Errorf("reading %s (%s): %w", tierLabel, mainPath, err)
	}
	if content != "" {
		files = append(files, directiveFile{Path: mainPath, Tier: tierLabel, Content: content})
	}

	// .glaude/GLAUDE.md
	dotPath := filepath.Join(dir, ".glaude", "GLAUDE.md")
	content, err = loadFileWithIncludes(dotPath, processedPaths, 0)
	if err != nil {
		return nil, fmt.Errorf("reading %s (.glaude) (%s): %w", tierLabel, dotPath, err)
	}
	if content != "" {
		files = append(files, directiveFile{Path: dotPath, Tier: tierLabel, Content: content})
	}

	// .glaude/rules/*.md
	rulesDir := filepath.Join(dir, ".glaude", "rules")
	ruleFiles, err := loadRulesDir(rulesDir, tierLabel, processedPaths)
	if err != nil {
		return nil, err
	}
	files = append(files, ruleFiles...)

	return files, nil
}

// loadRulesDir loads all .md files from a rules directory, sorted alphabetically.
func loadRulesDir(dir, tierLabel string, processedPaths map[string]bool) ([]directiveFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading rules dir %s: %w", dir, err)
	}

	// Sort entries by name for deterministic ordering
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	var files []directiveFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		content, err := loadFileWithIncludes(path, processedPaths, 0)
		if err != nil {
			return nil, fmt.Errorf("reading rule %s: %w", path, err)
		}
		if content != "" {
			files = append(files, directiveFile{
				Path:    path,
				Tier:    tierLabel + " Rule",
				Content: content,
			})
		}
	}
	return files, nil
}

// loadFileWithIncludes reads a file and processes @include directives recursively.
// It also strips HTML comments from the content.
func loadFileWithIncludes(path string, processedPaths map[string]bool, depth int) (string, error) {
	// Normalize path for dedup
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = filepath.Clean(path)
	}

	// Circular reference detection
	if processedPaths[absPath] {
		return "", nil
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	processedPaths[absPath] = true
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", nil
	}

	// Strip HTML comments
	content = stripHTMLComments(content)

	// Process @include directives (up to maxIncludeDepth)
	if depth < maxIncludeDepth {
		content = processIncludes(content, filepath.Dir(absPath), processedPaths, depth)
	}

	return content, nil
}

// includeRegex matches @path directives in text.
// The @ must be at start of line or preceded by whitespace.
// Supports backslash-escaped spaces in paths.
var includeRegex = regexp.MustCompile(`(?:^|\s)@((?:[^\s\\]|\\ )+)`)

// processIncludes replaces @path references with the content of the referenced files.
func processIncludes(content, baseDir string, processedPaths map[string]bool, depth int) string {
	lines := strings.Split(content, "\n")
	var result []string

	for _, line := range lines {
		// Skip code blocks (basic heuristic: lines starting with ``` or indented by 4+ spaces)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(line, "    ") {
			result = append(result, line)
			continue
		}

		matches := includeRegex.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			result = append(result, line)
			continue
		}

		expanded := line
		for _, m := range matches {
			rawPath := m[1]

			// Strip fragment identifiers (#heading)
			if idx := strings.Index(rawPath, "#"); idx >= 0 {
				rawPath = rawPath[:idx]
			}

			// Unescape backslash-escaped spaces
			rawPath = strings.ReplaceAll(rawPath, "\\ ", " ")

			resolvedPath := resolveIncludePath(rawPath, baseDir)
			if resolvedPath == "" {
				continue
			}

			// Only include text files
			if !isTextFile(resolvedPath) {
				continue
			}

			included, err := loadFileWithIncludes(resolvedPath, processedPaths, depth+1)
			if err != nil || included == "" {
				continue
			}

			// Replace the @path reference with included content
			expanded = strings.Replace(expanded, m[0], "\n"+included+"\n", 1)
		}
		result = append(result, expanded)
	}

	return strings.Join(result, "\n")
}

// resolveIncludePath resolves an @include path to an absolute path.
// Supports:
//   - ./relative  (relative to including file)
//   - ~/home      (relative to home directory)
//   - /absolute   (absolute path)
//   - bare        (treated as relative)
func resolveIncludePath(raw, baseDir string) string {
	if raw == "" || raw == "/" {
		return ""
	}

	// Filter out clearly invalid paths
	if strings.HasPrefix(raw, "@") || strings.HasPrefix(raw, "#") {
		return ""
	}

	if strings.HasPrefix(raw, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		return filepath.Join(home, raw[2:])
	}

	if filepath.IsAbs(raw) {
		return raw
	}

	// Relative path (including ./ prefix and bare paths)
	return filepath.Join(baseDir, raw)
}

// htmlCommentRegex matches HTML comments (non-greedy, spans newlines).
var htmlCommentRegex = regexp.MustCompile(`<!--[\s\S]*?-->`)

// stripHTMLComments removes block-level HTML comments from content.
// Comments inside code blocks are preserved (basic heuristic).
func stripHTMLComments(content string) string {
	if !strings.Contains(content, "<!--") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			result = append(result, line)
			continue
		}

		if inCodeBlock {
			result = append(result, line)
			continue
		}

		// Strip comments from non-code lines
		if strings.Contains(line, "<!--") {
			cleaned := htmlCommentRegex.ReplaceAllString(line, "")
			cleaned = strings.TrimSpace(cleaned)
			if cleaned != "" {
				result = append(result, cleaned)
			}
			// Skip empty lines after comment removal
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}

// textExtensions are file extensions considered safe to include.
var textExtensions = map[string]bool{
	".md": true, ".txt": true, ".yml": true, ".yaml": true,
	".json": true, ".toml": true, ".cfg": true, ".conf": true,
	".ini": true, ".properties": true, ".env": true,
	".go": true, ".py": true, ".js": true, ".ts": true,
	".jsx": true, ".tsx": true, ".rs": true, ".java": true,
	".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".rb": true, ".php": true, ".sh": true, ".bash": true,
	".zsh": true, ".fish": true, ".css": true, ".scss": true,
	".html": true, ".xml": true, ".svg": true, ".sql": true,
	".graphql": true, ".proto": true, ".swift": true, ".kt": true,
	".scala": true, ".r": true, ".lua": true, ".vim": true,
	".el": true, ".clj": true, ".ex": true, ".erl": true,
	".hs": true, ".ml": true, ".dart": true, ".cs": true,
}

// isTextFile returns true if the file extension indicates a text file.
func isTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return textExtensions[ext]
}

// managedConfigDir returns the managed/enterprise config directory.
// Returns empty string if not applicable for the current OS.
func managedConfigDir() string {
	switch runtime.GOOS {
	case "linux", "darwin":
		return "/etc/glaude"
	default:
		return ""
	}
}
