package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

type readTool struct{}

func (t *readTool) Name() string { return "read" }

func (t *readTool) Description() string {
	return "Read the contents of a file, an image, or list the structure of a directory. Returns file content with line numbers, image data for vision models, or a directory tree."
}

func (t *readTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file, image, or directory to read",
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "Line number to start reading from (0-based). Only for text files.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Maximum number of lines to read. Only for text files.",
			},
		},
		"required": []string{"path"},
	}
}

func (t *readTool) Run(_ context.Context, args map[string]any) (ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return ToolResult{}, fmt.Errorf("path is required")
	}
	path = expandHome(path)

	info, err := os.Stat(path)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	if info.IsDir() {
		content, err := readDir(path)
		return ToolResult{Content: content}, err
	}

	// Check if it's an image
	ext := strings.ToLower(filepath.Ext(path))
	if mediaType, ok := imageExtensions[ext]; ok {
		return readImage(path, mediaType)
	}

	content, err := readFile(path, args)
	return ToolResult{Content: content}, err
}

func readImage(path, mediaType string) (ToolResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolResult{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return ToolResult{
		Content: fmt.Sprintf("[image: %s (%d bytes)]", filepath.Base(path), len(data)),
		Images: []Image{{
			MediaType: mediaType,
			Data:      encoded,
		}},
	}, nil
}

func readFile(path string, args map[string]any) (string, error) {
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

func readDir(root string) (string, error) {
	var b strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}

		parts := strings.Split(rel, string(filepath.Separator))
		for _, p := range parts {
			if strings.HasPrefix(p, ".") {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		depth := len(parts) - 1
		if depth > 3 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		indent := strings.Repeat("  ", depth)
		name := info.Name()
		if info.IsDir() {
			fmt.Fprintf(&b, "%s%s/\n", indent, name)
		} else {
			fmt.Fprintf(&b, "%s%s\n", indent, name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}
