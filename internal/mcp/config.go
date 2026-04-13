package mcp

import (
	"context"

	"github.com/spf13/viper"

	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
)

// LoadFromConfig reads MCP server configurations from viper and
// registers discovered tools into the given Registry.
//
// Config format (in settings.json or .glaude.json):
//
//	{
//	  "mcp_servers": [
//	    {"name": "github", "command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"]}
//	  ]
//	}
func LoadFromConfig(ctx context.Context, reg *tool.Registry) (*Manager, error) {
	var configs []ServerConfig
	if err := viper.UnmarshalKey("mcp_servers", &configs); err != nil {
		telemetry.Log.
			WithField("error", err.Error()).
			Debug("mcp: no servers configured or invalid config")
		return NewManager(), nil
	}

	mgr := NewManager()
	if len(configs) == 0 {
		return mgr, nil
	}

	ConnectAll(ctx, mgr, configs, reg)
	return mgr, nil
}

// ConnectAll connects MCP servers from an explicit config list and registers
// their discovered tools into the given Registry. It is safe to call multiple
// times on the same Manager (e.g. once for viper config, once for plugins).
func ConnectAll(ctx context.Context, mgr *Manager, configs []ServerConfig, reg *tool.Registry) {
	for _, cfg := range configs {
		if cfg.Name == "" || cfg.Command == "" {
			telemetry.Log.
				WithField("config", cfg).
				Warn("mcp: skipping server with missing name or command")
			continue
		}

		tools, err := mgr.Connect(ctx, cfg)
		if err != nil {
			telemetry.Log.
				WithField("server", cfg.Name).
				WithField("error", err.Error()).
				Warn("mcp: failed to connect server")
			continue
		}

		for _, t := range tools {
			reg.Register(t)
			telemetry.Log.
				WithField("tool", t.Name()).
				Debug("mcp: registered tool")
		}
	}
}
