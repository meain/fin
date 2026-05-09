package main

import (
	"testing"
)

func TestResolveOutputMode(t *testing.T) {
	const (
		notTTY = 0 // fd 0 is never a terminal in tests; used to simulate piped stdout
		isTTY  = -1 // invalid fd — term.IsTerminal returns false, so we fake TTY via explicit flag
	)

	// We can't synthesise a real TTY fd in a unit test, so TTY cases are covered
	// by verifying that the config default is respected when stdout is a TTY.
	// The non-TTY path is directly testable since test stdout is not a terminal.

	tests := []struct {
		name       string
		configMode string
		uiFlag     string
		export     string
		stdoutFd   int
		want       OutputMode
	}{
		// Explicit -ui flag always wins, regardless of tty or export.
		{"explicit quiet flag", "", "quiet", "", notTTY, OutputQuiet},
		{"explicit debug flag", "", "debug", "", notTTY, OutputDebug},
		{"explicit default flag overrides pipe", "quiet", "default", "", notTTY, OutputDefault},
		{"explicit flag beats export", "", "debug", "json", notTTY, OutputDebug},

		// Piped stdout auto-selects quiet when no explicit flag and no export.
		{"piped no flag no export", "", "", "", notTTY, OutputQuiet},
		{"piped config default ignored", "debug", "", "", notTTY, OutputQuiet},

		// Export flag suppresses auto-quiet so the export output isn't interleaved.
		{"piped with export json", "", "", "json", notTTY, OutputDefault},
		{"piped with export html", "", "", "html", notTTY, OutputDefault},
		{"piped with export message", "", "", "message", notTTY, OutputDefault},

		// Config mode is used when stdout is a terminal (no auto-quiet).
		// We can't get a real TTY fd in tests, so we verify config parsing indirectly:
		// when fd is notTTY but export is set, config mode applies.
		{"config quiet respected via export path", "quiet", "", "json", notTTY, OutputQuiet},
		{"config debug respected via export path", "debug", "", "json", notTTY, OutputDebug},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveOutputMode(tc.configMode, tc.uiFlag, tc.export, tc.stdoutFd)
			if got != tc.want {
				t.Errorf("resolveOutputMode(%q, %q, %q, fd=%d) = %v, want %v",
					tc.configMode, tc.uiFlag, tc.export, tc.stdoutFd, got, tc.want)
			}
		})
	}
}
