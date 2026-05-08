package main

import "path/filepath"

// toolApproval holds pre-resolved tool approval decisions.
// Built once from merged CLI flags + config, then passed to the agent.
// Downstream code checks this struct instead of interpreting settings.
type toolApproval struct {
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

// buildToolApproval resolves the approval mode and per-tool config into a
// single struct. Call this once after merging CLI flags with config.
func buildToolApproval(mode string, tools map[string]ToolConfig) *toolApproval {
	ta := &toolApproval{auto: make(map[string]bool)}

	switch mode {
	case "none":
		return ta
	case "all":
		ta.approveAll = true
		return ta
	case "safe":
		ta.safe = true
		for name := range safeTools {
			ta.auto[name] = true
		}
	}

	for name, tc := range tools {
		if tc.Approval == "auto" {
			ta.auto[name] = true
		}
		if name == "shell" {
			ta.shellAllow = tc.Allow
			ta.shellDeny = tc.Deny
		}
	}

	return ta
}

// autoApprove reports whether the given tool call should be auto-approved.
func (ta *toolApproval) autoApprove(toolName string, args map[string]any) bool {
	if ta.approveAll {
		return true
	}

	if ta.auto[toolName] {
		return true
	}

	if toolName == "shell" {
		if cmd, ok := args["command"].(string); ok {
			for _, pattern := range ta.shellDeny {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return false
				}
			}
			for _, pattern := range ta.shellAllow {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return true
				}
			}
		}
	}

	return false
}

// forSubagent returns an approval policy for a child agent.
// In safe mode, subagents get a restricted set (read, compact, use_skill only).
// Otherwise the parent's policy is inherited.
func (ta *toolApproval) forSubagent() *toolApproval {
	if !ta.safe {
		return ta
	}
	return &toolApproval{
		auto: map[string]bool{
			"read":      true,
			"compact":   true,
			"use_skill": true,
		},
		shellAllow: ta.shellAllow,
		shellDeny:  ta.shellDeny,
	}
}
