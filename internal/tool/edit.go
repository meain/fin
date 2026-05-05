package tool

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

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

// nearMissHint checks if old_string matches after normalizing whitespace.
// If so, it returns a hint showing the whitespace difference.
func nearMissHint(content, oldStr string) string {
	normalized := normalizeWhitespace(oldStr)
	lines := strings.Split(content, "\n")

	// Try to find a contiguous block of lines that matches old_string
	// after whitespace normalization
	oldLines := strings.Split(oldStr, "\n")
	if len(oldLines) == 0 {
		return ""
	}

	firstNorm := normalizeWhitespace(oldLines[0])
	if firstNorm == "" {
		return ""
	}

	for i, line := range lines {
		if normalizeWhitespace(line) != firstNorm {
			continue
		}
		// Check if subsequent lines match
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
		if !match {
			continue
		}

		// Found a whitespace-normalized match — show the diff
		actual := strings.Join(lines[i:i+len(oldLines)], "\n")
		if actual == oldStr {
			continue // exact match means Count should have found it; skip
		}
		return fmt.Sprintf(
			" (whitespace mismatch — use the exact string from the file)\nexpected:\n%s\nactual:\n%s",
			quoted(oldStr), quoted(actual),
		)
	}

	// No whitespace match either — check if the non-whitespace content exists at all
	if strings.Contains(normalizeWhitespace(content), normalized) {
		return " (content exists but whitespace differs — re-read the file and copy the exact string)"
	}

	return ""
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
