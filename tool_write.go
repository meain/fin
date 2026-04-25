package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type writeTool struct{}

func (t *writeTool) Name() string { return "write" }

func (t *writeTool) Description() string {
	return "Write content to a file. Creates the file and any parent directories if they don't exist. Overwrites existing content."
}

func (t *writeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to write",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Content to write to the file",
			},
		},
		"required": []string{"path", "content"},
	}
}

func (t *writeTool) Run(_ context.Context, args map[string]any) (ToolResult, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return ToolResult{}, fmt.Errorf("path is required")
	}
	path = expandHome(path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return ToolResult{}, fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return ToolResult{}, fmt.Errorf("failed to write %s: %w", path, err)
	}

	return ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}
