// Package config provides layered configuration management.
//
// Merge priority (high to low):
//
//	environment variables (GLAUDE_ prefix) > project .glaude.json > global ~/.glaude/settings.json > defaults
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// GlobalDir returns the path to ~/.glaude/, creating it if necessary.
func GlobalDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	dir := filepath.Join(home, ".glaude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create global dir: %w", err)
	}
	return dir, nil
}

// Init initializes the layered configuration.
// It must be called before any viper.Get* usage.
func Init() error {
	globalDir, err := GlobalDir()
	if err != nil {
		return err
	}

	// Defaults
	viper.SetDefault("provider", "anthropic")
	viper.SetDefault("model", "claude-sonnet-4-20250514")
	viper.SetDefault("log_dir", filepath.Join(globalDir, "logs"))
	viper.SetDefault("log_level", "info")

	// Layer 1: global config ~/.glaude/settings.json
	viper.SetConfigFile(filepath.Join(globalDir, "settings.json"))
	_ = viper.ReadInConfig() // OK if not found

	// Layer 2: project config .glaude.json (merged on top)
	viper.SetConfigFile(".glaude.json")
	_ = viper.MergeInConfig() // OK if not found

	// Layer 3: environment variables (highest priority)
	viper.SetEnvPrefix("GLAUDE")
	viper.AutomaticEnv()

	return nil
}
