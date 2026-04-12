package hook

import (
	"context"
	"encoding/json"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// matchTool
// ---------------------------------------------------------------------------

func TestMatchTool(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		tool    string
		want    bool
	}{
		{"wildcard star", "*", "Bash", true},
		{"empty pattern", "", "Bash", true},
		{"exact match", "Bash", "Bash", true},
		{"exact mismatch", "Bash", "Read", false},
		{"pipe match first", "Write|Edit", "Write", true},
		{"pipe match second", "Write|Edit", "Edit", true},
		{"pipe mismatch", "Write|Edit", "Bash", false},
		{"glob match", "File*", "FileRead", true},
		{"glob mismatch", "File*", "Bash", false},
		{"whitespace pattern", "  *  ", "Bash", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchTool(tt.pattern, tt.tool)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// aggregate
// ---------------------------------------------------------------------------

func TestAggregate_DenyWins(t *testing.T) {
	boolFalse := false
	outputs := []*HookOutput{
		{Decision: "allow", Message: "hook1 ok"},
		{Decision: "deny", Message: "hook2 denied"},
	}
	result := aggregate(outputs, nil, false)
	assert.Equal(t, Deny, result.Decision)
	assert.Len(t, result.Messages, 2)

	// With continue=false
	outputs2 := []*HookOutput{
		{Continue: &boolFalse},
	}
	result2 := aggregate(outputs2, nil, false)
	assert.True(t, result2.StopSession)
}

func TestAggregate_AllowWhenNoDeny(t *testing.T) {
	outputs := []*HookOutput{
		{Decision: "allow"},
		{Decision: "allow"},
	}
	result := aggregate(outputs, nil, false)
	assert.Equal(t, Allow, result.Decision)
}

func TestAggregate_UpdatedInputLastWins(t *testing.T) {
	outputs := []*HookOutput{
		{UpdatedInput: json.RawMessage(`{"a":1}`)},
		{UpdatedInput: json.RawMessage(`{"b":2}`)},
	}
	result := aggregate(outputs, nil, false)
	assert.JSONEq(t, `{"b":2}`, string(result.UpdatedInput))
}

func TestAggregate_BlockedSetsDeny(t *testing.T) {
	result := aggregate(nil, nil, true)
	assert.Equal(t, Deny, result.Decision)
	assert.True(t, result.Blocked)
}

// ---------------------------------------------------------------------------
// runCommand — exit codes
// ---------------------------------------------------------------------------

func TestRunCommand_ExitCode0(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{Type: "command", Command: `echo '{"decision":"allow"}'`}
	input := &HookInput{CWD: "/tmp"}
	out, err := runCommand(context.Background(), entry, input)
	require.NoError(t, err)
	assert.Equal(t, "allow", out.Decision)
}

func TestRunCommand_ExitCode1(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{Type: "command", Command: `echo "warning" >&2; exit 1`}
	input := &HookInput{CWD: "/tmp"}
	_, err := runCommand(context.Background(), entry, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "warning")
	// Not a blocking error.
	_, isBlocking := err.(*blockingError)
	assert.False(t, isBlocking)
}

func TestRunCommand_ExitCode2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{Type: "command", Command: `echo "blocked" >&2; exit 2`}
	input := &HookInput{CWD: "/tmp"}
	_, err := runCommand(context.Background(), entry, input)
	require.Error(t, err)
	be, ok := err.(*blockingError)
	require.True(t, ok, "expected *blockingError")
	assert.Contains(t, be.Error(), "blocked")
}

// ---------------------------------------------------------------------------
// runCommand — JSON vs plain text
// ---------------------------------------------------------------------------

func TestRunCommand_JSONOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{
		Type:    "command",
		Command: `echo '{"decision":"deny","message":"not allowed"}'`,
	}
	input := &HookInput{CWD: "/tmp"}
	out, err := runCommand(context.Background(), entry, input)
	require.NoError(t, err)
	assert.Equal(t, "deny", out.Decision)
	assert.Equal(t, "not allowed", out.Message)
}

func TestRunCommand_PlainText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{Type: "command", Command: `echo "just a message"`}
	input := &HookInput{CWD: "/tmp"}
	out, err := runCommand(context.Background(), entry, input)
	require.NoError(t, err)
	assert.Equal(t, "just a message", out.Message)
}

func TestRunCommand_EmptyOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{Type: "command", Command: `true`}
	input := &HookInput{CWD: "/tmp"}
	out, err := runCommand(context.Background(), entry, input)
	require.NoError(t, err)
	assert.NotNil(t, out)
}

// ---------------------------------------------------------------------------
// runCommand — timeout
// ---------------------------------------------------------------------------

func TestRunCommand_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	entry := HookEntry{
		Type:    "command",
		Command: `sleep 30`,
		Timeout: 100, // 100ms
	}
	input := &HookInput{CWD: "/tmp"}
	start := time.Now()
	_, err := runCommand(context.Background(), entry, input)
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timed out")
	assert.Less(t, elapsed, 5*time.Second, "should not wait for full sleep")
}

// ---------------------------------------------------------------------------
// runCommand — stdin receives HookInput
// ---------------------------------------------------------------------------

func TestRunCommand_StdinReceivesJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	// The hook reads stdin and outputs the tool_name field.
	entry := HookEntry{
		Type:    "command",
		Command: `python3 -c "import sys,json; d=json.load(sys.stdin); print(d['tool_name'])"`,
	}
	input := &HookInput{CWD: "/tmp", ToolName: "Bash"}

	// Try with python3, skip if not available.
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		if _, err2 := os.Stat("/usr/local/bin/python3"); err2 != nil {
			t.Skip("python3 not available")
		}
	}

	out, err := runCommand(context.Background(), entry, input)
	require.NoError(t, err)
	assert.Equal(t, "Bash", out.Message)
}

// ---------------------------------------------------------------------------
// Engine.Dispatch — integration
// ---------------------------------------------------------------------------

func TestEngine_Dispatch_NoConfig(t *testing.T) {
	e := &Engine{sessionID: "test-session", config: HookConfig{}}
	result := e.Dispatch(context.Background(), PreToolUse, &HookInput{
		CWD:      "/tmp",
		ToolName: "Bash",
	})
	assert.Equal(t, None, result.Decision)
	assert.Empty(t, result.Errors)
}

func TestEngine_Dispatch_MatchAndExecute(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on windows")
	}
	e := &Engine{
		sessionID: "test-session",
		config: HookConfig{
			PreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []HookEntry{
						{Type: "command", Command: `echo '{"decision":"allow","message":"ok"}'`},
					},
				},
			},
		},
	}
	result := e.Dispatch(context.Background(), PreToolUse, &HookInput{
		CWD:      "/tmp",
		ToolName: "Bash",
	})
	assert.Equal(t, Allow, result.Decision)
	assert.Contains(t, result.Messages, "ok")
}

func TestEngine_Dispatch_NoMatch(t *testing.T) {
	e := &Engine{
		sessionID: "test-session",
		config: HookConfig{
			PreToolUse: {
				{
					Matcher: "Write",
					Hooks: []HookEntry{
						{Type: "command", Command: `echo 'should not run'`},
					},
				},
			},
		},
	}
	result := e.Dispatch(context.Background(), PreToolUse, &HookInput{
		CWD:      "/tmp",
		ToolName: "Bash",
	})
	assert.Equal(t, None, result.Decision)
}

func TestEngine_Nil_Safe(t *testing.T) {
	var e *Engine
	assert.False(t, e.HasHooks(PreToolUse))
	result := e.Dispatch(context.Background(), PreToolUse, &HookInput{})
	assert.Equal(t, None, result.Decision)
}

func TestEngine_HasHooks(t *testing.T) {
	e := &Engine{
		config: HookConfig{
			PreToolUse: {
				{Matcher: "*", Hooks: []HookEntry{{Type: "command", Command: "true"}}},
			},
		},
	}
	assert.True(t, e.HasHooks(PreToolUse))
	assert.False(t, e.HasHooks(PostToolUse))
}

// ---------------------------------------------------------------------------
// Decision.String
// ---------------------------------------------------------------------------

func TestDecision_String(t *testing.T) {
	assert.Equal(t, "none", None.String())
	assert.Equal(t, "allow", Allow.String())
	assert.Equal(t, "deny", Deny.String())
	assert.Equal(t, "unknown", Decision(99).String())
}
