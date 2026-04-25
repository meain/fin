package main

import (
	"context"
	"fmt"
	"strings"
)

// skillTool lets the agent activate skills and read skill resources.
type skillTool struct {
	skills []*Skill
}

func newSkillTool(skills []*Skill) *skillTool {
	return &skillTool{skills: skills}
}

func (t *skillTool) Name() string { return "use_skill" }

func (t *skillTool) Description() string {
	return "Activate a skill to load its full instructions, or read a file from a skill's directory. Pass the skill name to activate it. Optionally pass a file path to read a specific resource from the skill."
}

func (t *skillTool) Parameters() map[string]any {
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

func (t *skillTool) Run(_ context.Context, args map[string]any) (string, error) {
	name, _ := args["name"].(string)
	if name == "" {
		return "", fmt.Errorf("skill name is required")
	}

	// Find the skill
	var skill *Skill
	for _, s := range t.skills {
		if s.Name == name {
			skill = s
			break
		}
	}
	if skill == nil {
		return "", fmt.Errorf("skill %q not found", name)
	}

	// If a file is requested, read it
	if file, ok := args["file"].(string); ok && file != "" {
		content, err := LoadSkillFile(skill, file)
		if err != nil {
			return "", err
		}
		return content, nil
	}

	// Otherwise, activate: load the full body
	body, err := LoadSkillBody(skill)
	if err != nil {
		return "", err
	}

	var result strings.Builder
	fmt.Fprintf(&result, "# Skill: %s\n\n", skill.Name)
	if skill.Compatibility != "" {
		fmt.Fprintf(&result, "Compatibility: %s\n\n", skill.Compatibility)
	}
	result.WriteString(body)

	return result.String(), nil
}
