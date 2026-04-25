package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTool_NewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "output.txt")

	wt := &WriteTool{}
	result, err := wt.Run(context.Background(), map[string]any{
		"path":    path,
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "wrote") {
		t.Errorf("expected 'wrote' in result, got: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("file content = %q, want %q", string(data), "hello world")
	}
}

func TestWriteTool_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "deep.txt")

	wt := &WriteTool{}
	_, err := wt.Run(context.Background(), map[string]any{
		"path":    path,
		"content": "deep content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "deep content" {
		t.Errorf("file content = %q, want %q", string(data), "deep content")
	}
}

func TestWriteTool_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}

	wt := &WriteTool{}
	_, err := wt.Run(context.Background(), map[string]any{
		"path":    path,
		"content": "new content",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "new content" {
		t.Errorf("file content = %q, want %q", string(data), "new content")
	}
}
