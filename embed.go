package main

import "embed"

//go:embed system_prompt.md
var baseSystemPrompt string

//go:embed skills
var builtinSkillsFS embed.FS
