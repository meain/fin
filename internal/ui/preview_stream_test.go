package ui

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	t "github.com/meain/fin/internal/types"
)

// capture redirects the package-level stdout var to a pipe for the duration
// of fn, returning everything written to it.
func capture(t2 *testing.T, fn func(*UI)) string {
	t2.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t2.Fatal(err)
	}
	old := stdout
	stdout = w
	u := New(nil, OutputDefault, false)
	fn(u)
	u.Close()
	stdout = old
	w.Close()

	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}

// TestWriteStreamsProgressively guards the fix for "write never streams
// output": its content is fully known upfront, so it must still be revealed
// into the scrollback one line at a time (like shell's real output) rather
// than flashing in all at once, then collapse cleanly once done.
func TestWriteStreamsProgressively(t2 *testing.T) {
	writeArgs := map[string]any{
		"path":    "/tmp/example.go",
		"content": "line1\nline2\nline3\nline4\nline5\n",
	}

	out := capture(t2, func(u *UI) {
		u.ToolStart(0, 1, "write", writeArgs)
		time.Sleep(400 * time.Millisecond) // let several lines reveal
		u.ToolDone(0, "write", writeArgs, t.ToolResult{Content: "wrote 30 bytes to /tmp/example.go"}, nil)
		time.Sleep(600 * time.Millisecond) // let hold + finalize settle
	})

	lineCounts := map[int]bool{}
	for _, chunk := range strings.Split(out, "\033[0J") {
		count := 0
		for i := 1; i <= 5; i++ {
			if strings.Contains(chunk, fmt.Sprintf("line%d", i)) {
				count++
			}
		}
		if count > 0 {
			lineCounts[count] = true
		}
	}
	if len(lineCounts) < 3 {
		t2.Fatalf("expected a progressive reveal (multiple distinct partial line counts), got %v", lineCounts)
	}
	if !lineCounts[5] {
		t2.Fatalf("expected a frame showing all 5 lines eventually, counts seen=%v", lineCounts)
	}

	redrawFrames := strings.Split(out, "\033[0J")
	lastFrame := redrawFrames[len(redrawFrames)-1]
	if strings.Contains(lastFrame, "line5") || strings.Contains(lastFrame, "line1") {
		t2.Fatalf("final frame still shows scrollback content, not collapsed:\n%s", lastFrame)
	}
	if !strings.Contains(lastFrame, "write") || !strings.Contains(lastFrame, "example.go") {
		t2.Fatalf("final frame missing expected collapsed summary:\n%s", lastFrame)
	}
}

// TestEditStreamsDiffPreview verifies edit's diff-style preview (removed
// then added lines) streams in the same way as write's content.
func TestEditStreamsDiffPreview(t2 *testing.T) {
	editArgs := map[string]any{
		"path":       "/tmp/example.go",
		"old_string": "aaa",
		"new_string": "bbb\nccc\nddd",
	}
	out := capture(t2, func(u *UI) {
		u.ToolStart(0, 1, "edit", editArgs)
		time.Sleep(600 * time.Millisecond)
		u.ToolDone(0, "edit", editArgs, t.ToolResult{Content: "edited /tmp/example.go"}, nil)
		time.Sleep(600 * time.Millisecond)
	})
	for _, want := range []string{"- aaa", "+ bbb", "+ ccc", "+ ddd"} {
		if !strings.Contains(out, want) {
			t2.Fatalf("expected diff preview %q, got:\n%s", want, out)
		}
	}
}

// TestShellStillStreamsRealOutput ensures the write/edit synthetic-preview
// change didn't regress shell's genuine incremental OnOutput streaming.
func TestShellStillStreamsRealOutput(t2 *testing.T) {
	shellArgs := map[string]any{"command": "for i in 1 2 3; do echo line-$i; done"}
	out := capture(t2, func(u *UI) {
		u.ToolStart(0, 1, "shell", shellArgs)
		for i := 1; i <= 3; i++ {
			time.Sleep(50 * time.Millisecond)
			u.ToolOutput(0, fmt.Sprintf("line-%d", i), i)
		}
		time.Sleep(500 * time.Millisecond)
		u.ToolDone(0, "shell", shellArgs, t.ToolResult{Content: "line-1\nline-2\nline-3\n"}, nil)
		time.Sleep(500 * time.Millisecond)
	})
	for _, want := range []string{"line-1", "line-2", "line-3"} {
		if !strings.Contains(out, want) {
			t2.Fatalf("expected shell output %q to appear somewhere, got:\n%s", want, out)
		}
	}
	redrawFrames := strings.Split(out, "\033[0J")
	last := redrawFrames[len(redrawFrames)-1]
	if strings.Contains(last, "line-1") {
		t2.Fatalf("shell block did not collapse at the end:\n%s", last)
	}
}

// TestReadCollapsesImmediately ensures tools with no preview/content (read,
// use_skill, ...) still collapse right away, without the artificial hold
// that write/edit/shell get when they have something to show.
func TestReadCollapsesImmediately(t2 *testing.T) {
	start := time.Now()
	var doneAt time.Duration
	capture(t2, func(u *UI) {
		u.ToolStart(0, 1, "read", map[string]any{"path": "foo.go"})
		time.Sleep(20 * time.Millisecond)
		u.ToolDone(0, "read", map[string]any{"path": "foo.go"}, t.ToolResult{Content: "package main\n"}, nil)
		time.Sleep(100 * time.Millisecond)
		doneAt = time.Since(start)
	})
	if doneAt > 200*time.Millisecond {
		t2.Fatalf("read tool call took %s to settle, expected near-instant collapse (no hold)", doneAt)
	}
}
