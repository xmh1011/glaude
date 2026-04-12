package permission

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMode_String(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeDefault, "default"},
		{ModeAutoEdit, "auto-edit"},
		{ModePlanOnly, "plan-only"},
		{ModeAutoFull, "auto-full"},
		{Mode(99), "unknown(99)"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, tt.mode.String())
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"default", ModeDefault},
		{"auto-edit", ModeAutoEdit},
		{"autoedit", ModeAutoEdit},
		{"plan-only", ModePlanOnly},
		{"planonly", ModePlanOnly},
		{"plan", ModePlanOnly},
		{"auto-full", ModeAutoFull},
		{"autofull", ModeAutoFull},
		{"auto", ModeAutoFull},
		{"yolo", ModeAutoFull},
		{"YOLO", ModeAutoFull},
		{"  Auto-Edit  ", ModeAutoEdit},
		{"garbage", ModeDefault},
		{"", ModeDefault},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ParseMode(tt.input), "input=%q", tt.input)
	}
}

func TestAllModes(t *testing.T) {
	modes := AllModes()
	assert.Len(t, modes, 4)
	assert.Equal(t, ModeDefault, modes[0])
	assert.Equal(t, ModeAutoFull, modes[3])
}
