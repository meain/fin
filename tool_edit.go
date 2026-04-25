package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type editTool struct{}

func (t *editTool) Name() string { return "edit" }

func (t *editTool) Description() string {
	return "Edit a file by replacing an exact string match. The old_string must appear exactly once in the file. Use this for surgical edits rather than rewriting entire files."
}

func (t *editTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact string to find in the file. Must be unique.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "String to replace old_string with",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

func (t *editTool) Run(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)

	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if oldStr == "" {
		return "", fmt.Errorf("old_string is required")
	}
	path = expandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("old_string appears %d times in %s (must be unique)", count, path)
	}

	newContent := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}

	return fmt.Sprintf("edited %s", path), nil
}
