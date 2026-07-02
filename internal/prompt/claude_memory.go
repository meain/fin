package prompt

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meain/fin/internal/config"
)

// claudeMemoryMaxLines and claudeMemoryMaxBytes mirror Claude Code's own
// MEMORY.md load cap, so fin surfaces the same slice of the index.
const (
	claudeMemoryMaxLines = 200
	claudeMemoryMaxBytes = 25 * 1024
)

// ClaudeMemoryDir returns the Claude Code auto-memory directory for the
// current project: ~/.claude/projects/<project>/memory, where <project> is
// the git repository root (falling back to cwd outside a repo) with "/"
// replaced by "-". This mirrors how Claude Code itself derives the path.
func ClaudeMemoryDir() string {
	root, err := os.Getwd()
	if err != nil {
		return ""
	}

	if out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			root = trimmed
		}
	}

	key := strings.ReplaceAll(root, "/", "-")
	return filepath.Join(config.HomeDir(), ".claude", "projects", key, "memory")
}

// ClaudeMemoryPath returns the path to the current project's Claude Code
// MEMORY.md, regardless of whether it exists.
func ClaudeMemoryPath() string {
	dir := ClaudeMemoryDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "MEMORY.md")
}

// readClaudeMemory reads the Claude Code auto-memory MEMORY.md for the
// current project, if present, truncated to the same 200-line/25KB limit
// Claude Code itself applies. It also lists sibling topic files (everything
// in the memory dir except MEMORY.md) so the model knows they exist and can
// read them on demand.
func readClaudeMemory() (content string, topics []string, dir string, truncated bool) {
	dir = ClaudeMemoryDir()
	if dir == "" {
		return "", nil, "", false
	}

	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	if err != nil {
		return "", nil, "", false
	}

	raw := strings.TrimSpace(string(data))
	if raw == "" {
		return "", nil, "", false
	}

	content, truncated = truncateClaudeMemory(raw)

	entries, err := os.ReadDir(dir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() || e.Name() == "MEMORY.md" || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			topics = append(topics, e.Name())
		}
		sort.Strings(topics)
	}

	return content, topics, dir, truncated
}

// truncateClaudeMemory caps s to claudeMemoryMaxLines lines, then to
// claudeMemoryMaxBytes bytes, whichever limit is hit first.
func truncateClaudeMemory(s string) (out string, truncated bool) {
	lines := strings.Split(s, "\n")
	if len(lines) > claudeMemoryMaxLines {
		lines = lines[:claudeMemoryMaxLines]
		truncated = true
	}
	out = strings.Join(lines, "\n")

	if len(out) > claudeMemoryMaxBytes {
		out = out[:claudeMemoryMaxBytes]
		truncated = true
	}

	return out, truncated
}
