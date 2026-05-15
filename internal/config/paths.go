package config

import (
	"os"
	"path/filepath"
)

// AgentsDir is the directory name for agent configuration and skills.
const AgentsDir = ".agents"

// AgentsFile is the project-level agent instructions filename.
const AgentsFile = "AGENTS.md"

// SkillFile is the per-skill metadata + body filename.
const SkillFile = "SKILL.md"

// SkillsDirName is the directory name for skills within AgentsDir.
const SkillsDirName = "skills"

// HomeDir returns the user's home directory (HOME env var; empty if unset).
func HomeDir() string { return os.Getenv("HOME") }

// ConfigPath returns the absolute path to the TOML configuration file.
func ConfigPath() string {
	return filepath.Join(HomeDir(), ".config", "fin", "config.toml")
}

// SessionPath returns the directory where session JSONL files are stored.
func SessionPath() string {
	return filepath.Join(HomeDir(), ".local", "share", "fin", "sessions")
}
