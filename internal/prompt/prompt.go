// Package prompt assembles the system prompt sent to the model: the
// embedded base prompt, runtime context, available skills, user-level
// AGENTS.md, and the project's instruction file.
package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/meain/fin/internal/config"
	finembed "github.com/meain/fin/internal/embed"
	"github.com/meain/fin/internal/skill"
)

// BuildSystem assembles the full system prompt. enabled controls which tool
// sections of the base prompt are kept; pass nil to keep every section.
func BuildSystem(cfg *config.Config, skills []*skill.Skill, sessionID string, enabled map[string]bool) string {
	var b strings.Builder
	b.WriteString(gateSystem(finembed.SystemPrompt, enabled))

	cwd, _ := os.Getwd()
	fmt.Fprintf(&b, "\n\nEnvironment:\n- Date: %s\n- OS: %s/%s\n- Working directory: %s",
		time.Now().Format("2006-01-02 Monday"),
		runtime.GOOS, runtime.GOARCH,
		cwd,
	)
	if sessionID != "" {
		fmt.Fprintf(&b, "\n- Session ID: %s", sessionID)
	}

	useSkillOn := enabled == nil || enabled["use_skill"]
	if useSkillOn && len(skills) > 0 {
		b.WriteString("\n\nAvailable skills (use the use_skill tool to activate one):\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- %s: %s (skill file: %s)\n", s.Name, s.Description, filepath.Join(s.Dir, config.SkillFile))
		}
	}

	if agentsContent := readAgentsMD(); agentsContent != "" {
		fmt.Fprintf(&b, "\n\nUser instructions:\n%s", agentsContent)
	}

	projectContent := findProjectFile(cfg.Settings.ProjectFile)
	if projectContent != "" {
		fmt.Fprintf(&b, "\n\nProject instructions:\n%s", projectContent)
	}

	if !cfg.Settings.DisableClaudeMemory {
		if memoryContent, topics, dir, truncated := readClaudeMemory(); memoryContent != "" {
			fmt.Fprintf(&b, "\n\nClaude Code auto-memory (read-only reference from %s/MEMORY.md; learnings Claude Code captured about this project):\n%s",
				dir, memoryContent)
			if truncated {
				b.WriteString("\n(index truncated — read the file directly for the full contents)")
			}
			if len(topics) > 0 {
				b.WriteString("\nAdditional topic files with more detail (read on demand with the read tool):\n")
				for _, t := range topics {
					fmt.Fprintf(&b, "- %s\n", filepath.Join(dir, t))
				}
			}
		}
	}

	return b.String()
}

// gateSystem strips ## sections of the base prompt that reference disabled
// tools. Section → required tool:
//
//	Tool usage  → any tool enabled
//	About       → use_skill enabled
//	Shell       → shell tool
//	Subagents   → subagent tool
//	Compact     → compact tool
//
// Unknown sections are kept as-is so editors can add prose without code
// changes. nil enabled keeps every section.
func gateSystem(s string, enabled map[string]bool) string {
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
