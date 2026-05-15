package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillMD_Valid(t *testing.T) {
	data := []byte(`---
name: greet
description: Greet the user warmly
license: MIT
allowed-tools: shell
---
Hello! This is the body of the skill.

It has multiple paragraphs.`)

	skill, err := ParseMD(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Name != "greet" {
		t.Errorf("expected name %q, got %q", "greet", skill.Name)
	}
	if skill.Description != "Greet the user warmly" {
		t.Errorf("expected description %q, got %q", "Greet the user warmly", skill.Description)
	}
	if skill.License != "MIT" {
		t.Errorf("expected license %q, got %q", "MIT", skill.License)
	}
	if skill.AllowedTools != "shell" {
		t.Errorf("expected allowed-tools %q, got %q", "shell", skill.AllowedTools)
	}
	if !strings.Contains(skill.Body, "Hello!") {
		t.Errorf("expected body to contain %q, got %q", "Hello!", skill.Body)
	}
	if !strings.Contains(skill.Body, "multiple paragraphs") {
		t.Errorf("expected body to contain multiple paragraphs")
	}
}

func TestParseSkillMD_MissingName(t *testing.T) {
	data := []byte(`---
description: A skill without a name
---
Body text.`)

	_, err := ParseMD(data)
	if err == nil {
		t.Fatal("expected error for missing name, got nil")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got: %v", err)
	}
}

func TestParseSkillMD_MissingDescription(t *testing.T) {
	data := []byte(`---
name: nodesc
---
Body text.`)

	_, err := ParseMD(data)
	if err == nil {
		t.Fatal("expected error for missing description, got nil")
	}
	if !strings.Contains(err.Error(), "description is required") {
		t.Errorf("expected 'description is required' error, got: %v", err)
	}
}

func TestParseSkillMD_NoFrontmatter(t *testing.T) {
	data := []byte(`Just some markdown without frontmatter.`)

	_, err := ParseMD(data)
	if err == nil {
		t.Fatal("expected error for no frontmatter, got nil")
	}
	if !strings.Contains(err.Error(), "no YAML frontmatter") {
		t.Errorf("expected 'no YAML frontmatter' error, got: %v", err)
	}
}

func TestParseSkillMD_EmptyBody(t *testing.T) {
	data := []byte(`---
name: empty
description: A skill with no body
---
`)

	skill, err := ParseMD(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skill.Body != "" {
		t.Errorf("expected empty body, got %q", skill.Body)
	}
}

func TestScanSkillsDir_Discovery(t *testing.T) {
	dir := t.TempDir()

	// Create a valid skill directory: "deploy" with SKILL.md
	deployDir := filepath.Join(dir, "deploy")
	if err := os.MkdirAll(deployDir, 0755); err != nil {
		t.Fatal(err)
	}
	deploySkill := `---
name: deploy
description: Deploy the application
---
Run the deploy script.`
	if err := os.WriteFile(filepath.Join(deployDir, "SKILL.md"), []byte(deploySkill), 0644); err != nil {
		t.Fatal(err)
	}

	// Create another valid skill directory: "lint"
	lintDir := filepath.Join(dir, "lint")
	if err := os.MkdirAll(lintDir, 0755); err != nil {
		t.Fatal(err)
	}
	lintSkill := `---
name: lint
description: Run linters
---
Lint all the things.`
	if err := os.WriteFile(filepath.Join(lintDir, "SKILL.md"), []byte(lintSkill), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a directory without SKILL.md (should be skipped)
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file (not a directory, should be skipped)
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	skills := scanDir(dir)
	if len(skills) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(skills))
	}

	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
		// Body should be cleared in discovery phase
		if s.Body != "" {
			t.Errorf("expected empty body for skill %q during discovery, got %q", s.Name, s.Body)
		}
		// Dir should be set
		if s.Dir == "" {
			t.Errorf("expected Dir to be set for skill %q", s.Name)
		}
	}
	if !names["deploy"] {
		t.Error("expected to find skill 'deploy'")
	}
	if !names["lint"] {
		t.Error("expected to find skill 'lint'")
	}
}

func TestScanSkillsDir_NameMismatch(t *testing.T) {
	dir := t.TempDir()

	// Skill name doesn't match directory name -- should be skipped
	skillDir := filepath.Join(dir, "mydir")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := `---
name: different-name
description: Mismatched name
---
Body.`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	skills := scanDir(dir)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (name mismatch), got %d", len(skills))
	}
}

func TestScanSkillsDir_NonexistentDir(t *testing.T) {
	skills := scanDir("/nonexistent/path/that/does/not/exist")
	if skills != nil {
		t.Errorf("expected nil for nonexistent dir, got %v", skills)
	}
}
