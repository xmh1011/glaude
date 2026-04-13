package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

// initConfig represents the .glaude.json configuration file.
type initConfig struct {
	Provider       string      `json:"provider"`
	Model          string      `json:"model"`
	PermissionMode string      `json:"permission_mode"`
	Hooks          interface{} `json:"hooks,omitempty"`
}

func buildInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .glaude.json configuration file",
		Long:  "Interactive wizard to generate a .glaude.json project configuration file.",
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	const configFile = ".glaude.json"

	// Check if .glaude.json already exists.
	if _, err := os.Stat(configFile); err == nil {
		var overwrite bool
		err := huh.NewConfirm().
			Title(configFile + " already exists. Overwrite?").
			Value(&overwrite).
			Run()
		if err != nil {
			return err
		}
		if !overwrite {
			fmt.Println("Aborted.")
			return nil
		}
	}

	var provider, model, permMode string
	var includeHooks bool

	// Step 1: Choose provider.
	providerForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Provider").
				Options(
					huh.NewOption("anthropic", "anthropic"),
					huh.NewOption("openai", "openai"),
					huh.NewOption("ollama", "ollama"),
				).
				Value(&provider),
		),
	)
	if err := providerForm.Run(); err != nil {
		return err
	}

	// Step 2: Choose model (default depends on provider).
	defaultModel := "claude-sonnet-4-20250514"
	if provider == "openai" {
		defaultModel = "gpt-4o"
	} else if provider == "ollama" {
		defaultModel = "llama3"
	}
	model = defaultModel

	modelForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Model").
				Placeholder(defaultModel).
				Value(&model),
		),
	)
	if err := modelForm.Run(); err != nil {
		return err
	}
	if model == "" {
		model = defaultModel
	}

	// Step 3: Choose permission mode.
	permForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Permission mode").
				Options(
					huh.NewOption("default", "default"),
					huh.NewOption("auto-edit", "auto-edit"),
					huh.NewOption("plan-only", "plan-only"),
					huh.NewOption("auto-full", "auto-full"),
				).
				Value(&permMode),
		),
	)
	if err := permForm.Run(); err != nil {
		return err
	}

	// Step 4: Include example hooks?
	hookForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Include example hooks?").
				Value(&includeHooks),
		),
	)
	if err := hookForm.Run(); err != nil {
		return err
	}

	cfg := initConfig{
		Provider:       provider,
		Model:          model,
		PermissionMode: permMode,
	}

	if includeHooks {
		cfg.Hooks = map[string]interface{}{
			"PreToolUse": []map[string]interface{}{
				{
					"matcher": "Bash",
					"hooks": []map[string]interface{}{
						{
							"type":    "command",
							"command": "echo '{\"decision\":\"allow\"}'",
						},
					},
				},
			},
		}
	}

	// Write config file.
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		return fmt.Errorf("write %s: %w", configFile, err)
	}

	fmt.Printf("Created %s\n", configFile)
	return nil
}
