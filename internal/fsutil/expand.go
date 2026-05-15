package fsutil

import (
	"os"
	"path/filepath"
)

// ExpandHome expands ~/... paths to the user's home directory.
func ExpandHome(path string) string {
	if len(path) >= 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
