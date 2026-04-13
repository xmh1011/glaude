package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xmh1011/glaude/internal/hook"
	"github.com/xmh1011/glaude/internal/mcp"
	"github.com/xmh1011/glaude/internal/skill"
)

// helper to write a plugin.json manifest to a temp directory.
func writeManifest(t *testing.T, dir, name string, m Manifest) {
	t.Helper()
	pluginDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	data, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644))
}

// helper to write a SKILL.md inside a plugin's skills directory.
func writePluginSkill(t *testing.T, pluginDir, skillName, body string) {
	t.Helper()
	skillDir := filepath.Join(pluginDir, "skills", skillName)
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644))
}

func TestDiscover_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	plugins, errors := Discover(tmp)
	assert.Empty(t, plugins)
	assert.Empty(t, errors)
}

func TestDiscover_SinglePlugin(t *testing.T) {
	tmp := t.TempDir()
	pluginsDir := filepath.Join(tmp, ".glaude", "plugins")

	m := Manifest{Name: "test-plugin", Description: "a test"}
	writeManifest(t, pluginsDir, "test-plugin", m)

	plugins, errors := Discover(tmp)
	assert.Empty(t, errors)
	require.Len(t, plugins, 1)
	assert.Equal(t, "test-plugin", plugins[0].Manifest.Name)
	assert.Equal(t, "project", plugins[0].Source)
	assert.Equal(t, "skills", plugins[0].Manifest.SkillsDir) // default
}

func TestDiscover_NameMismatch(t *testing.T) {
	tmp := t.TempDir()
	pluginsDir := filepath.Join(tmp, ".glaude", "plugins")

	m := Manifest{Name: "wrong-name"}
	writeManifest(t, pluginsDir, "actual-dir", m)

	plugins, errors := Discover(tmp)
	assert.Empty(t, plugins)
	require.Len(t, errors, 1)
	assert.Contains(t, errors[0].Error(), "does not match directory")
}

func TestDiscover_InvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	pluginDir := filepath.Join(tmp, ".glaude", "plugins", "bad")
	require.NoError(t, os.MkdirAll(pluginDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte("{invalid"), 0o644))

	plugins, errors := Discover(tmp)
	assert.Empty(t, plugins)
	require.Len(t, errors, 1)
	assert.Contains(t, errors[0].Error(), "parse plugin.json")
}

func TestDiscover_EmptyName_UsesDir(t *testing.T) {
	tmp := t.TempDir()
	pluginsDir := filepath.Join(tmp, ".glaude", "plugins")

	m := Manifest{Description: "no name field"}
	writeManifest(t, pluginsDir, "auto-named", m)

	plugins, errors := Discover(tmp)
	assert.Empty(t, errors)
	require.Len(t, plugins, 1)
	assert.Equal(t, "auto-named", plugins[0].Manifest.Name)
}

func TestMergedHooks(t *testing.T) {
	mgr := &Manager{
		plugins: []*Plugin{
			{
				Manifest: Manifest{
					Name: "p1",
					Hooks: hook.HookConfig{
						hook.PreToolUse: {
							{Matcher: "Bash", Hooks: []hook.Entry{{Type: "command", Command: "echo p1"}}},
						},
					},
				},
			},
			{
				Manifest: Manifest{
					Name: "p2",
					Hooks: hook.HookConfig{
						hook.PreToolUse: {
							{Matcher: "*", Hooks: []hook.Entry{{Type: "command", Command: "echo p2"}}},
						},
						hook.PostToolUse: {
							{Matcher: "Edit", Hooks: []hook.Entry{{Type: "command", Command: "echo post"}}},
						},
					},
				},
			},
		},
	}

	merged := mgr.MergedHooks()

	// PreToolUse should have groups from both plugins
	require.Len(t, merged[hook.PreToolUse], 2)
	assert.Equal(t, "Bash", merged[hook.PreToolUse][0].Matcher)
	assert.Equal(t, "*", merged[hook.PreToolUse][1].Matcher)

	// PostToolUse only from p2
	require.Len(t, merged[hook.PostToolUse], 1)
	assert.Equal(t, "Edit", merged[hook.PostToolUse][0].Matcher)
}

func TestMCPConfigs(t *testing.T) {
	mgr := &Manager{
		plugins: []*Plugin{
			{
				Manifest: Manifest{
					Name: "my-plugin",
					MCPServers: []mcp.ServerConfig{
						{Name: "github", Command: "npx", Args: []string{"-y", "server-github"}},
						{Name: "fs", Command: "node", Args: []string{"fs-server.js"}},
					},
				},
			},
		},
	}

	configs := mgr.MCPConfigs()
	require.Len(t, configs, 2)
	assert.Equal(t, "my-plugin/github", configs[0].Name)
	assert.Equal(t, "my-plugin/fs", configs[1].Name)
	assert.Equal(t, "npx", configs[0].Command)
}

func TestLoadSkills(t *testing.T) {
	tmp := t.TempDir()
	pluginDir := filepath.Join(tmp, "test-plugin")
	writePluginSkill(t, pluginDir, "greet", "---\ndescription: greeting\n---\nHello $ARGUMENTS")

	mgr := &Manager{
		plugins: []*Plugin{
			{
				Manifest: Manifest{Name: "test-plugin", SkillsDir: "skills"},
				Dir:      pluginDir,
				Source:   "project",
			},
		},
	}

	reg := skill.NewRegistry()
	mgr.LoadSkills(reg)

	s := reg.Get("greet")
	require.NotNil(t, s)
	assert.Equal(t, "greeting", s.Description)
	assert.Equal(t, "plugin:test-plugin", s.Source)

	prompt, err := s.GetPrompt("world")
	require.NoError(t, err)
	assert.Contains(t, prompt, "Hello world")
}

func TestMergedHooks_Empty(t *testing.T) {
	mgr := &Manager{plugins: nil}
	merged := mgr.MergedHooks()
	assert.Empty(t, merged)
}

func TestMCPConfigs_Empty(t *testing.T) {
	mgr := &Manager{plugins: nil}
	configs := mgr.MCPConfigs()
	assert.Empty(t, configs)
}
