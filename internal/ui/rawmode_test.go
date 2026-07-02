package ui

import (
	"bytes"
	"os"
	"testing"
	"time"

	t "github.com/meain/fin/internal/types"
)

// TestRawModeCRLF guards against a real regression: redrawAllTools once
// wrote multi-line tool blocks via fmt.Fprintln directly to stdout. Raw
// terminal mode disables OPOST, so a bare "\n" doesn't return the cursor to
// column 0 — every line after the first drifted right, and the expanded
// view rendered garbled (looked like it never appeared, only the final
// single-line collapse showed cleanly). Every advance to a new line must be
// "\r\n" explicitly, matching the CRLF translation u.write() gets for free
// via x/term.Terminal.Write.
func TestRawModeCRLF(t2 *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t2.Fatal(err)
	}
	oldStdout := stdout
	stdout = w
	defer func() { stdout = oldStdout }()

	u := New(nil, OutputDefault, false)
	editArgs := map[string]any{
		"path":       "/tmp/example.go",
		"old_string": "a",
		"new_string": "b\nc",
	}
	u.ToolStart(0, 1, "edit", editArgs)
	time.Sleep(50 * time.Millisecond)
	u.ToolDone(0, "edit", editArgs, t.ToolResult{Content: "edited /tmp/example.go"}, nil)
	time.Sleep(500 * time.Millisecond)
	u.Close()
	w.Close()

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	bad := 0
	for i, c := range out {
		if c == '\n' && (i == 0 || out[i-1] != '\r') {
			bad++
		}
	}
	if bad > 0 {
		t2.Fatalf("found %d bare newline(s) not preceded by \\r in tool block output — will misalign in raw terminal mode", bad)
	}
}
