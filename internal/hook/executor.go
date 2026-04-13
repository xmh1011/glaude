package hook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const defaultTimeout = 10 * time.Second

// blockingError is returned when a hook exits with code 2.
type blockingError struct {
	stderr string
}

func (e *blockingError) Error() string {
	if e.stderr != "" {
		return e.stderr
	}
	return "hook returned blocking error (exit code 2)"
}

// runCommand executes a single hook entry, passing input via stdin and
// parsing the result from stdout. The exit-code protocol is:
//
//	0 = success (stdout parsed as JSON or plain-text message)
//	1 = non-blocking error (stderr logged, execution continues)
//	2 = blocking error (tool execution is prevented)
func runCommand(ctx context.Context, entry Entry, input *Input) (*Output, error) {
	timeout := defaultTimeout
	if entry.Timeout > 0 {
		timeout = time.Duration(entry.Timeout) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", entry.Command)

	// Write HookInput as JSON to stdin.
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("marshal hook input: %w", err)
	}
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	// Handle exit codes.
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("hook timed out after %v", timeout)
		}

		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			code := exitErr.ExitCode()
			switch code {
			case 2:
				// Blocking error: prevent tool execution.
				return nil, &blockingError{stderr: stderr.String()}
			default:
				// Non-blocking error: log and continue.
				return nil, fmt.Errorf("hook exited with code %d: %s", code, stderr.String())
			}
		}
		return nil, fmt.Errorf("hook execution failed: %w", err)
	}

	// Exit 0: parse stdout.
	return parseOutput(stdout.Bytes()), nil
}

// parseOutput tries to parse stdout as JSON Output. If that fails,
// the raw text is used as an informational message.
func parseOutput(data []byte) *Output {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return &Output{}
	}

	var out Output
	if err := json.Unmarshal(data, &out); err != nil {
		// Not valid JSON: treat as plain-text message.
		return &Output{Message: string(data)}
	}
	return &out
}

// isExitError unwraps err into an *exec.ExitError. Returns true on success.
func isExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
