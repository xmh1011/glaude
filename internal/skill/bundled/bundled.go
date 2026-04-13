// Package bundled registers built-in skills that ship with glaude.
package bundled

import "github.com/xmh1011/glaude/internal/skill"

// RegisterAll registers all bundled skills into the given registry.
func RegisterAll(reg *skill.Registry) {
	reg.Register(simplifySkill())
	reg.Register(commitSkill())
}
