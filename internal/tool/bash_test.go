package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBashTool_SimpleCommand(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "echo hello"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(result) != "hello" {
		t.Fatalf("expected 'hello', got %q", result)
	}
}

func TestBashTool_StatePersistence(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	// Set a variable
	input1, _ := json.Marshal(bashInput{Command: "export MY_TEST_VAR=glaude42"})
	_, err := bt.Execute(context.Background(), input1)
	if err != nil {
		t.Fatalf("set var error: %v", err)
	}

	// Read it back - should persist across calls
	input2, _ := json.Marshal(bashInput{Command: "echo $MY_TEST_VAR"})
	result, err := bt.Execute(context.Background(), input2)
	if err != nil {
		t.Fatalf("read var error: %v", err)
	}
	if strings.TrimSpace(result) != "glaude42" {
		t.Fatalf("expected 'glaude42', got %q", result)
	}
}

func TestBashTool_CdPersistence(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input1, _ := json.Marshal(bashInput{Command: "cd /tmp"})
	_, err := bt.Execute(context.Background(), input1)
	if err != nil {
		t.Fatalf("cd error: %v", err)
	}

	input2, _ := json.Marshal(bashInput{Command: "pwd"})
	result, err := bt.Execute(context.Background(), input2)
	if err != nil {
		t.Fatalf("pwd error: %v", err)
	}
	// On macOS /tmp is a symlink to /private/tmp
	trimmed := strings.TrimSpace(result)
	if trimmed != "/tmp" && trimmed != "/private/tmp" {
		t.Fatalf("expected /tmp, got %q", trimmed)
	}
}

func TestBashTool_NonZeroExit(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	// Use a command that exits non-zero without killing the shell
	input, _ := json.Marshal(bashInput{Command: "ls /nonexistent_path_xyz"})
	_, err := bt.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "exited with code") {
		t.Fatalf("expected exit code error, got: %v", err)
	}
}

func TestBashTool_StderrMerged(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "echo stdout; echo stderr >&2"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "stdout") || !strings.Contains(result, "stderr") {
		t.Fatalf("expected both stdout and stderr, got %q", result)
	}
}

func TestBashTool_Timeout(t *testing.T) {
	bt := NewBashTool()
	bt.timeout = 1 * time.Second
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "sleep 30"})
	start := time.Now()
	_, err := bt.Execute(context.Background(), input)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}

func TestBashTool_MultilineOutput(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "for i in 1 2 3 4 5; do echo line$i; done"})
	result, err := bt.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(result), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d: %q", len(lines), result)
	}
}
