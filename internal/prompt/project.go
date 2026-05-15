package prompt

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/fsutil"
)

// readAgentsMD reads ~/.agents/AGENTS.md when present and trims trailing
// whitespace.
func readAgentsMD() string {
	data, err := os.ReadFile(filepath.Join(config.HomeDir(), config.AgentsDir, config.AgentsFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// findProjectFile walks up from cwd looking for the named file and returns
// its trimmed content. Empty name short-circuits to "".
func findProjectFile(name string) string {
	if name == "" {
		return ""
	}

	var found string
	fsutil.WalkUpFromCwd(func(dir string) {
		if found != "" {
			return
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil {
			found = strings.TrimSpace(string(data))
		}
	})

	return found
}
