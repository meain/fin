package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTruncateClaudeMemory_UnderLimit(t *testing.T) {
	s := "line1\nline2\nline3"
	out, truncated := truncateClaudeMemory(s)
	if truncated {
		t.Errorf("expected no truncation, got truncated")
	}
	if out != s {
		t.Errorf("expected %q, got %q", s, out)
	}
}

func TestTruncateClaudeMemory_LineLimit(t *testing.T) {
	lines := make([]string, claudeMemoryMaxLines+50)
	for i := range lines {
		lines[i] = "line"
	}
	s := strings.Join(lines, "\n")

	out, truncated := truncateClaudeMemory(s)
	if !truncated {
		t.Errorf("expected truncation for %d lines", len(lines))
	}
	if got := len(strings.Split(out, "\n")); got != claudeMemoryMaxLines {
		t.Errorf("expected %d lines, got %d", claudeMemoryMaxLines, got)
	}
}

func TestTruncateClaudeMemory_ByteLimit(t *testing.T) {
	// A single line well beyond the byte cap, safely under the line cap.
	s := strings.Repeat("x", claudeMemoryMaxBytes*2)

	out, truncated := truncateClaudeMemory(s)
	if !truncated {
		t.Errorf("expected truncation for oversized content")
	}
	if len(out) != claudeMemoryMaxBytes {
		t.Errorf("expected %d bytes, got %d", claudeMemoryMaxBytes, len(out))
	}
}

// withFakeHomeAndCwd points HOME at a fresh temp dir and chdir's into
// another fresh temp dir (outside any git repo), restoring both on cleanup.
// It returns the fake HOME dir.
func withFakeHomeAndCwd(t *testing.T) string {
	t.Helper()

	origHome := os.Getenv("HOME")
	t.Cleanup(func() { os.Setenv("HOME", origHome) })
	fakeHome := t.TempDir()
	os.Setenv("HOME", fakeHome)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(origDir) })

	workDir := t.TempDir()
	// Resolve symlinks (e.g. macOS /var -> /private/var) so the derived
	// project key matches what os.Getwd() reports after chdir.
	resolved, err := filepath.EvalSymlinks(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(resolved); err != nil {
		t.Fatal(err)
	}

	return fakeHome
}

func TestReadClaudeMemory_NotFound(t *testing.T) {
	withFakeHomeAndCwd(t)

	content, topics, dir, truncated := readClaudeMemory()
	if content != "" || topics != nil || dir != "" || truncated {
		t.Errorf("expected empty result, got content=%q topics=%v dir=%q truncated=%v", content, topics, dir, truncated)
	}
}

func TestReadClaudeMemory_Found(t *testing.T) {
	withFakeHomeAndCwd(t)

	memDir := ClaudeMemoryDir()
	if memDir == "" {
		t.Fatal("expected non-empty memory dir")
	}
	if err := os.MkdirAll(memDir, 0755); err != nil {
		t.Fatal(err)
	}

	indexContent := "- [foo.md](foo.md) — some learning"
	if err := os.WriteFile(filepath.Join(memDir, "MEMORY.md"), []byte(indexContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memDir, "foo.md"), []byte("detail"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-.md files must be ignored.
	if err := os.WriteFile(filepath.Join(memDir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	content, topics, dir, truncated := readClaudeMemory()
	if content != indexContent {
		t.Errorf("expected %q, got %q", indexContent, content)
	}
	if truncated {
		t.Errorf("expected no truncation")
	}
	if dir != memDir {
		t.Errorf("expected dir %q, got %q", memDir, dir)
	}
	if len(topics) != 1 || topics[0] != "foo.md" {
		t.Errorf("expected topics [foo.md], got %v", topics)
	}
}

func TestClaudeMemoryPath(t *testing.T) {
	withFakeHomeAndCwd(t)

	p := ClaudeMemoryPath()
	if !strings.HasSuffix(p, "/memory/MEMORY.md") {
		t.Errorf("expected path to end with /memory/MEMORY.md, got %q", p)
	}
}
