package approval

import (
	"testing"

	"github.com/meain/fin/internal/config"
)

func TestApproval_AllMode(t *testing.T) {
	ta := Build("all", nil)

	if !ta.AutoApprove("read", nil) {
		t.Error("all mode should approve read")
	}
	if !ta.AutoApprove("shell", map[string]any{"command": "rm -rf /"}) {
		t.Error("all mode should approve any shell command")
	}
	if !ta.AutoApprove("unknown_tool", nil) {
		t.Error("all mode should approve unknown tools")
	}
}

func TestApproval_NoneMode(t *testing.T) {
	ta := Build("none", map[string]config.ToolConfig{
		"read": {Approval: "auto"},
	})

	if ta.AutoApprove("read", nil) {
		t.Error("none mode should deny read even with per-tool auto")
	}
	if ta.AutoApprove("shell", nil) {
		t.Error("none mode should deny shell")
	}
}

func TestApproval_SafeMode(t *testing.T) {
	ta := Build("safe", map[string]config.ToolConfig{
		"write": {Approval: "confirm"},
		"edit":  {Approval: "auto"},
	})

	// Safe tools approved
	for _, name := range []string{"read", "use_skill", "compact", "subagent"} {
		if !ta.AutoApprove(name, nil) {
			t.Errorf("safe mode should approve %s", name)
		}
	}

	// Per-tool auto still works
	if !ta.AutoApprove("edit", nil) {
		t.Error("safe mode should still honor per-tool auto for edit")
	}

	// Confirm tools not approved
	if ta.AutoApprove("write", nil) {
		t.Error("safe mode should not approve write with confirm")
	}

	// Unknown tools not approved
	if ta.AutoApprove("unknown", nil) {
		t.Error("safe mode should not approve unknown tools")
	}
}

func TestApproval_DefaultMode(t *testing.T) {
	ta := Build("", map[string]config.ToolConfig{
		"read":  {Approval: "auto"},
		"write": {Approval: "confirm"},
		"shell": {Approval: "deny"},
	})

	if !ta.AutoApprove("read", nil) {
		t.Error("default mode should approve read with auto")
	}
	if ta.AutoApprove("write", nil) {
		t.Error("default mode should not approve write with confirm")
	}
	if ta.AutoApprove("shell", nil) {
		t.Error("default mode should not approve shell with deny")
	}
	if ta.AutoApprove("unknown", nil) {
		t.Error("default mode should not approve unconfigured tools")
	}
}

func TestApproval_ShellPatterns(t *testing.T) {
	ta := Build("", map[string]config.ToolConfig{
		"shell": {
			Approval: "confirm",
			Allow:    []string{"go *", "ls"},
			Deny:     []string{"rm *"},
		},
	})

	args := func(cmd string) map[string]any {
		return map[string]any{"command": cmd}
	}

	if !ta.AutoApprove("shell", args("go test")) {
		t.Error("shell should be approved by allow pattern 'go *'")
	}
	if !ta.AutoApprove("shell", args("ls")) {
		t.Error("shell should be approved by allow pattern 'ls'")
	}
	if ta.AutoApprove("shell", args("rm file.txt")) {
		t.Error("shell should be denied by deny pattern 'rm *'")
	}
	if ta.AutoApprove("shell", args("curl http://example.com")) {
		t.Error("shell should not approve unmatched command")
	}
}

func TestApproval_ShellDenyTakesPrecedence(t *testing.T) {
	ta := Build("", map[string]config.ToolConfig{
		"shell": {
			Approval: "confirm",
			Allow:    []string{"*"},
			Deny:     []string{"rm *"},
		},
	})

	args := func(cmd string) map[string]any {
		return map[string]any{"command": cmd}
	}

	if ta.AutoApprove("shell", args("rm foo")) {
		t.Error("deny should take precedence over allow")
	}
	if !ta.AutoApprove("shell", args("ls")) {
		t.Error("non-denied command should be allowed by wildcard")
	}
}

func TestApproval_ShellAutoSkipsPatterns(t *testing.T) {
	ta := Build("", map[string]config.ToolConfig{
		"shell": {
			Approval: "auto",
			Deny:     []string{"rm *"},
		},
	})

	// When shell is "auto", deny patterns are not checked (approval short-circuits)
	if !ta.AutoApprove("shell", map[string]any{"command": "rm foo"}) {
		t.Error("shell auto should approve without checking deny patterns")
	}
}

func TestApproval_ForSubagent_AllMode(t *testing.T) {
	ta := Build("all", nil)
	child := ta.ForSubagent()

	if !child.AutoApprove("shell", map[string]any{"command": "anything"}) {
		t.Error("all mode subagent should inherit approveAll")
	}
}

func TestApproval_ForSubagent_NoneMode(t *testing.T) {
	ta := Build("none", nil)
	child := ta.ForSubagent()

	if child.AutoApprove("read", nil) {
		t.Error("none mode subagent should inherit deny-all")
	}
}

func TestApproval_ForSubagent_SafeMode(t *testing.T) {
	ta := Build("safe", map[string]config.ToolConfig{
		"edit": {Approval: "auto"},
		"shell": {
			Approval: "confirm",
			Allow:    []string{"go *"},
		},
	})
	child := ta.ForSubagent()

	// Restricted set
	if !child.AutoApprove("read", nil) {
		t.Error("safe subagent should approve read")
	}
	if !child.AutoApprove("compact", nil) {
		t.Error("safe subagent should approve compact")
	}
	if !child.AutoApprove("use_skill", nil) {
		t.Error("safe subagent should approve use_skill")
	}

	// Parent-auto tools downgraded
	if child.AutoApprove("edit", nil) {
		t.Error("safe subagent should not auto-approve edit")
	}
	if child.AutoApprove("subagent", nil) {
		t.Error("safe subagent should not auto-approve subagent")
	}

	// Shell patterns inherited
	if !child.AutoApprove("shell", map[string]any{"command": "go test"}) {
		t.Error("safe subagent should inherit shell allow patterns")
	}
}

func TestApproval_ForSubagent_DefaultMode(t *testing.T) {
	ta := Build("", map[string]config.ToolConfig{
		"read": {Approval: "auto"},
		"edit": {Approval: "auto"},
	})
	child := ta.ForSubagent()

	// Default mode: subagent inherits parent's approval
	if !child.AutoApprove("read", nil) {
		t.Error("default subagent should inherit read auto")
	}
	if !child.AutoApprove("edit", nil) {
		t.Error("default subagent should inherit edit auto")
	}
}

func TestApproval_CLIMerge_YoloOverridesConfig(t *testing.T) {
	// Simulate: config has auto_approve="none", CLI has -yolo
	// main.go resolves: approveMode = "all" (yolo wins, then config not consulted)
	ta := Build("all", map[string]config.ToolConfig{
		"shell": {Approval: "deny"},
	})

	if !ta.AutoApprove("shell", map[string]any{"command": "rm -rf /"}) {
		t.Error("-yolo should override per-tool deny")
	}
}

func TestApproval_CLIMerge_FlagOverridesConfig(t *testing.T) {
	// Simulate: config has auto_approve="all", CLI has -approve none
	// main.go resolves: approveMode = "none" (flag wins)
	ta := Build("none", map[string]config.ToolConfig{
		"read": {Approval: "auto"},
	})

	if ta.AutoApprove("read", nil) {
		t.Error("-approve none should override config auto_approve=all")
	}
}
