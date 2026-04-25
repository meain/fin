package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditTool_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	et := &EditTool{}
	result, err := et.Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "world",
		"new_string": "gopher",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "edited") {
		t.Errorf("expected 'edited' in result, got: %s", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello gopher" {
		t.Errorf("file content = %q, want %q", string(data), "hello gopher")
	}
}

func TestEditTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	et := &EditTool{}
	_, err := et.Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "missing",
		"new_string": "replacement",
	})
	if err == nil {
		t.Fatal("expected error for old_string not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEditTool_MultipleTimes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("aaa bbb aaa"), 0644); err != nil {
		t.Fatal(err)
	}

	et := &EditTool{}
	_, err := et.Run(context.Background(), map[string]any{
		"path":       path,
		"old_string": "aaa",
		"new_string": "ccc",
	})
	if err == nil {
		t.Fatal("expected error for multiple matches")
	}
	if !strings.Contains(err.Error(), "2 times") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestEditTool_PathRequired(t *testing.T) {
	et := &EditTool{}
	_, err := et.Run(context.Background(), map[string]any{
		"old_string": "foo",
		"new_string": "bar",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("unexpected error: %v", err)
	}
}
