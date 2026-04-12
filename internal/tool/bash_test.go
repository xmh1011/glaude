package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBashTool_SimpleCommand(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "echo hello"})
	result, err := bt.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, "hello", strings.TrimSpace(result))
}

func TestBashTool_StatePersistence(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	// Set a variable
	input1, _ := json.Marshal(bashInput{Command: "export MY_TEST_VAR=glaude42"})
	_, err := bt.Execute(context.Background(), input1)
	require.NoError(t, err)

	// Read it back - should persist across calls
	input2, _ := json.Marshal(bashInput{Command: "echo $MY_TEST_VAR"})
	result, err := bt.Execute(context.Background(), input2)
	require.NoError(t, err)
	assert.Equal(t, "glaude42", strings.TrimSpace(result))
}

func TestBashTool_CdPersistence(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input1, _ := json.Marshal(bashInput{Command: "cd /tmp"})
	_, err := bt.Execute(context.Background(), input1)
	require.NoError(t, err)

	input2, _ := json.Marshal(bashInput{Command: "pwd"})
	result, err := bt.Execute(context.Background(), input2)
	require.NoError(t, err)
	// On macOS /tmp is a symlink to /private/tmp
	trimmed := strings.TrimSpace(result)
	assert.True(t, trimmed == "/tmp" || trimmed == "/private/tmp",
		"expected /tmp or /private/tmp, got %q", trimmed)
}

func TestBashTool_NonZeroExit(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "ls /nonexistent_path_xyz"})
	_, err := bt.Execute(context.Background(), input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exited with code")
}

func TestBashTool_StderrMerged(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "echo stdout; echo stderr >&2"})
	result, err := bt.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Contains(t, result, "stdout")
	assert.Contains(t, result, "stderr")
}

func TestBashTool_Timeout(t *testing.T) {
	bt := NewBashTool()
	bt.timeout = 1 * time.Second
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "sleep 30"})
	start := time.Now()
	_, err := bt.Execute(context.Background(), input)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, elapsed, 5*time.Second, "timeout took too long")
}

func TestBashTool_MultilineOutput(t *testing.T) {
	bt := NewBashTool()
	defer bt.Close()

	input, _ := json.Marshal(bashInput{Command: "for i in 1 2 3 4 5; do echo line$i; done"})
	result, err := bt.Execute(context.Background(), input)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(result), "\n")
	assert.Len(t, lines, 5)
}
