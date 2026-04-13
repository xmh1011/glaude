// Package configtool implements the Config tool for reading/writing settings.
package configtool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// AllowedSettings defines the set of keys that can be modified.
var AllowedSettings = map[string]bool{
	"model":           true,
	"provider":        true,
	"permission_mode": true,
}

// Tool reads and writes glaude settings.
type Tool struct{}

// Input is the parsed input for the Config tool.
type Input struct {
	Setting string `json:"setting"`
	Value   *any   `json:"value,omitempty"`
}

// Name returns the tool name.
func (t *Tool) Name() string { return "Config" }

// Description returns the LLM-facing description.
func (t *Tool) Description() string {
	return "Gets or sets glaude configuration values. Omit value to read; provide value to write. Writable keys: model, provider, permission_mode."
}

// InputSchema returns the JSON Schema for the tool's input.
func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"setting": {"type": "string", "description": "The setting key to get or set"},
			"value": {"description": "The value to set. Omit to get the current value."}
		},
		"required": ["setting"]
	}`)
}

// IsReadOnly returns false since this can write settings.
func (t *Tool) IsReadOnly() bool { return false }

// Execute gets or sets a config value.
func (t *Tool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	if in.Setting == "" {
		return "", fmt.Errorf("setting is required")
	}

	// GET mode
	if in.Value == nil {
		val := viper.Get(in.Setting)
		if val == nil {
			return fmt.Sprintf("%s: (not set)", in.Setting), nil
		}
		return fmt.Sprintf("%s: %v", in.Setting, val), nil
	}

	// SET mode
	if !AllowedSettings[in.Setting] {
		return fmt.Sprintf("Error: setting %q is not writable. Allowed: model, provider, permission_mode.", in.Setting), nil
	}

	viper.Set(in.Setting, *in.Value)

	// Persist to settings file.
	if err := persistSetting(in.Setting, *in.Value); err != nil {
		return fmt.Sprintf("Value set in memory but failed to persist: %v", err), nil
	}

	return fmt.Sprintf("%s set to %v", in.Setting, *in.Value), nil
}

func persistSetting(key string, value any) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	settingsPath := filepath.Join(home, ".glaude", "settings.json")
	var settings map[string]any

	data, err := os.ReadFile(settingsPath)
	if err == nil {
		json.Unmarshal(data, &settings) //nolint:errcheck // best effort
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	settings[key] = value
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(settingsPath, out, 0644)
}
