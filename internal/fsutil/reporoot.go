package fsutil

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoRoot returns the repository root for dir, trying git then jj. Falls
// back to dir itself if neither VCS is detected.
func RepoRoot(dir string) string {
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return trimmed
		}
	}
	if out, err := exec.Command("jj", "-R", dir, "root").Output(); err == nil {
		if trimmed := strings.TrimSpace(string(out)); trimmed != "" {
			return trimmed
		}
	}
	return dir
}

// RepoName returns the basename of RepoRoot(dir), suitable for embedding in
// a filename or displaying as a short repo identifier. Returns "" for
// degenerate cases (e.g. dir is "/" or ".") where a basename would be "/"
// or "." — neither is a meaningful repo name, and "/" in particular would
// corrupt a filename that embeds it.
func RepoName(dir string) string {
	base := filepath.Base(RepoRoot(dir))
	if base == "/" || base == "." {
		return ""
	}
	return base
}
