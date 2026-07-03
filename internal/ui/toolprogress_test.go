package ui

import (
	"strings"
	"testing"
)

// TestExtractPathArg covers the best-effort partial-JSON scan used to pull
// a "path" field out of a tool call's arguments while they're still
// streaming in.
func TestExtractPathArg(t *testing.T) {
	cases := []struct {
		name string
		args string
		want string
	}{
		{
			name: "complete simple path",
			args: `{"path": "/tmp/foo.go", "content": "hello`,
			want: "/tmp/foo.go",
		},
		{
			name: "path with escaped characters",
			args: `{"path": "/tmp/a \"quoted\" dir/foo.go", "content":`,
			want: `/tmp/a "quoted" dir/foo.go`,
		},
		{
			name: "path not yet closed",
			args: `{"path": "/tmp/fo`,
			want: "",
		},
		{
			name: "no path field at all",
			args: `{"command": "ls -la`,
			want: "",
		},
		{
			name: "empty input",
			args: ``,
			want: "",
		},
		{
			name: "path key present but value not started",
			args: `{"path":`,
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractPathArg(c.args)
			if got != c.want {
				t.Fatalf("extractPathArg(%q) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

// TestToolCallProgressShowsPathBeforeContentStreams guards the fix for
// write/edit's expanded progress not appearing until the whole (possibly
// large) argument payload had streamed in: once "path" is fully parseable,
// it must show immediately, even with zero content lines streamed so far.
func TestToolCallProgressShowsPathBeforeContentStreams(t *testing.T) {
	for _, name := range []string{"write", "edit", "read"} {
		t.Run(name, func(t *testing.T) {
			out := capture(t, func(u *UI) {
				// path is fully known, but content/old_string hasn't even
				// started streaming yet — no lines counted.
				u.ToolCallProgress(name, `{"path": "/tmp/example.go", "content": "`)
			})
			if !strings.Contains(out, "/tmp/example.go") {
				t.Fatalf("expected path to appear in progress output before content streamed, got:\n%s", out)
			}
			if strings.Contains(out, "lines)") {
				t.Fatalf("did not expect a line count yet (no newlines streamed), got:\n%s", out)
			}
		})
	}
}

// TestToolCallProgressNoPathArgToolsUnaffected ensures tools without a
// "path" argument (shell, subagent, ...) keep the original behavior: no
// output until content lines actually stream in.
func TestToolCallProgressNoPathArgToolsUnaffected(t *testing.T) {
	out := capture(t, func(u *UI) {
		u.ToolCallProgress("shell", `{"command": "ls -la /tmp`)
	})
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected no progress output yet for shell with no newlines, got:\n%q", out)
	}
}

// TestToolCallProgressLineCountAppearsAfterPath verifies the line count
// suffix still shows up once content starts streaming, alongside the path
// shown earlier.
func TestToolCallProgressLineCountAppearsAfterPath(t *testing.T) {
	out := capture(t, func(u *UI) {
		u.ToolCallProgress("write", `{"path": "/tmp/example.go", "content": "line1`)
		u.ToolCallProgress("write", `{"path": "/tmp/example.go", "content": "line1\nline2`)
	})
	if !strings.Contains(out, "/tmp/example.go") {
		t.Fatalf("expected path in output, got:\n%s", out)
	}
	if !strings.Contains(out, "(1 lines)") {
		t.Fatalf("expected line count to appear once a newline streamed in, got:\n%s", out)
	}
}
