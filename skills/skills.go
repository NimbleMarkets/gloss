// Package skills carries the skill that teaches agents to use gloss, so that
// the binary can print it: gloss --skill.
package skills

import _ "embed"

// Gloss is skills/gloss/SKILL.md, whole, as an agent would install it.
//
//go:embed gloss/SKILL.md
var Gloss string
