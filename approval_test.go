package main

import "testing"

func TestApproval_AllMode(t *testing.T) {
	ta := buildToolApproval("all", nil)

	if !ta.autoApprove("read", nil) {
		t.Error("all mode should approve read")
	}
	if !ta.autoApprove("shell", map[string]any{"command": "rm -rf /"}) {
		t.Error("all mode should approve any shell command")
	}
	if !ta.autoApprove("unknown_tool", nil) {
		t.Error("all mode should approve unknown tools")
	}
}

func TestApproval_NoneMode(t *testing.T) {
	ta := buildToolApproval("none", map[string]ToolConfig{
		"read": {Approval: "auto"},
	})

	if ta.autoApprove("read", nil) {
		t.Error("none mode should deny read even with per-tool auto")
	}
	if ta.autoApprove("shell", nil) {
		t.Error("none mode should deny shell")
	}
}

func TestApproval_SafeMode(t *testing.T) {
	ta := buildToolApproval("safe", map[string]ToolConfig{
		"write": {Approval: "confirm"},
		"edit":  {Approval: "auto"},
	})

	// Safe tools approved
	for _, name := range []string{"read", "use_skill", "compact", "subagent"} {
		if !ta.autoApprove(name, nil) {
			t.Errorf("safe mode should approve %s", name)
		}
	}

	// Per-tool auto still works
	if !ta.autoApprove("edit", nil) {
		t.Error("safe mode should still honor per-tool auto for edit")
	}

	// Confirm tools not approved
	if ta.autoApprove("write", nil) {
		t.Error("safe mode should not approve write with confirm")
	}

	// Unknown tools not approved
	if ta.autoApprove("unknown", nil) {
		t.Error("safe mode should not approve unknown tools")
	}
}

func TestApproval_DefaultMode(t *testing.T) {
	ta := buildToolApproval("", map[string]ToolConfig{
		"read":  {Approval: "auto"},
		"write": {Approval: "confirm"},
		"shell": {Approval: "deny"},
	})

	if !ta.autoApprove("read", nil) {
		t.Error("default mode should approve read with auto")
	}
	if ta.autoApprove("write", nil) {
		t.Error("default mode should not approve write with confirm")
	}
	if ta.autoApprove("shell", nil) {
		t.Error("default mode should not approve shell with deny")
	}
	if ta.autoApprove("unknown", nil) {
		t.Error("default mode should not approve unconfigured tools")
	}
}

func TestApproval_ShellPatterns(t *testing.T) {
	ta := buildToolApproval("", map[string]ToolConfig{
		"shell": {
			Approval: "confirm",
			Allow:    []string{"go *", "ls"},
			Deny:     []string{"rm *"},
		},
	})

	args := func(cmd string) map[string]any {
		return map[string]any{"command": cmd}
	}

	if !ta.autoApprove("shell", args("go test")) {
		t.Error("shell should be approved by allow pattern 'go *'")
	}
	if !ta.autoApprove("shell", args("ls")) {
		t.Error("shell should be approved by allow pattern 'ls'")
	}
	if ta.autoApprove("shell", args("rm file.txt")) {
		t.Error("shell should be denied by deny pattern 'rm *'")
	}
	if ta.autoApprove("shell", args("curl http://example.com")) {
		t.Error("shell should not approve unmatched command")
	}
}

func TestApproval_ShellDenyTakesPrecedence(t *testing.T) {
	ta := buildToolApproval("", map[string]ToolConfig{
		"shell": {
			Approval: "confirm",
			Allow:    []string{"*"},
			Deny:     []string{"rm *"},
		},
	})

	args := func(cmd string) map[string]any {
		return map[string]any{"command": cmd}
	}

	if ta.autoApprove("shell", args("rm foo")) {
		t.Error("deny should take precedence over allow")
	}
	if !ta.autoApprove("shell", args("ls")) {
		t.Error("non-denied command should be allowed by wildcard")
	}
}

func TestApproval_ShellAutoSkipsPatterns(t *testing.T) {
	ta := buildToolApproval("", map[string]ToolConfig{
		"shell": {
			Approval: "auto",
			Deny:     []string{"rm *"},
		},
	})

	// When shell is "auto", deny patterns are not checked (approval short-circuits)
	if !ta.autoApprove("shell", map[string]any{"command": "rm foo"}) {
		t.Error("shell auto should approve without checking deny patterns")
	}
}

func TestApproval_ForSubagent_AllMode(t *testing.T) {
	ta := buildToolApproval("all", nil)
	child := ta.forSubagent()

	if !child.autoApprove("shell", map[string]any{"command": "anything"}) {
		t.Error("all mode subagent should inherit approveAll")
	}
}

func TestApproval_ForSubagent_NoneMode(t *testing.T) {
	ta := buildToolApproval("none", nil)
	child := ta.forSubagent()

	if child.autoApprove("read", nil) {
		t.Error("none mode subagent should inherit deny-all")
	}
}

func TestApproval_ForSubagent_SafeMode(t *testing.T) {
	ta := buildToolApproval("safe", map[string]ToolConfig{
		"edit": {Approval: "auto"},
		"shell": {
			Approval: "confirm",
			Allow:    []string{"go *"},
		},
	})
	child := ta.forSubagent()

	// Restricted set
	if !child.autoApprove("read", nil) {
		t.Error("safe subagent should approve read")
	}
	if !child.autoApprove("compact", nil) {
		t.Error("safe subagent should approve compact")
	}
	if !child.autoApprove("use_skill", nil) {
		t.Error("safe subagent should approve use_skill")
	}

	// Parent-auto tools downgraded
	if child.autoApprove("edit", nil) {
		t.Error("safe subagent should not auto-approve edit")
	}
	if child.autoApprove("subagent", nil) {
		t.Error("safe subagent should not auto-approve subagent")
	}

	// Shell patterns inherited
	if !child.autoApprove("shell", map[string]any{"command": "go test"}) {
		t.Error("safe subagent should inherit shell allow patterns")
	}
}

func TestApproval_ForSubagent_DefaultMode(t *testing.T) {
	ta := buildToolApproval("", map[string]ToolConfig{
		"read": {Approval: "auto"},
		"edit": {Approval: "auto"},
	})
	child := ta.forSubagent()

	// Default mode: subagent inherits parent's approval
	if !child.autoApprove("read", nil) {
		t.Error("default subagent should inherit read auto")
	}
	if !child.autoApprove("edit", nil) {
		t.Error("default subagent should inherit edit auto")
	}
}

func TestApproval_CLIMerge_YoloOverridesConfig(t *testing.T) {
	// Simulate: config has auto_approve="none", CLI has -yolo
	// main.go resolves: approveMode = "all" (yolo wins, then config not consulted)
	ta := buildToolApproval("all", map[string]ToolConfig{
		"shell": {Approval: "deny"},
	})

	if !ta.autoApprove("shell", map[string]any{"command": "rm -rf /"}) {
		t.Error("-yolo should override per-tool deny")
	}
}

func TestApproval_CLIMerge_FlagOverridesConfig(t *testing.T) {
	// Simulate: config has auto_approve="all", CLI has -approve none
	// main.go resolves: approveMode = "none" (flag wins)
	ta := buildToolApproval("none", map[string]ToolConfig{
		"read": {Approval: "auto"},
	})

	if ta.autoApprove("read", nil) {
		t.Error("-approve none should override config auto_approve=all")
	}
}
