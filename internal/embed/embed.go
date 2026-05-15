// Package embed centralizes all //go:embed declarations for fin's static
// assets so other packages can consume them without re-embedding.
package embed

import "embed"

//go:embed system_prompt.md
var SystemPrompt string

//go:embed skills
var BuiltinSkillsFS embed.FS

//go:embed hljs.min.js
var HljsJS string

//go:embed hljs.min.css
var HljsCSS string
