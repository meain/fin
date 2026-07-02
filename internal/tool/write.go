package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meain/fin/internal/fsutil"
	t "github.com/meain/fin/internal/types"
)

// WriteTool writes content to a file.
type WriteTool struct{}

func (wt *WriteTool) Name() string { return "write" }

func (wt *WriteTool) Description() string {
	return "Write content to a file. Creates the file and any parent directories if they don't exist. Overwrites existing content."
}

func (wt *WriteTool) PrimaryArg(args map[string]any) string {
	path, _ := args["path"].(string)
	return path
}

func (wt *WriteTool) Label(args map[string]any) ToolLabel {
	path, _ := args["path"].(string)
	return ToolLabel{Primary: path}
}

// Preview returns the content lines to show in the expanded live view. The
// full content is already known before Run executes, so there's nothing to
// stream incrementally — the whole thing is shown (scrollback trims it to
// the last few lines, same as a running shell command).
func (wt *WriteTool) Preview(args map[string]any) []string {
	content, _ := args["content"].(string)
	return previewLines(strings.TrimRight(content, "\n"))
}

func (wt *WriteTool) Parameters() map[string]any {
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

func (wt *WriteTool) Run(_ context.Context, args map[string]any) (t.ToolResult, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" {
		return t.ToolResult{}, fmt.Errorf("path is required")
	}
	path = fsutil.ExpandHome(path)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to create directories: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to write %s: %w", path, err)
	}

	return t.ToolResult{Content: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}
