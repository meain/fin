package tool

import (
	"context"
	"fmt"
	"os"
	"strings"

	t "github.com/meain/fin/internal/types"
)

// EditTool edits a file by replacing an exact string match.
type EditTool struct{}

func (et *EditTool) Name() string { return "edit" }

func (et *EditTool) Description() string {
	return "Edit a file by replacing an exact string match. The old_string must appear exactly once in the file. Use this for surgical edits rather than rewriting entire files."
}

func (et *EditTool) Parameters() map[string]any {
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

func (et *EditTool) Run(_ context.Context, args map[string]any) (t.ToolResult, error) {
	path, _ := args["path"].(string)
	oldStr, _ := args["old_string"].(string)
	newStr, _ := args["new_string"].(string)

	if path == "" {
		return t.ToolResult{}, fmt.Errorf("path is required")
	}
	if oldStr == "" {
		return t.ToolResult{}, fmt.Errorf("old_string is required")
	}
	path = t.ExpandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		return t.ToolResult{}, fmt.Errorf("old_string not found in %s", path)
	}
	if count > 1 {
		return t.ToolResult{}, fmt.Errorf("old_string appears %d times in %s (must be unique)", count, path)
	}

	newContent := strings.Replace(content, oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to write %s: %w", path, err)
	}

	return t.ToolResult{Content: fmt.Sprintf("edited %s", path)}, nil
}
