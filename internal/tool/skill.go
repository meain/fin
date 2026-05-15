package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	t "github.com/meain/fin/internal/types"
)

// SkillEntry holds the minimal skill data needed by the skill tool.
type SkillEntry struct {
	Name          string
	Description   string
	Compatibility string
	Dir           string
	Body          string // if set, used instead of loading from Dir
}

// SkillTool lets the agent activate skills and read skill resources.
type SkillTool struct {
	Skills []SkillEntry
}

func (st *SkillTool) Name() string { return "use_skill" }

func (st *SkillTool) Label(args map[string]any) ToolLabel {
	name, _ := args["name"].(string)
	return ToolLabel{Primary: name}
}

func (st *SkillTool) Description() string {
	return "Activate a skill to load its full instructions, or read a file from a skill's directory. Pass the skill name to activate it. Optionally pass a file path to read a specific resource from the skill."
}

func (st *SkillTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The name of the skill to activate",
			},
			"file": map[string]any{
				"type":        "string",
				"description": "Optional: relative path to a file within the skill directory to read (e.g. 'references/REFERENCE.md', 'scripts/extract.py')",
			},
		},
		"required": []string{"name"},
	}
}

func (st *SkillTool) Run(_ context.Context, args map[string]any) (t.ToolResult, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return t.ToolResult{}, fmt.Errorf("skill name is required")
	}

	// Find the skill
	var skill *SkillEntry
	for i := range st.Skills {
		if st.Skills[i].Name == name {
			skill = &st.Skills[i]
			break
		}
	}
	if skill == nil {
		return t.ToolResult{}, fmt.Errorf("skill %q not found", name)
	}

	// If a file is requested, read it
	if file, ok := args["file"].(string); ok && file != "" {
		content, err := LoadSkillFile(skill.Dir, file)
		if err != nil {
			return t.ToolResult{}, err
		}
		return t.ToolResult{Content: content}, nil
	}

	// Otherwise, activate: load the full body
	var body string
	if skill.Body != "" {
		body = skill.Body
	} else {
		var err error
		body, err = LoadSkillBody(skill.Dir)
		if err != nil {
			return t.ToolResult{}, err
		}
	}

	var result strings.Builder
	fmt.Fprintf(&result, "# Skill: %s\n\n", skill.Name)
	if skill.Compatibility != "" {
		fmt.Fprintf(&result, "Compatibility: %s\n\n", skill.Compatibility)
	}
	result.WriteString(body)

	return t.ToolResult{Content: result.String()}, nil
}

// LoadSkillBody reads and returns the full SKILL.md body for a skill
// given the skill's directory path.
func LoadSkillBody(dir string) (string, error) {
	skillFile := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(skillFile)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", skillFile, err)
	}

	body, err := extractSkillBody(data)
	if err != nil {
		return "", err
	}

	return body, nil
}

// LoadSkillFile reads a file relative to the skill directory.
func LoadSkillFile(dir, relPath string) (string, error) {
	// Prevent path traversal
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid path: must be relative to skill directory")
	}

	absPath := filepath.Join(dir, cleaned)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", relPath, err)
	}

	return string(data), nil
}

// extractSkillBody extracts the markdown body after YAML frontmatter.
func extractSkillBody(data []byte) (string, error) {
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return "", fmt.Errorf("no YAML frontmatter found")
	}

	// Find the closing ---
	rest := content[4:]
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return "", fmt.Errorf("no YAML frontmatter found")
	}

	body := strings.TrimSpace(rest[idx+4:])
	return body, nil
}
