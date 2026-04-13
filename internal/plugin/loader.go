package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xmh1011/glaude/internal/config"
)

const manifestFile = "plugin.json"

// Discover scans plugin directories and returns all successfully loaded plugins
// along with any per-plugin errors. Project plugins override user plugins
// with the same name.
func Discover(cwd string) ([]*Plugin, []PluginError) {
	seen := make(map[string]*Plugin)
	var errors []PluginError

	// Layer 1: user plugins (lower priority)
	if globalDir, err := config.GlobalDir(); err == nil {
		userDir := filepath.Join(globalDir, "plugins")
		loadFromDir(userDir, "user", seen, &errors)
	}

	// Layer 2: project plugins (higher priority — overrides user)
	projectDir := filepath.Join(cwd, ".glaude", "plugins")
	loadFromDir(projectDir, "project", seen, &errors)

	// Collect all plugins in deterministic order (sorted by name)
	plugins := make([]*Plugin, 0, len(seen))
	for _, p := range seen {
		plugins = append(plugins, p)
	}

	return plugins, errors
}

// loadFromDir scans dir for subdirectories containing plugin.json.
func loadFromDir(dir, source string, seen map[string]*Plugin, errors *[]PluginError) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return // directory doesn't exist — not an error
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		pluginDir := filepath.Join(dir, dirName)
		manifestPath := filepath.Join(pluginDir, manifestFile)

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			if !os.IsNotExist(err) {
				*errors = append(*errors, PluginError{Name: dirName, Err: err})
			}
			continue
		}

		var m Manifest
		if err := json.Unmarshal(data, &m); err != nil {
			*errors = append(*errors, PluginError{
				Name: dirName,
				Err:  fmt.Errorf("parse %s: %w", manifestFile, err),
			})
			continue
		}

		// Validate: manifest name must match directory name
		if m.Name == "" {
			m.Name = dirName
		} else if m.Name != dirName {
			*errors = append(*errors, PluginError{
				Name: dirName,
				Err:  fmt.Errorf("manifest name %q does not match directory %q", m.Name, dirName),
			})
			continue
		}

		// Default skills_dir
		if m.SkillsDir == "" {
			m.SkillsDir = "skills"
		}

		absDir, _ := filepath.Abs(pluginDir)
		seen[m.Name] = &Plugin{
			Manifest: m,
			Dir:      absDir,
			Source:   source,
		}
	}
}
