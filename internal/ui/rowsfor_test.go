package ui

import (
	"strings"
	"testing"
)

// TestRawTermWidthVsGetTermWidth guards the split between the two width
// helpers: getTermWidth() keeps its -1 safety margin (for truncation),
// rawTermWidth() must return the real column count used for wrap math.
func TestRawTermWidthVsGetTermWidth(t *testing.T) {
	raw := rawTermWidth()
	clamped := getTermWidth()
	if clamped != raw-1 {
		t.Fatalf("getTermWidth() = %d, want rawTermWidth()-1 = %d", clamped, raw-1)
	}
}

// TestRowsForUsesRawTermWidth guards the fix for cursor-drift on long,
// unclamped lines (e.g. a shell command's full label, which is allowed to
// wrap instead of being truncated): rowsFor must count wraps against the
// real terminal width, not getTermWidth()'s narrower, margin-adjusted one —
// otherwise repeated redraws progressively miscount cursor-up distance and
// corrupt the display.
func TestRowsForUsesRawTermWidth(t *testing.T) {
	w := rawTermWidth()

	cases := []struct {
		name string
		n    int
		want int
	}{
		{"empty", 0, 1},
		{"one char", 1, 1},
		{"exactly one row", w, 1},
		{"one over a row", w + 1, 2},
		{"exactly two rows", 2 * w, 2},
		{"one over two rows", 2*w + 1, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := strings.Repeat("x", c.n)
			if got := rowsFor(s); got != c.want {
				t.Fatalf("rowsFor(%d chars) = %d, want %d (raw width=%d)", c.n, got, c.want, w)
			}
		})
	}
}

// TestBlockLinesLongShellCommandRowsMatchRowsFor is an end-to-end guard: the
// expanded header for a long, unclamped shell command must have its row
// count (as used for cursor-up math) computed consistently by rowsFor,
// using the real terminal width rather than under-counting and drifting
// the redraw cursor position over repeated frames.
func TestBlockLinesLongShellCommandRowsMatchRowsFor(t *testing.T) {
	u := New(nil, OutputDefault, false)
	defer u.Close()

	longCmd := "echo " + strings.Repeat("y", 300)
	tl := toolLineState{
		name:    "shell",
		args:    map[string]any{"command": longCmd},
		running: true,
	}

	lines := u.blockLines(tl)
	if len(lines) != 1 {
		t.Fatalf("expected 1 header line (no output yet), got %d", len(lines))
	}

	w := rawTermWidth()
	// The header (containing the full, unclamped command) must span more
	// than one row once it exceeds the real terminal width.
	wantRows := (len(longCmd) + w - 1) / w
	if wantRows < 2 {
		t.Fatalf("test command too short to exercise wrapping at width %d", w)
	}
	if got := rowsFor(lines[0]); got != wantRows {
		t.Fatalf("rowsFor(header) = %d, want %d (raw width=%d, header visible len should track command length)", got, wantRows, w)
	}
}
