// Package approval resolves per-tool approval decisions from CLI flags
// merged with config. Callers build an Approval once, then check
// Decide(toolName, args) for each tool call.
package approval

import (
	"path/filepath"

	"github.com/meain/fin/internal/config"
)

// Decision is the outcome of an approval check.
type Decision int

const (
	Ask   Decision = iota // prompt the user
	Allow                 // run without asking
	Deny                  // refuse without asking
)

// Approval holds pre-resolved tool approval decisions.
type Approval struct {
	approveAll bool
	safe       bool            // built from "safe" mode — subagents get restricted
	auto       map[string]bool // per-tool auto-approve
	denied     map[string]bool // per-tool deny (config approval = "deny")
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
// Call once after merging CLI flags with config. Denials from config
// (approval = "deny" and shell deny patterns) apply in every mode except
// "all", which approves everything.
func Build(mode string, tools map[string]config.ToolConfig) *Approval {
	a := &Approval{auto: make(map[string]bool), denied: make(map[string]bool)}
	if mode == "all" {
		a.approveAll = true
		return a
	}

	for name, tc := range tools {
		if tc.Approval == "deny" {
			a.denied[name] = true
		}
		if name == "shell" {
			a.shellDeny = tc.Deny
		}
	}

	switch mode {
	case "none":
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
		}
	}

	return a
}

// Decide reports whether the given tool call should run, be refused, or
// prompt the user. Denials are checked before per-tool auto, safe mode and
// shell allow patterns so none of them can bypass a deny.
func (a *Approval) Decide(toolName string, args map[string]any) Decision {
	if a.denied[toolName] {
		return Deny
	}

	cmd, isShell := "", false
	if toolName == "shell" {
		cmd, isShell = args["command"].(string)
	}

	if isShell {
		for _, pattern := range a.shellDeny {
			if matched, _ := filepath.Match(pattern, cmd); matched {
				return Deny
			}
		}
	}

	if a.approveAll || a.auto[toolName] {
		return Allow
	}

	if isShell {
		for _, pattern := range a.shellAllow {
			if matched, _ := filepath.Match(pattern, cmd); matched {
				return Allow
			}
		}
	}

	return Ask
}

// AutoApprove reports whether the given tool call should run without
// asking the user.
func (a *Approval) AutoApprove(toolName string, args map[string]any) bool {
	return a.Decide(toolName, args) == Allow
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
		denied:     a.denied,
		shellAllow: a.shellAllow,
		shellDeny:  a.shellDeny,
	}
}
