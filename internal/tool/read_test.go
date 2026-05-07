package tool

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTool_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("line one\nline two\nline three\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rt := &ReadTool{}
	result, err := rt.Run(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify line numbers are present
	if !strings.Contains(result.Content, "1│line one") {
		t.Errorf("expected line 1 with content, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "2│line two") {
		t.Errorf("expected line 2 with content, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "3│line three") {
		t.Errorf("expected line 3 with content, got:\n%s", result.Content)
	}
}

func TestReadTool_OffsetLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\nd\ne\n"), 0644); err != nil {
		t.Fatal(err)
	}

	rt := &ReadTool{}
	result, err := rt.Run(context.Background(), map[string]any{
		"path":   path,
		"offset": float64(1), // 0-based, so skip first line
		"limit":  float64(2),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should contain lines 2 and 3 (b, c) with 1-based line numbers
	if !strings.Contains(result.Content, "2│b") {
		t.Errorf("expected line 2 (b), got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "3│c") {
		t.Errorf("expected line 3 (c), got:\n%s", result.Content)
	}
	// Should NOT contain line 1 or line 4
	if strings.Contains(result.Content, "1│a") {
		t.Errorf("should not contain line 1, got:\n%s", result.Content)
	}
	if strings.Contains(result.Content, "4│d") {
		t.Errorf("should not contain line 4, got:\n%s", result.Content)
	}
}

func TestReadTool_Directory(t *testing.T) {
	dir := t.TempDir()
	// Create some files and a subdirectory
	if err := os.WriteFile(filepath.Join(dir, "file1.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(dir, "sub")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subdir, "file2.txt"), []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}

	rt := &ReadTool{}
	result, err := rt.Run(context.Background(), map[string]any{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result.Content, "file1.txt") {
		t.Errorf("expected file1.txt in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "sub/") {
		t.Errorf("expected sub/ in output, got:\n%s", result.Content)
	}
	if !strings.Contains(result.Content, "file2.txt") {
		t.Errorf("expected file2.txt in output, got:\n%s", result.Content)
	}
}

func TestReadTool_Image(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.png")

	// Minimal valid 1x1 PNG (67 bytes)
	png := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, // IDAT chunk
		0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, // IEND chunk
		0x44, 0xAE, 0x42, 0x60, 0x82,
	}
	if err := os.WriteFile(path, png, 0644); err != nil {
		t.Fatal(err)
	}

	rt := &ReadTool{}
	result, err := rt.Run(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Images) != 1 {
		t.Fatalf("expected 1 image, got %d", len(result.Images))
	}
	if result.Images[0].MediaType != "image/png" {
		t.Errorf("expected media type image/png, got %s", result.Images[0].MediaType)
	}
	if result.Images[0].Data == "" {
		t.Error("expected non-empty base64 data")
	}
	if !strings.Contains(result.Content, "tiny.png") {
		t.Errorf("expected filename in content, got: %s", result.Content)
	}
}

func TestReadTool_NonexistentFile(t *testing.T) {
	rt := &ReadTool{}
	_, err := rt.Run(context.Background(), map[string]any{"path": "/nonexistent/path/file.txt"})
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "failed to stat") {
		t.Errorf("unexpected error message: %v", err)
	}
}
