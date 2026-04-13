package plugin

import (
	"fmt"
	"path/filepath"

	"github.com/xmh1011/glaude/internal/hook"
	"github.com/xmh1011/glaude/internal/mcp"
	"github.com/xmh1011/glaude/internal/skill"
	"github.com/xmh1011/glaude/internal/telemetry"
)

// Manager orchestrates plugin discovery and extracts skills, hooks, and
// MCP server configs from discovered plugins.
type Manager struct {
	plugins []*Plugin
	errors  []PluginError
}

// NewManager discovers plugins under cwd and returns a Manager.
func NewManager(cwd string) *Manager {
	plugins, errors := Discover(cwd)
	return &Manager{plugins: plugins, errors: errors}
}

// Plugins returns all successfully loaded plugins.
func (m *Manager) Plugins() []*Plugin {
	return m.plugins
}

// Errors returns all per-plugin load errors.
func (m *Manager) Errors() []PluginError {
	return m.errors
}

// LoadSkills loads SKILL.md files from each plugin's skills_dir and registers
// them into the given skill registry.
func (m *Manager) LoadSkills(reg *skill.Registry) {
	for _, p := range m.plugins {
		skillsDir := filepath.Join(p.Dir, p.Manifest.SkillsDir)
		source := fmt.Sprintf("plugin:%s", p.Manifest.Name)
		skills, err := skill.LoadFromDir(skillsDir, source)
		if err != nil {
			telemetry.Log.
				WithField("plugin", p.Manifest.Name).
				WithField("error", err.Error()).
				Debug("plugin: no skills found")
			continue
		}
		for _, s := range skills {
			reg.Register(s)
		}
	}
}

// MergedHooks returns a single HookConfig that merges hook declarations
// from all plugins. Groups for the same event are concatenated in plugin order.
func (m *Manager) MergedHooks() hook.HookConfig {
	configs := make([]hook.HookConfig, 0, len(m.plugins))
	for _, p := range m.plugins {
		if len(p.Manifest.Hooks) > 0 {
			configs = append(configs, p.Manifest.Hooks)
		}
	}
	return hook.MergeConfigs(configs...)
}

// MCPConfigs collects MCP server declarations from all plugins.
// Server names are prefixed with the plugin name to avoid collisions:
// "plugin-name/server-name".
func (m *Manager) MCPConfigs() []mcp.ServerConfig {
	var configs []mcp.ServerConfig
	for _, p := range m.plugins {
		for _, cfg := range p.Manifest.MCPServers {
			namespaced := cfg
			namespaced.Name = fmt.Sprintf("%s/%s", p.Manifest.Name, cfg.Name)
			configs = append(configs, namespaced)
		}
	}
	return configs
}
