// Package permission implements security boundaries and the permission evaluation pipeline.
//
// Four security modes control what the agent can do without user confirmation:
//   - Default: every mutating tool call requires explicit user approval
//   - AutoEdit: file edits (Edit, Write) are auto-approved; Bash still requires approval
//   - PlanOnly: all mutating operations are rejected; read-only tools only
//   - AutoFull: all operations are auto-approved (dangerous — opt-in only)
package permission

import (
	"fmt"
	"strings"
)

// Mode represents one of the four security modes.
type Mode int

const (
	ModeDefault  Mode = iota // every mutation needs y/n
	ModeAutoEdit             // file edits auto-approved
	ModePlanOnly             // mutations rejected, read-only
	ModeAutoFull             // everything auto-approved (yolo)
)

// String returns the human-readable mode name.
func (m Mode) String() string {
	switch m {
	case ModeDefault:
		return "default"
	case ModeAutoEdit:
		return "auto-edit"
	case ModePlanOnly:
		return "plan-only"
	case ModeAutoFull:
		return "auto-full"
	default:
		return fmt.Sprintf("unknown(%d)", int(m))
	}
}

// ParseMode converts a string to a Mode. Returns ModeDefault on unknown input.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "default":
		return ModeDefault
	case "auto-edit", "autoedit":
		return ModeAutoEdit
	case "plan-only", "planonly", "plan":
		return ModePlanOnly
	case "auto-full", "autofull", "auto", "yolo":
		return ModeAutoFull
	default:
		return ModeDefault
	}
}

// AllModes returns a slice of all valid modes for display purposes.
func AllModes() []Mode {
	return []Mode{ModeDefault, ModeAutoEdit, ModePlanOnly, ModeAutoFull}
}
