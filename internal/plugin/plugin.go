// Package plugin provides a unified "three-in-one" plugin system that
// combines Skills, Hooks, and MCP server declarations into a single
// plugin.json manifest.
//
// Plugins are discovered from two directories (higher priority first):
//
//	{cwd}/.glaude/plugins/{name}/plugin.json   (project)
//	~/.glaude/plugins/{name}/plugin.json        (user)
//
// A project plugin with the same name overrides a user plugin.
package plugin

import (
	"fmt"

	"github.com/xmh1011/glaude/internal/hook"
	"github.com/xmh1011/glaude/internal/mcp"
)

// Manifest is the deserialized content of a plugin.json file.
type Manifest struct {
	Name        string             `json:"name"`
	Description string             `json:"description,omitempty"`
	SkillsDir   string             `json:"skills_dir,omitempty"`
	Hooks       hook.HookConfig    `json:"hooks,omitempty"`
	MCPServers  []mcp.ServerConfig `json:"mcp_servers,omitempty"`
}

// Plugin is a fully resolved plugin with its manifest and metadata.
type Plugin struct {
	Manifest Manifest
	Dir      string // absolute path to the plugin root directory
	Source   string // "project" or "user"
}

// PluginError records a per-plugin load failure so that one bad plugin
// does not prevent others from loading.
type PluginError struct {
	Name string
	Err  error
}

// Error implements the error interface.
func (e *PluginError) Error() string {
	return fmt.Sprintf("plugin %q: %v", e.Name, e.Err)
}
