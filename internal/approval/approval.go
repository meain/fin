// Package approval resolves per-tool approval decisions from CLI flags
// merged with config. Callers build an Approval once, then check
// AutoApprove(toolName, args) for each tool call.
package approval

import (
	"path/filepath"

	"github.com/meain/fin/internal/config"
)

// Approval holds pre-resolved tool approval decisions.
type Approval struct {
	approveAll bool
	safe       bool            // built from "safe" mode — subagents get restricted
	auto       map[string]bool // per-tool auto-approve
	shellAllow []string        // glob patterns that auto-approve shell commands
	shellDeny  []string        // glob patterns that deny shell commands
}

// safeTools are auto-approved in "safe" mode.
var safeTools = map[string]bool{
	"read":      true,
	"use_skill": true,
	"compact":   true,
	"subagent":  true,
}

// Build resolves the approval mode and per-tool config into an Approval.
// Call once after merging CLI flags with config.
func Build(mode string, tools map[string]config.ToolConfig) *Approval {
	a := &Approval{auto: make(map[string]bool)}

	switch mode {
	case "none":
		return a
	case "all":
		a.approveAll = true
		return a
	case "safe":
		a.safe = true
		for name := range safeTools {
			a.auto[name] = true
		}
	}

	for name, tc := range tools {
		if tc.Approval == "auto" {
			a.auto[name] = true
		}
		if name == "shell" {
			a.shellAllow = tc.Allow
			a.shellDeny = tc.Deny
		}
	}

	return a
}

// AutoApprove reports whether the given tool call should run without
// asking the user. Shell calls check deny patterns before allow patterns.
func (a *Approval) AutoApprove(toolName string, args map[string]any) bool {
	if a.approveAll {
		return true
	}

	if a.auto[toolName] {
		return true
	}

	if toolName == "shell" {
		if cmd, ok := args["command"].(string); ok {
			for _, pattern := range a.shellDeny {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return false
				}
			}
			for _, pattern := range a.shellAllow {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return true
				}
			}
		}
	}

	return false
}

// ForSubagent returns an approval policy for a child agent. In safe mode,
// subagents get a restricted set (read, compact, use_skill only). Otherwise
// the parent's policy is inherited.
func (a *Approval) ForSubagent() *Approval {
	if !a.safe {
		return a
	}
	return &Approval{
		auto: map[string]bool{
			"read":      true,
			"compact":   true,
			"use_skill": true,
		},
		shellAllow: a.shellAllow,
		shellDeny:  a.shellDeny,
	}
}
