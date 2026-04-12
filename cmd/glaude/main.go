// Package main is the entry point for the glaude CLI.
//
// It establishes the global context cancellation tree, initializes
// configuration and telemetry, then dispatches to the appropriate
// command handler via cobra.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xmh1011/glaude/internal/agent"
	"github.com/xmh1011/glaude/internal/config"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/memory"
	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/prompt"
	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
	"github.com/xmh1011/glaude/internal/ui"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// --- Cancellation Tree ---
	// First signal: graceful shutdown (cancel context).
	// Second signal: force exit.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %s, shutting down gracefully...\n", sig)
		cancel()
		// Second signal: force exit
		sig = <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %s again, forcing exit.\n", sig)
		os.Exit(1)
	}()

	// --- CLI Command Tree ---
	rootCmd := buildRootCmd(ctx)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func buildRootCmd(ctx context.Context) *cobra.Command {
	var userPrompt string

	rootCmd := &cobra.Command{
		Use:   "glaude",
		Short: "AI Coding Agent powered by LLM",
		Long:  "glaude is a Go implementation of an AI coding agent, inspired by Claude Code architecture.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Initialize config (layered merge)
			if err := config.Init(); err != nil {
				return fmt.Errorf("config: %w", err)
			}
			// Initialize ghost logging
			if err := telemetry.Init(); err != nil {
				return fmt.Errorf("telemetry: %w", err)
			}
			telemetry.Log.WithField("version", version).Info("glaude session started")
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			telemetry.Close()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if userPrompt != "" {
				// One-shot mode: run a single prompt and exit
				telemetry.Log.WithField("mode", "oneshot").Info("prompt received")

				provider := llm.NewAnthropicProvider("")
				reg := buildRegistry(nil)

				// Load directive files (GLAUDE.md) from all tiers
				cwd, _ := os.Getwd()
				mem := &memory.FileStore{}
				instructions, err := mem.Load(cwd)
				if err != nil {
					telemetry.Log.WithField("error", err.Error()).Warn("failed to load directives")
				}

				sysPrompt := prompt.NewBuilder().WithCustomInstructions(instructions).Build()
				a := agent.New(provider, "claude-sonnet-4-20250514", sysPrompt, reg)
				text, err := a.Run(cmd.Context(), userPrompt)

				usage := a.TotalUsage()
				telemetry.Log.
					WithField("input_tokens", usage.InputTokens).
					WithField("output_tokens", usage.OutputTokens).
					Info("oneshot complete")

				if text != "" {
					fmt.Println(text)
				}
				return err
			}
			// Default: REPL mode
			telemetry.Log.WithField("mode", "repl").Info("entering REPL")

			provider := llm.NewAnthropicProvider("")
			cp := memory.NewCheckpoint()
			reg := buildRegistry(cp)

			cwd, _ := os.Getwd()
			mem := &memory.FileStore{}
			instructions, err := mem.Load(cwd)
			if err != nil {
				telemetry.Log.WithField("error", err.Error()).Warn("failed to load directives")
			}

			sysPrompt := prompt.NewBuilder().WithCustomInstructions(instructions).Build()
			a := agent.New(provider, "claude-sonnet-4-20250514", sysPrompt, reg)

			m := ui.NewModel(a, cp, cmd.Context())
			p := ui.NewProgram(m)

			// Wire permission gate: reads mode from config, bridges Ask to UI prompt
			permMode := permission.ParseMode(viper.GetString("permission_mode"))
			telemetry.Log.WithField("permission_mode", permMode.String()).Info("permission mode configured")
			ui.WirePermissionGate(a, p, permMode)

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("UI: %w", err)
			}
			return nil
		},
	}

	// Flags
	rootCmd.Flags().StringVarP(&userPrompt, "prompt", "p", "", "Run a single prompt and exit")

	// Subcommands
	rootCmd.AddCommand(buildVersionCmd())

	return rootCmd
}

func buildVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("glaude %s\n", version)
		},
	}
}

// buildRegistry creates a tool registry with all built-in tools.
// If cp is nil, a new Checkpoint is created internally.
func buildRegistry(cp *memory.Checkpoint) *tool.Registry {
	if cp == nil {
		cp = memory.NewCheckpoint()
	}
	reg := tool.NewRegistry()
	reg.Register(&tool.FileReadTool{})
	reg.Register(&tool.FileEditTool{Checkpoint: cp})
	reg.Register(&tool.FileWriteTool{Checkpoint: cp})
	reg.Register(tool.NewBashTool())
	reg.Register(&tool.GlobTool{})
	reg.Register(&tool.GrepTool{})
	reg.Register(&tool.LSTool{})
	return reg
}
