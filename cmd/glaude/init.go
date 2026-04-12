package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
	reader := bufio.NewReader(os.Stdin)

	// Step 1: Check if .glaude.json already exists.
	const configFile = ".glaude.json"
	if _, err := os.Stat(configFile); err == nil {
		fmt.Printf("%s already exists. Overwrite? [y/N] ", configFile)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Step 2: Choose provider.
	provider := promptChoice(reader, "Provider", []string{"anthropic", "openai", "ollama"}, "anthropic")

	// Step 3: Choose model.
	defaultModel := "claude-sonnet-4-20250514"
	if provider == "openai" {
		defaultModel = "gpt-4o"
	} else if provider == "ollama" {
		defaultModel = "llama3"
	}
	model := promptInput(reader, "Model", defaultModel)

	// Step 4: Choose permission mode.
	permMode := promptChoice(reader, "Permission mode", []string{"default", "auto-edit", "plan-only", "auto-full"}, "default")

	// Step 5: Include example hooks?
	fmt.Print("Include example hooks? [y/N] ")
	hookAnswer, _ := reader.ReadString('\n')
	hookAnswer = strings.TrimSpace(strings.ToLower(hookAnswer))

	cfg := initConfig{
		Provider:       provider,
		Model:          model,
		PermissionMode: permMode,
	}

	if hookAnswer == "y" || hookAnswer == "yes" {
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

// promptChoice presents a numbered list and returns the selected value.
func promptChoice(reader *bufio.Reader, label string, choices []string, defaultVal string) string {
	fmt.Printf("\n%s:\n", label)
	for i, c := range choices {
		marker := "  "
		if c == defaultVal {
			marker = "> "
		}
		fmt.Printf("  %s%d) %s\n", marker, i+1, c)
	}
	fmt.Printf("Choice [%s]: ", defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	// Try to match by number or value.
	for i, c := range choices {
		if input == fmt.Sprintf("%d", i+1) || strings.EqualFold(input, c) {
			return c
		}
	}
	return defaultVal
}

// promptInput asks for a free-text value with a default.
func promptInput(reader *bufio.Reader, label, defaultVal string) string {
	fmt.Printf("%s [%s]: ", label, defaultVal)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}
