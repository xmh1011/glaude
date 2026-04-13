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
	"time"

	"github.com/charmbracelet/huh"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/xmh1011/glaude/internal/agent"
	"github.com/xmh1011/glaude/internal/config"
	"github.com/xmh1011/glaude/internal/hook"
	"github.com/xmh1011/glaude/internal/llm"
	"github.com/xmh1011/glaude/internal/mcp"
	"github.com/xmh1011/glaude/internal/memory"
	"github.com/xmh1011/glaude/internal/permission"
	"github.com/xmh1011/glaude/internal/plugin"
	"github.com/xmh1011/glaude/internal/prompt"
	"github.com/xmh1011/glaude/internal/session"
	"github.com/xmh1011/glaude/internal/skill"
	"github.com/xmh1011/glaude/internal/skill/bundled"
	"github.com/xmh1011/glaude/internal/state"
	"github.com/xmh1011/glaude/internal/telemetry"
	"github.com/xmh1011/glaude/internal/tool"
	"github.com/xmh1011/glaude/internal/tool/askuser"
	"github.com/xmh1011/glaude/internal/tool/bash"
	"github.com/xmh1011/glaude/internal/tool/configtool"
	"github.com/xmh1011/glaude/internal/tool/fileedit"
	"github.com/xmh1011/glaude/internal/tool/fileread"
	"github.com/xmh1011/glaude/internal/tool/filewrite"
	"github.com/xmh1011/glaude/internal/tool/glob"
	"github.com/xmh1011/glaude/internal/tool/grep"
	"github.com/xmh1011/glaude/internal/tool/ls"
	"github.com/xmh1011/glaude/internal/tool/mcpreslist"
	"github.com/xmh1011/glaude/internal/tool/mcpresread"
	"github.com/xmh1011/glaude/internal/tool/notebookedit"
	"github.com/xmh1011/glaude/internal/tool/planenter"
	"github.com/xmh1011/glaude/internal/tool/planexit"
	"github.com/xmh1011/glaude/internal/tool/skilltool"
	"github.com/xmh1011/glaude/internal/tool/sleep"
	"github.com/xmh1011/glaude/internal/tool/subagent"
	"github.com/xmh1011/glaude/internal/tool/taskcreate"
	"github.com/xmh1011/glaude/internal/tool/taskget"
	"github.com/xmh1011/glaude/internal/tool/tasklist"
	"github.com/xmh1011/glaude/internal/tool/taskoutput"
	"github.com/xmh1011/glaude/internal/tool/taskstop"
	"github.com/xmh1011/glaude/internal/tool/taskupdate"
	"github.com/xmh1011/glaude/internal/tool/todowrite"
	"github.com/xmh1011/glaude/internal/tool/toolsearch"
	"github.com/xmh1011/glaude/internal/tool/webfetch"
	"github.com/xmh1011/glaude/internal/tool/websearch"
	"github.com/xmh1011/glaude/internal/tool/worktreeenter"
	"github.com/xmh1011/glaude/internal/tool/worktreeexit"
	"github.com/xmh1011/glaude/internal/ui"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// --- Cancellation Tree ---
	// Signal handling for one-shot and subcommands.
	// In REPL mode, bubbletea manages signals in raw mode, so we stop
	// the external handler before starting the UI to avoid races.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig, ok := <-sigCh
		if !ok {
			return // channel closed, REPL mode took over
		}
		fmt.Fprintf(os.Stderr, "\nReceived %s, shutting down gracefully...\n", sig)
		cancel()
		// Second signal: force exit
		sig, ok = <-sigCh
		if !ok {
			return
		}
		fmt.Fprintf(os.Stderr, "\nReceived %s again, forcing exit.\n", sig)
		os.Exit(1)
	}()

	// --- CLI Command Tree ---
	rootCmd := buildRootCmd(ctx, sigCh)

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

func buildRootCmd(ctx context.Context, sigCh chan os.Signal) *cobra.Command {
	var (
		userPrompt    string
		continueFlag  bool
		resumeSession string
		skipPerms     bool
	)

	rootCmd := &cobra.Command{
		Use:           "glaude",
		Short:         "AI Coding Agent powered by LLM",
		Long:          "glaude is a Go implementation of an AI coding agent, inspired by Claude Code architecture.",
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
			model := viper.GetString("model")
			providerName := viper.GetString("provider")
			cwd, _ := os.Getwd()

			if userPrompt != "" {
				// One-shot mode: run a single prompt and exit
				telemetry.Log.WithField("mode", "oneshot").Info("prompt received")

				provider, err := llm.NewProvider(cmd.Context(), providerName)
				if err != nil {
					return fmt.Errorf("create provider: %w", err)
				}
				skillReg := buildSkillRegistry(cwd)

				// Discover and load plugins
				pluginMgr := plugin.NewManager(cwd)
				for _, pe := range pluginMgr.Errors() {
					telemetry.Log.WithField("plugin", pe.Name).WithField("error", pe.Err.Error()).Warn("plugin load error")
				}
				pluginMgr.LoadSkills(skillReg)

				reg, fileState := buildRegistry(nil, provider, model, skillReg, state.New(), nil, mcp.NewManager())

				// Load MCP servers from config + plugins
				mcpMgr, err := mcp.LoadFromConfig(cmd.Context(), reg)
				if err != nil {
					return fmt.Errorf("load MCP servers: %w", err)
				}
				mcp.ConnectAll(cmd.Context(), mcpMgr, pluginMgr.MCPConfigs(), reg)
				defer mcpMgr.Close()

				// Load directive files (GLAUDE.md) from all tiers
				mem := &memory.FileStore{}
				instructions, err := mem.Load(cwd)
				if err != nil {
					telemetry.Log.WithField("error", err.Error()).Warn("failed to load directives")
				}

				sysPrompt := prompt.NewBuilder().
					WithCustomInstructions(instructions).
					WithSkills(skillReg.ForPrompt()).
					Build()
				a := agent.New(provider, model, sysPrompt, reg)
				a.SetFileState(fileState)

				// Skip all permission checks if requested
				if skipPerms {
					a.Gate().SetSkipAll(true)
				}

				// Session persistence for one-shot mode
				sessionID := uuid.New().String()
				store := session.NewStore(cwd, sessionID)
				defer store.Close()
				a.SetSession(store)

				// Hook engine for one-shot mode (merges config + plugin hooks)
				viperHooks := hook.LoadConfig()
				mergedHooks := hook.MergeConfigs(viperHooks, pluginMgr.MergedHooks())
				hookEngine := hook.NewEngineWithConfig(sessionID, mergedHooks)
				a.SetHookEngine(hookEngine)

				text, err := a.Run(cmd.Context(), userPrompt)
				a.RecordLastPrompt(userPrompt)

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

			provider, err := llm.NewProvider(cmd.Context(), providerName)
			if err != nil {
				return fmt.Errorf("create provider: %w", err)
			}
			cp := memory.NewCheckpoint()
			skillReg := buildSkillRegistry(cwd)

			// Discover and load plugins
			pluginMgr := plugin.NewManager(cwd)
			for _, pe := range pluginMgr.Errors() {
				telemetry.Log.WithField("plugin", pe.Name).WithField("error", pe.Err.Error()).Warn("plugin load error")
			}
			pluginMgr.LoadSkills(skillReg)

			// Create shared state and permission checker for plan mode tools.
			st := state.New()
			permMode := permission.ParseMode(viper.GetString("permission_mode"))
			permChecker := permission.NewCheckerWithMode(permMode)
			mcpMgr := mcp.NewManager()

			reg, fileState := buildRegistry(cp, provider, model, skillReg, st, permChecker, mcpMgr)

			// Load MCP servers from config + plugins into shared manager
			mcp.ConnectAllFromConfig(cmd.Context(), mcpMgr, reg)
			mcp.ConnectAll(cmd.Context(), mcpMgr, pluginMgr.MCPConfigs(), reg)
			defer mcpMgr.Close()

			mem := &memory.FileStore{}
			instructions, err := mem.Load(cwd)
			if err != nil {
				telemetry.Log.WithField("error", err.Error()).Warn("failed to load directives")
			}

			sysPrompt := prompt.NewBuilder().
				WithCustomInstructions(instructions).
				WithSkills(skillReg.ForPrompt()).
				Build()
			a := agent.New(provider, model, sysPrompt, reg)
			a.SetFileState(fileState)

			// Session persistence
			sessionID := uuid.New().String()

			// Hook engine for REPL mode (merges config + plugin hooks)
			viperHooks := hook.LoadConfig()
			mergedHooks := hook.MergeConfigs(viperHooks, pluginMgr.MergedHooks())
			hookEngine := hook.NewEngineWithConfig(sessionID, mergedHooks)
			a.SetHookEngine(hookEngine)

			// Handle --continue: resume most recent session
			if continueFlag {
				if info := session.MostRecentSession(cwd); info != nil {
					sessionID = info.ID
					entries, loadErr := session.LoadEntries(info.Path)
					if loadErr == nil && len(entries) > 0 {
						a.RestoreFrom(entries)
						telemetry.Log.
							WithField("session_id", sessionID).
							WithField("messages", len(entries)).
							Info("resumed session (--continue)")
					}
				}
			}

			// Handle --resume: interactive picker or specific session
			if resumeSession != "" {
				if resumeSession == "pick" {
					// Sentinel value from the flag default — show interactive picker
					picked, pickErr := pickSession(cwd)
					if pickErr != nil {
						return pickErr
					}
					if picked == "" {
						fmt.Println("No session selected.")
						return nil
					}
					resumeSession = picked
				}
				sessionID = resumeSession
				path := session.SessionFilePath(cwd, resumeSession)
				entries, loadErr := session.LoadEntries(path)
				if loadErr != nil {
					return fmt.Errorf("failed to load session %s: %w", resumeSession, loadErr)
				}
				if len(entries) > 0 {
					a.RestoreFrom(entries)
					telemetry.Log.
						WithField("session_id", sessionID).
						WithField("messages", len(entries)).
						Info("resumed session (--resume)")
				}
			}

			store := session.NewStore(cwd, sessionID)
			defer store.Close()
			a.SetSession(store)

			m := ui.NewModel(a, cp, cmd.Context())
			m.SetSkillRegistry(skillReg)
			m.SetSessionID(sessionID)

			// Restore display messages from session history so the user
			// can see the previous conversation when using --continue/--resume.
			if continueFlag || resumeSession != "" {
				m.RestoreMessages()
			}

			p := ui.NewProgram(m)

			// Wire permission gate: uses shared checker, bridges Ask to UI prompt
			telemetry.Log.WithField("permission_mode", permMode.String()).Info("permission mode configured")
			ui.WirePermissionGateWithChecker(a, p, permChecker)

			// Wire AskUserQuestion tool to the UI
			if askTool, ok := reg.Get("AskUserQuestion").(*askuser.Tool); ok {
				ui.WireAskUser(askTool, p)
			}

			// Skip all permission checks if requested
			if skipPerms {
				a.Gate().SetSkipAll(true)
			}

			// Stop the external signal handler before entering REPL.
			// Bubbletea manages SIGINT/SIGTERM in raw mode; having two
			// handlers race can cause Ctrl+C to not reach the UI.
			signal.Stop(sigCh)
			close(sigCh)

			if _, err := p.Run(); err != nil {
				return fmt.Errorf("UI: %w", err)
			}

			// Print exit message to stdout after alt screen is cleared
			fmt.Print(m.ExitMessage())
			return nil
		},
	}

	// Flags
	rootCmd.Flags().StringVarP(&userPrompt, "prompt", "p", "", "Run a single prompt and exit")
	rootCmd.Flags().BoolVarP(&continueFlag, "continue", "c", false, "Resume the most recent session")
	rootCmd.Flags().StringVar(&resumeSession, "resume", "", "Resume a session (interactive picker if no ID given)")
	rootCmd.Flags().Lookup("resume").NoOptDefVal = "pick" // --resume without value triggers picker
	rootCmd.Flags().BoolVar(&skipPerms, "dangerously-skip-permissions", false, "Skip all permission checks (use with caution)")

	// Subcommands
	rootCmd.AddCommand(buildVersionCmd())
	rootCmd.AddCommand(buildInitCmd())

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
// provider and model are used for sub-agent spawning.
// skillReg is used to register the Skill tool (may be nil).
// st is the shared session state for task/todo/plan/worktree tools.
// checker is the permission checker for plan mode tools.
// mcpMgr is the MCP manager for resource tools (may be nil).
func buildRegistry(cp *memory.Checkpoint, provider llm.Provider, model string, skillReg *skill.Registry, st *state.State, checker *permission.Checker, mcpMgr *mcp.Manager) (*tool.Registry, *tool.FileStateCache) {
	if cp == nil {
		cp = memory.NewCheckpoint()
	}
	if st == nil {
		st = state.New()
	}
	fileState := tool.NewFileStateCache()
	reg := tool.NewRegistry()

	// Core tools
	reg.Register(&fileread.Tool{FileState: fileState})
	reg.Register(&fileedit.Tool{Checkpoint: cp, FileState: fileState})
	reg.Register(&filewrite.Tool{Checkpoint: cp, FileState: fileState})
	reg.Register(bash.New())
	reg.Register(&glob.Tool{})
	reg.Register(&grep.Tool{})
	reg.Register(&ls.Tool{})
	reg.Register(&subagent.Tool{Provider: provider, Model: model, Registry: reg})
	reg.Register(webfetch.New(provider, model))
	reg.Register(&askuser.Tool{})

	// Sleep
	reg.Register(&sleep.Tool{})

	// Todo
	reg.Register(&todowrite.Tool{State: st})

	// Task system
	reg.Register(&taskcreate.Tool{State: st})
	reg.Register(&taskget.Tool{State: st})
	reg.Register(&tasklist.Tool{State: st})
	reg.Register(&taskupdate.Tool{State: st})
	reg.Register(&taskstop.Tool{})
	reg.Register(&taskoutput.Tool{})

	// Plan mode
	if checker != nil {
		reg.Register(&planenter.Tool{State: st, Checker: checker})
		reg.Register(&planexit.Tool{State: st, Checker: checker})
	}

	// Config + ToolSearch
	reg.Register(&configtool.Tool{})
	reg.Register(&toolsearch.Tool{Registry: reg})

	// MCP resources
	if mcpMgr != nil {
		reg.Register(&mcpreslist.Tool{Manager: mcpMgr})
		reg.Register(&mcpresread.Tool{Manager: mcpMgr})
	}

	// Notebook
	reg.Register(&notebookedit.Tool{})

	// Worktree
	reg.Register(&worktreeenter.Tool{State: st})
	reg.Register(&worktreeexit.Tool{State: st})

	// WebSearch
	reg.Register(&websearch.Tool{Provider: provider, Model: model})

	// Skill (conditional)
	if skillReg != nil && len(skillReg.All()) > 0 {
		reg.Register(&skilltool.Tool{SkillRegistry: skillReg})
	}

	return reg, fileState
}

// buildSkillRegistry creates a skill registry with bundled and disk-based skills.
func buildSkillRegistry(cwd string) *skill.Registry {
	skillReg := skill.NewRegistry()

	// Register bundled skills first (lowest priority)
	bundled.RegisterAll(skillReg)

	// Load disk-based skills (user + project, project overrides user)
	diskSkills, err := skill.LoadAll(cwd)
	if err != nil {
		telemetry.Log.WithField("error", err.Error()).Warn("failed to load disk skills")
	}
	for _, s := range diskSkills {
		skillReg.Register(s)
	}

	telemetry.Log.WithField("count", len(skillReg.All())).Info("skills loaded")
	return skillReg
}

// pickSession shows an interactive picker for recent sessions.
// Returns the selected session ID, or "" if the user cancels.
func pickSession(cwd string) (string, error) {
	sessions, err := session.ListSessions(cwd)
	if err != nil {
		return "", fmt.Errorf("list sessions: %w", err)
	}
	if len(sessions) == 0 {
		return "", fmt.Errorf("no saved sessions found")
	}

	// Limit to 20 most recent
	if len(sessions) > 20 {
		sessions = sessions[:20]
	}

	// Build options for the picker
	options := make([]huh.Option[string], 0, len(sessions))
	for _, s := range sessions {
		label := formatSessionLabel(s)
		options = append(options, huh.NewOption(label, s.ID))
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a session to resume").
				Options(options...).
				Value(&selected),
		),
	)
	if err := form.Run(); err != nil {
		return "", nil // user cancelled
	}
	return selected, nil
}

// formatSessionLabel creates a human-readable label for a session.
func formatSessionLabel(s session.SessionInfo) string {
	age := time.Since(s.Timestamp)
	var ageStr string
	switch {
	case age < time.Minute:
		ageStr = "just now"
	case age < time.Hour:
		ageStr = fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		ageStr = fmt.Sprintf("%dh ago", int(age.Hours()))
	default:
		ageStr = s.Timestamp.Format("2006-01-02 15:04")
	}

	title := s.Title
	if title == "" && s.LastPrompt != "" {
		title = s.LastPrompt
		if len(title) > 50 {
			title = title[:47] + "..."
		}
	}
	if title == "" {
		title = s.ID[:8] + "..."
	}

	return fmt.Sprintf("[%s] %s", ageStr, title)
}
