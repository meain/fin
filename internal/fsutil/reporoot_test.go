package fsutil

import "testing"

// TestRepoName_RootIsNotAValidRepoName guards against filepath.Base("/")
// returning "/" itself: RepoRoot("/") falls back to "/" (no VCS at the
// filesystem root), and embedding a literal "/" in a filename would corrupt
// it (e.g. by being interpreted as a path separator on rename/write).
func TestRepoName_RootIsNotAValidRepoName(t *testing.T) {
	if got := RepoName("/"); got != "" {
		t.Errorf(`RepoName("/") = %q, want ""`, got)
	}
}

func TestRepoName_RegularDir(t *testing.T) {
	if got := RepoName("."); got == "/" || got == "." {
		t.Errorf("RepoName(%q) = %q, degenerate value leaked through", ".", got)
	}
}
