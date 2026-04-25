package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

type readTool struct{}

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string {
	return "Read the contents of a file. Returns the file content with line numbers."
}

func (t *readTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Absolute or relative path to the file to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Line number to start reading from (0-based). Omit to read from beginning.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read. Omit to read entire file.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *readTool) Run(_ context.Context, args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	lines := strings.Split(string(data), "\n")

	offset := 0
	if v, ok := args["offset"].(float64); ok {
		offset = int(v)
	}
	limit := len(lines)
	if v, ok := args["limit"].(float64); ok {
		limit = int(v)
	}

	if offset > len(lines) {
		offset = len(lines)
	}
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}

	var b strings.Builder
	for i := offset; i < end; i++ {
		fmt.Fprintf(&b, "%d\t%s\n", i+1, lines[i])
	}
	return b.String(), nil
}
