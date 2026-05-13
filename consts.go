package main

import (
	"os"
	"path/filepath"
)

// Home directory of the current user.
func homeDir() string { return os.Getenv("HOME") }

// Path to the TOML configuration file.
func configPath() string { return filepath.Join(homeDir(), ".config", "fin", "config.toml") }

// Directory where session JSON files are stored.
func sessionPath() string { return filepath.Join(homeDir(), ".local", "share", "fin", "sessions") }

// Directory name for agent configuration and skills.
const agentsDir = ".agents"

// Filename for project-level agent instructions.
const agentsFile = "AGENTS.md"

// Filename for skill definitions within a skill directory.
const skillFile = "SKILL.md"

// Directory name for skills within the agents directory.
const skillsDirName = "skills"
