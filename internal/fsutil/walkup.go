package fsutil

import (
	"os"
	"path/filepath"
)

// WalkUpFromCwd invokes visit for the cwd and each parent directory until the
// filesystem root. Returns silently if cwd cannot be resolved.
func WalkUpFromCwd(visit func(dir string)) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		visit(dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}
