package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeSkillDir(t *testing.T, name string) (string, SkillEntry) {
	t.Helper()
	dir := t.TempDir()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skillMD := `---
name: ` + name + `
description: A test skill
---

This is the skill body.
Instructions go here.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatal(err)
	}

	return skillDir, SkillEntry{
		Name:        name,
		Description: "A test skill",
		Dir:         skillDir,
	}
}

func TestSkillTool_Activate(t *testing.T) {
	_, entry := makeSkillDir(t, "test-skill")

	st := &SkillTool{Skills: []SkillEntry{entry}}
	result, err := st.Run(context.Background(), map[string]any{
		"name": "test-skill",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "# Skill: test-skill") {
		t.Errorf("expected skill header, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "This is the skill body.") {
		t.Errorf("expected skill body, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "Instructions go here.") {
		t.Errorf("expected instructions, got:\n%s", result.Content)
	}
}

func TestSkillTool_ReadFile(t *testing.T) {
	skillDir, entry := makeSkillDir(t, "file-skill")

	// Create a resource file inside the skill directory
	refDir := filepath.Join(skillDir, "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refDir, "REF.md"), []byte("reference content"), 0644); err != nil {
		t.Fatal(err)
	}

	st := &SkillTool{Skills: []SkillEntry{entry}}
	result, err := st.Run(context.Background(), map[string]any{
		"name": "file-skill",
		"file": "references/REF.md",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "reference content" {
		t.Errorf("expected %q, got %q", "reference content", result.Content)
	}
}

func TestSkillTool_NotFound(t *testing.T) {
	st := &SkillTool{Skills: []SkillEntry{}}
	_, err := st.Run(context.Background(), map[string]any{
		"name": "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for skill not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}
