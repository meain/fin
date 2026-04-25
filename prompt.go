package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const baseSystemPrompt = `You are fin, a coding assistant that runs in the terminal.

You have access to the following tools:
- read: Read file contents (with optional line range)
- write: Write/create files
- edit: Replace exact strings in files (surgical edits)
- shell: Execute shell commands

Guidelines:
- Read files before editing them.
- Prefer edit over write for modifying existing files.
- Keep shell commands focused and non-destructive.
- Be concise in your responses.`

func buildSystemPrompt(config *Config, skills []*Skill) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt)

	// Skill tool description (if skills are available)
	if len(skills) > 0 {
		b.WriteString("\n- use_skill: Activate a skill to load its full instructions or read a skill's resource files")
	}

	// Runtime context
	cwd, _ := os.Getwd()
	fmt.Fprintf(&b, "\n\nEnvironment:\n- Date: %s\n- OS: %s/%s\n- Working directory: %s",
		time.Now().Format("2006-01-02 Monday"),
		runtime.GOOS, runtime.GOARCH,
		cwd,
	)

	// Available skills (progressive disclosure: name + description only)
	if len(skills) > 0 {
		b.WriteString("\n\nAvailable skills (use the use_skill tool to activate one):\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}
	}

	// User-level instructions: ~/.agents/AGENTS.md
	if agentsContent := readAgentsMD(); agentsContent != "" {
		fmt.Fprintf(&b, "\n\nUser instructions:\n%s", agentsContent)
	}

	// Project file
	projectContent := findProjectFile(config.Settings.ProjectFile)
	if projectContent != "" {
		fmt.Fprintf(&b, "\n\nProject instructions:\n%s", projectContent)
	}

	return b.String()
}

// readAgentsMD reads ~/.agents/AGENTS.md if it exists.
func readAgentsMD() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(home, ".agents", "AGENTS.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// findProjectFile walks up from cwd looking for the named file.
func findProjectFile(name string) string {
	if name == "" {
		return ""
	}

	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return ""
}
