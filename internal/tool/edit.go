package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/meain/fin/internal/fsutil"
	t "github.com/meain/fin/internal/types"
)

// EditTool edits a file by replacing an exact string match.
type EditTool struct{}

func (et *EditTool) Name() string { return "edit" }

func (et *EditTool) PrimaryArg(args map[string]any) string {
	path, _ := args["path"].(string)
	return path
}

func (et *EditTool) Description() string {
	return "Edit a file by replacing an exact string match. The old_string must appear exactly once in the file. Use this for surgical edits rather than rewriting entire files."
}

func (et *EditTool) Label(args map[string]any) ToolLabel {
	path, _ := args["path"].(string)
	old, _ := args["old_string"].(string)
	nw, _ := args["new_string"].(string)
	oldLines := strings.Count(old, "\n") + 1
	newLines := strings.Count(nw, "\n") + 1
	return ToolLabel{
		Primary: path,
		Detail:  fmt.Sprintf("-%d +%d", oldLines, newLines),
	}
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
	path = fsutil.ExpandHome(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return t.ToolResult{}, fmt.Errorf("failed to read %s: %w", path, err)
	}

	content := string(data)
	count := strings.Count(content, oldStr)
	if count == 0 {
		hint := nearMissHint(content, oldStr)
		return t.ToolResult{}, fmt.Errorf("old_string not found in %s%s", path, hint)
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

// nearMissHint diagnoses a Replace failure: when old_string doesn't match
// exactly, return a hint pointing at whitespace differences (or just confirm
// the content exists modulo whitespace).
func nearMissHint(content, oldStr string) string {
	lines := strings.Split(content, "\n")
	oldLines := strings.Split(oldStr, "\n")

	if start, ok := findWhitespaceMatch(lines, oldLines); ok {
		actual := strings.Join(lines[start:start+len(oldLines)], "\n")
		if actual != oldStr {
			return fmt.Sprintf(
				" (whitespace mismatch — use the exact string from the file)\nexpected:\n%s\nactual:\n%s",
				quoted(oldStr), quoted(actual),
			)
		}
	}

	if strings.Contains(normalizeWhitespace(content), normalizeWhitespace(oldStr)) {
		return " (content exists but whitespace differs — re-read the file and copy the exact string)"
	}

	return ""
}

// findWhitespaceMatch searches lines for a contiguous block matching oldLines
// after whitespace normalization. Returns the starting index and true on
// match.
func findWhitespaceMatch(lines, oldLines []string) (int, bool) {
	if len(oldLines) == 0 {
		return 0, false
	}
	firstNorm := normalizeWhitespace(oldLines[0])
	if firstNorm == "" {
		return 0, false
	}
	for i, line := range lines {
		if normalizeWhitespace(line) != firstNorm {
			continue
		}
		if i+len(oldLines) > len(lines) {
			continue
		}
		match := true
		for j := 1; j < len(oldLines); j++ {
			if normalizeWhitespace(lines[i+j]) != normalizeWhitespace(oldLines[j]) {
				match = false
				break
			}
		}
		if match {
			return i, true
		}
	}
	return 0, false
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// quoted shows whitespace characters visibly
func quoted(s string) string {
	s = strings.ReplaceAll(s, "\t", "→   ")
	return s
}
