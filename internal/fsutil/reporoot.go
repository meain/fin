package fsutil

import (
	"os/exec"
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
