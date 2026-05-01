package main

import "embed"

//go:embed system_prompt.md
var baseSystemPrompt string

//go:embed skills
var builtinSkillsFS embed.FS

//go:embed hljs.min.js
var hljsJS string

//go:embed hljs.min.css
var hljsCSS string
