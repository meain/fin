package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func buildSystemPrompt(config *Config, skills []*Skill, sessionID string, enabled map[string]bool) string {
	var b strings.Builder
	b.WriteString(gateSystemPrompt(baseSystemPrompt, enabled))

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

	// Available skills (progressive disclosure: name + description only).
	// Suppressed entirely when use_skill is disabled — the model cannot activate them.
	useSkillOn := enabled == nil || enabled["use_skill"]
	if useSkillOn && len(skills) > 0 {
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

// gateSystemPrompt strips sections of the base prompt that reference disabled tools.
// Sections are recognized by `## <header>` markers. The intro (text before the first
// `## ` header) is always kept. With enabled == nil all sections are kept.
//
// Section → tool requirement:
//   Tool usage → any tool enabled
//   About      → use_skill enabled (mentions the about_fin skill)
//   Shell      → shell tool
//   Subagents  → subagent tool
//   Compact    → compact tool
//
// Unknown sections are kept as-is so editors can add prose without code changes.
func gateSystemPrompt(s string, enabled map[string]bool) string {
	if enabled == nil {
		return s
	}

	keep := func(header string) bool {
		switch header {
		case "Tool usage":
			return len(enabled) > 0
		case "About":
			return enabled["use_skill"]
		case "Shell":
			return enabled["shell"]
		case "Subagents":
			return enabled["subagent"]
		case "Compact":
			return enabled["compact"]
		default:
			return true
		}
	}

	lines := strings.Split(s, "\n")
	var out strings.Builder
	dropping := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			header := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			dropping = !keep(header)
			if dropping {
				continue
			}
		}
		if !dropping {
			out.WriteString(line)
			out.WriteString("\n")
		}
	}

	return strings.TrimRight(out.String(), "\n")
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
