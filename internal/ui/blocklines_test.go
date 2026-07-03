package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/meain/fin/internal/render"
	t "github.com/meain/fin/internal/types"
)

// TestBlockLinesTruncatesLongOutputLines guards the fix for scrollback lines
// overflowing the terminal: a single very long streamed/preview line must be
// truncated to the terminal width, not printed in full.
func TestBlockLinesTruncatesLongOutputLines(t2 *testing.T) {
	u := New(nil, OutputDefault, false)
	defer u.Close()

	longLine := strings.Repeat("x", 500)
	shortLine := "short line"
	tl := toolLineState{
		name:      "shell",
		args:      map[string]any{"command": "echo"},
		running:   true,
		start:     time.Now(),
		outputBuf: []string{longLine, shortLine},
	}

	lines := u.blockLines(tl)
	if len(lines) != 3 { // header + 2 output lines
		t2.Fatalf("expected 3 lines (header + 2 output), got %d: %v", len(lines), lines)
	}

	// Long lines must be truncated with a 5-char margin of error to the
	// right of the terminal width, not right up against it.
	width := getTermWidth()
	wantMax := width - 5
	for i, l := range lines[1:] {
		if vl := render.VisibleLen(l); vl > wantMax {
			t2.Fatalf("output line %d has visible length %d, exceeds width-with-margin %d (width=%d):\n%s", i, vl, wantMax, width, l)
		}
	}

	// The short line should be unaffected by truncation.
	if !strings.Contains(lines[2], shortLine) {
		t2.Fatalf("expected short line to pass through unchanged, got %q", lines[2])
	}
}

// TestShellLongOutputDoesNotOverflow is an end-to-end guard: streaming a
// long real shell output line through ToolOutput must never produce a
// rendered frame line wider than the terminal.
func TestShellLongOutputDoesNotOverflow(t2 *testing.T) {
	longLine := strings.Repeat("y", 500)
	shellArgs := map[string]any{"command": "echo long"}

	out := capture(t2, func(u *UI) {
		u.ToolStart(0, 1, "shell", shellArgs)
		u.ToolOutput(0, longLine, 1)
		time.Sleep(300 * time.Millisecond)
		u.ToolDone(0, "shell", shellArgs, t.ToolResult{Content: longLine + "\n"}, nil)
		time.Sleep(500 * time.Millisecond)
	})

	width := getTermWidth()
	for _, frame := range strings.Split(out, "\r\n") {
		if vl := render.VisibleLen(frame); vl > width {
			t2.Fatalf("rendered frame line has visible length %d, exceeds terminal width %d:\n%s", vl, width, frame)
		}
	}
	if !strings.Contains(out, fmt.Sprintf("%.20s", longLine)) {
		t2.Fatalf("expected truncated long line to still appear (prefix), got:\n%s", out)
	}
}
