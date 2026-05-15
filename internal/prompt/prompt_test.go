package prompt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectFile_WalksUp(t *testing.T) {
	// Create a temp dir tree: root/a/b/c
	// Put the project file in root/a
	root := t.TempDir()
	deepDir := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deepDir, 0755); err != nil {
		t.Fatal(err)
	}

	projectContent := "This is the project file."
	if err := os.WriteFile(filepath.Join(root, "a", "AGENTS.md"), []byte(projectContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Change cwd to the deep directory so findProjectFile walks up
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(deepDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	result := findProjectFile("AGENTS.md")
	if result != projectContent {
		t.Errorf("expected %q, got %q", projectContent, result)
	}
}

func TestFindProjectFile_NotFound(t *testing.T) {
	// Use a temp dir with no project file anywhere up the tree
	dir := t.TempDir()

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	result := findProjectFile("NONEXISTENT_FILE.md")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestFindProjectFile_EmptyName(t *testing.T) {
	result := findProjectFile("")
	if result != "" {
		t.Errorf("expected empty string for empty name, got %q", result)
	}
}

func TestFindProjectFile_InCurrentDir(t *testing.T) {
	dir := t.TempDir()
	content := "Project instructions here."
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	result := findProjectFile("AGENTS.md")
	if result != content {
		t.Errorf("expected %q, got %q", content, result)
	}
}
