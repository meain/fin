package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func buildSystemPrompt(config *Config, skills []*Skill, sessionID string) string {
	var b strings.Builder
	b.WriteString(baseSystemPrompt)

	// Runtime context
	cwd, _ := os.Getwd()
	fmt.Fprintf(&b, "\n\nEnvironment:\n- Date: %s\n- OS: %s/%s\n- Working directory: %s",
		time.Now().Format("2006-01-02 Monday"),
		runtime.GOOS, runtime.GOARCH,
		cwd,
	)
	if sessionID != "" {
		fmt.Fprintf(&b, "\n- Session ID: %s", sessionID)
	}

	// Available skills (progressive disclosure: name + description only)
	if len(skills) > 0 {
		b.WriteString("\n\nAvailable skills (use the use_skill tool to activate one):\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- %s: %s (skill file: %s)\n", s.Name, s.Description, filepath.Join(s.Dir, skillFile))
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
	data, err := os.ReadFile(filepath.Join(homeDir(), agentsDir, agentsFile))
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
