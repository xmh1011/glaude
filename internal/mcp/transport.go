package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/xmh1011/glaude/internal/telemetry"
)

// Transport abstracts the communication channel with an MCP server.
type Transport interface {
	// Send sends a JSON-RPC request and waits for the response.
	Send(ctx context.Context, req *Request) (*Response, error)

	// Notify sends a JSON-RPC notification (no response expected).
	Notify(ctx context.Context, n *Notification) error

	// Close shuts down the transport.
	Close() error
}

// StdioTransport communicates with an MCP server subprocess via stdin/stdout.
// Each line of stdout is a JSON-RPC response. Requests are written as newline-
// delimited JSON to stdin.
type StdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
	done   chan struct{}
	nextID atomic.Int64

	// pending maps request IDs to response channels
	pendingMu sync.Mutex
	pending   map[int]chan *Response
}

// NewStdioTransport starts a subprocess and returns a transport connected to it.
func NewStdioTransport(command string, args []string, env []string) (*StdioTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	// Discard stderr to prevent blocking
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start MCP server %q: %w", command, err)
	}

	done := make(chan struct{})
	t := &StdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  bufio.NewScanner(stdout),
		done:    done,
		pending: make(map[int]chan *Response),
	}

	// Start the reader goroutine
	go t.readLoop(done)

	telemetry.Log.
		WithField("command", command).
		WithField("pid", cmd.Process.Pid).
		Debug("mcp: stdio transport started")

	return t, nil
}

// readLoop reads lines from stdout and dispatches responses to pending channels.
func (t *StdioTransport) readLoop(done chan struct{}) {
	defer close(done)
	for t.stdout.Scan() {
		line := t.stdout.Bytes()

		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			telemetry.Log.
				WithField("error", err.Error()).
				WithField("line", string(line)).
				Debug("mcp: failed to parse response")
			continue
		}

		t.pendingMu.Lock()
		ch, ok := t.pending[resp.ID]
		if ok {
			delete(t.pending, resp.ID)
		}
		t.pendingMu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// Send sends a request and waits for the matching response.
func (t *StdioTransport) Send(ctx context.Context, req *Request) (*Response, error) {
	req.JSONRPC = jsonRPCVersion
	if req.ID == 0 {
		req.ID = int(t.nextID.Add(1))
	}

	// Register pending response channel
	ch := make(chan *Response, 1)
	t.pendingMu.Lock()
	t.pending[req.ID] = ch
	t.pendingMu.Unlock()

	// Marshal and send
	data, err := json.Marshal(req)
	if err != nil {
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	t.mu.Lock()
	_, err = fmt.Fprintf(t.stdin, "%s\n", data)
	t.mu.Unlock()

	if err != nil {
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
		return nil, fmt.Errorf("write to stdin: %w", err)
	}

	// Wait for response or context cancellation
	select {
	case <-ctx.Done():
		t.pendingMu.Lock()
		delete(t.pending, req.ID)
		t.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp, nil
	case <-t.done:
		return nil, fmt.Errorf("mcp server process exited")
	}
}

// Notify sends a notification (no response expected).
func (t *StdioTransport) Notify(_ context.Context, n *Notification) error {
	n.JSONRPC = jsonRPCVersion

	data, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	_, err = fmt.Fprintf(t.stdin, "%s\n", data)
	return err
}

// Close terminates the subprocess.
func (t *StdioTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.stdin != nil {
		t.stdin.Close()
	}

	// Wait briefly then force kill
	select {
	case <-t.done:
		return nil
	case <-time.After(3 * time.Second):
		if t.cmd.Process != nil {
			t.cmd.Process.Kill()
		}
		return nil
	}
}
