package types

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("could not get home dir: %v", err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "tilde slash expands to home",
			path: "~/Documents/notes.txt",
			want: filepath.Join(home, "Documents/notes.txt"),
		},
		{
			name: "absolute path unchanged",
			path: "/usr/local/bin/foo",
			want: "/usr/local/bin/foo",
		},
		{
			name: "relative path unchanged",
			path: "some/relative/path",
			want: "some/relative/path",
		},
		{
			name: "empty path unchanged",
			path: "",
			want: "",
		},
		{
			name: "bare tilde unchanged",
			path: "~",
			want: "~",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandHome(tt.path)
			if got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
