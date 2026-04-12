package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"

	"github.com/xmh1011/glaude/internal/telemetry"
)

const (
	bashMaxOutputBytes = 100 * 1024 // 100KB output limit
	bashDefaultTimeout = 120 * time.Second
)

// BashTool executes shell commands in a persistent bash subprocess.
//
// Unlike single-shot exec, the shell persists across calls so stateful
// commands (cd, export, etc.) carry forward. Output is delimited by a
// UUID sentinel to precisely capture each command's stdout+stderr.
type BashTool struct {
	mu      sync.Mutex
	shell   *persistentShell
	timeout time.Duration
}

// NewBashTool creates a BashTool. The shell subprocess is started lazily
// on the first Execute call.
func NewBashTool() *BashTool {
	return &BashTool{timeout: bashDefaultTimeout}
}

func (b *BashTool) Name() string { return "Bash" }

func (b *BashTool) Description() string {
	return "Executes a bash command and returns its output. The shell persists between calls, so state (cd, env vars) carries forward."
}

func (b *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "The bash command to execute"},
			"timeout": {"type": "integer", "description": "Timeout in milliseconds (max 600000, default 120000)"},
			"description": {"type": "string", "description": "Description of what the command does"}
		},
		"required": ["command"]
	}`)
}

func (b *BashTool) IsReadOnly() bool { return false }

type bashInput struct {
	Command     string `json:"command"`
	Timeout     int    `json:"timeout"`
	Description string `json:"description"`
}

func (b *BashTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := b.timeout
	if in.Timeout > 0 {
		t := time.Duration(in.Timeout) * time.Millisecond
		if t > 10*time.Minute {
			t = 10 * time.Minute
		}
		timeout = t
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Lazy start the shell
	if b.shell == nil || !b.shell.alive() {
		sh, err := newPersistentShell()
		if err != nil {
			return "", fmt.Errorf("starting shell: %w", err)
		}
		b.shell = sh
	}

	return b.shell.exec(ctx, in.Command, timeout)
}

// Close terminates the persistent shell subprocess.
func (b *BashTool) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.shell != nil {
		b.shell.close()
		b.shell = nil
	}
}

// persistentShell manages a long-lived bash process.
type persistentShell struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	done   chan struct{}
}

func newPersistentShell() (*persistentShell, error) {
	cmd := exec.Command("bash", "--norc", "--noprofile")
	// Create new process group for cleanup
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	// Merge stderr into stdout for unified output capture
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start bash: %w", err)
	}

	done := make(chan struct{})
	go func() {
		cmd.Wait()
		close(done)
	}()

	telemetry.Log.WithField("pid", cmd.Process.Pid).Debug("bash: persistent shell started")

	return &persistentShell{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewReaderSize(stdout, 64*1024),
		done:   done,
	}, nil
}

func (s *persistentShell) alive() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// exec runs a command and returns its output, delimited by a UUID sentinel.
func (s *persistentShell) exec(ctx context.Context, command string, timeout time.Duration) (string, error) {
	sentinel := uuid.New().String()

	// Write command followed by sentinel echo.
	// The sentinel appears in stdout after the command finishes,
	// allowing us to precisely capture the command's output.
	wrapped := fmt.Sprintf(
		"%s 2>&1; echo \"__SENTINEL_%s_EXIT_$?__\"\n",
		command, sentinel,
	)

	if _, err := io.WriteString(s.stdin, wrapped); err != nil {
		return "", fmt.Errorf("writing to shell: %w", err)
	}

	sentinelPrefix := fmt.Sprintf("__SENTINEL_%s_EXIT_", sentinel)

	// Read output lines until we see the sentinel
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		output   string
		exitCode string
		err      error
	}
	ch := make(chan result, 1)

	go func() {
		var b strings.Builder
		truncated := false
		for {
			line, err := s.stdout.ReadString('\n')
			if err != nil {
				ch <- result{err: fmt.Errorf("reading output: %w", err)}
				return
			}
			line = strings.TrimRight(line, "\n")

			if strings.HasPrefix(line, sentinelPrefix) {
				// Extract exit code from sentinel
				exitCode := strings.TrimPrefix(line, sentinelPrefix)
				exitCode = strings.TrimSuffix(exitCode, "__")
				ch <- result{output: b.String(), exitCode: exitCode}
				return
			}

			if !truncated {
				if b.Len()+len(line)+1 > bashMaxOutputBytes {
					b.WriteString("\n... (output truncated at 100KB)")
					truncated = true
				} else {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.WriteString(line)
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		// Kill the current command's process group
		s.killCurrentCommand()
		return "", fmt.Errorf("command timed out after %s", timeout)
	case r := <-ch:
		if r.err != nil {
			return "", r.err
		}
		output := r.output
		if r.exitCode != "0" {
			if output == "" {
				output = fmt.Sprintf("(exit code %s)", r.exitCode)
			}
			return output, fmt.Errorf("command exited with code %s", r.exitCode)
		}
		return output, nil
	}
}

// killCurrentCommand sends SIGKILL to the shell's process group to terminate
// any running command. The shell itself may survive or be restarted.
func (s *persistentShell) killCurrentCommand() {
	if s.cmd.Process != nil {
		// Kill the entire process group
		pgid, err := syscall.Getpgid(s.cmd.Process.Pid)
		if err == nil {
			syscall.Kill(-pgid, syscall.SIGKILL)
		}
	}
}

func (s *persistentShell) close() {
	if s.stdin != nil {
		io.WriteString(s.stdin, "exit\n")
		s.stdin.Close()
	}
	// Wait briefly then force kill
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		if s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
	}
}
