package tool

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meain/fin/internal/fsutil"
	t "github.com/meain/fin/internal/types"
)

var imageExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".svg":  "image/svg+xml",
}

// ReadTool reads files, images, or directory structures.
type ReadTool struct {
	tracker *FileTracker
}

func (rt *ReadTool) Name() string { return "read" }

func (rt *ReadTool) Description() string {
	return "Read the contents of a file, an image, or list the structure of a directory. Returns file content with line numbers, image data for vision models, or a directory tree."
}

func (rt *ReadTool) Label(args map[string]any) ToolLabel {
	path, _ := args["path"].(string)
	offset, hasOffset := args["offset"].(float64)
	limit, hasLimit := args["limit"].(float64)
	return ToolLabel{Primary: path, Detail: rangeDetail(offset, limit, hasOffset, hasLimit)}
}

func (rt *ReadTool) Parameters() map[string]any {
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

func (rt *ReadTool) Run(_ context.Context, args map[string]any) (t.ToolResult, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return t.ToolResult{}, fmt.Errorf("path is required")
	}
	path = fsutil.ExpandHome(path)

	info, err := os.Stat(path)
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	if info.IsDir() {
		content, err := readDir(path)
		return t.ToolResult{Content: content}, err
	}

	// Check if it's an image
	ext := strings.ToLower(filepath.Ext(path))
	if mediaType, ok := imageExtensions[ext]; ok {
		return readImage(path, mediaType)
	}

	content, err := readFile(path, args)
	if err == nil && rt.tracker != nil {
		rt.tracker.RecordRead(path)
	}
	return t.ToolResult{Content: content}, err
}

func readImage(path, mediaType string) (t.ToolResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	encoded := base64.StdEncoding.EncodeToString(data)
	return t.ToolResult{
		Content: fmt.Sprintf("[image: %s (%d bytes)]", filepath.Base(path), len(data)),
		Images: []t.Image{{
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
	end = min(end, len(lines))

	var b strings.Builder
	for i := offset; i < end; i++ {
		fmt.Fprintf(&b, "%d│%s\n", i+1, lines[i])
	}
	return b.String(), nil
}

const maxDirEntries = 1000

func readDir(root string) (string, error) {
	// Count total entries to decide between tree and flat listing.
	total := 0
	tooMany := false
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		depth := len(parts) - 1
		if depth > 3 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		total++
		if total > maxDirEntries {
			tooMany = true
			return filepath.SkipAll
		}
		return nil
	})

	if tooMany {
		return readDirFlat(root)
	}
	return readDirTree(root)
}

// readDirFlat lists only the immediate children of root.
func readDirFlat(root string) (string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", fmt.Errorf("failed to read directory %s: %w", root, err)
	}

	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		isHidden := strings.HasPrefix(name, ".")
		if e.IsDir() {
			if isHidden {
				fmt.Fprintf(&b, "%s/ [hidden, contents not shown]\n", name)
			} else {
				fmt.Fprintf(&b, "%s/\n", name)
			}
		} else if !isHidden {
			fmt.Fprintf(&b, "%s\n", name)
		}
	}
	fmt.Fprintf(&b, "\n[directory has too many entries for full tree — showing top-level only. Read specific subdirectories to explore further.]\n")
	return b.String(), nil
}

// readDirTree walks up to depth 3, printing an indented tree.
func readDirTree(root string) (string, error) {
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
		depth := len(parts) - 1
		if depth > 3 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		name := info.Name()
		isHidden := strings.HasPrefix(name, ".")
		indent := strings.Repeat("  ", depth)

		if info.IsDir() {
			if isHidden {
				fmt.Fprintf(&b, "%s%s/ [hidden, contents not shown]\n", indent, name)
				return filepath.SkipDir
			}
			fmt.Fprintf(&b, "%s%s/\n", indent, name)
		} else if !isHidden {
			fmt.Fprintf(&b, "%s%s\n", indent, name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}
