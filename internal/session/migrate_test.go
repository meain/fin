package session

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	t2 "github.com/meain/fin/internal/types"
)

// writeLegacyTestSession writes a session file using the pre-migration
// filename format ("YYYYMMDD-HHMMSS_<uuid>[_<name>].jsonl"), for Migrate tests.
func writeLegacyTestSession(t *testing.T, dir string, h sessionHeader, legacyName string) string {
	t.Helper()
	filename := h.StartedAt.Format("20060102-150405") + "_" + h.ID
	if legacyName != "" {
		filename += "_" + legacyName
	}
	filename += ".jsonl"
	path := filepath.Join(dir, filename)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(h); err != nil {
		t.Fatalf("writeLegacyTestSession encode header: %v", err)
	}
	if err := enc.Encode(t2.Message{Role: t2.RoleUser, Content: "test", Timestamp: h.StartedAt}); err != nil {
		t.Fatalf("writeLegacyTestSession encode msg: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatalf("writeLegacyTestSession: %v", err)
	}
	return path
}

func TestMigrate_RenamesLegacyFile(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	started := time.Date(2026, 7, 17, 15, 4, 5, 0, time.UTC)
	writeLegacyTestSession(t, sessDir, sessionHeader{
		ID:        "aaaa1111-0000-0000-0000-000000000000",
		Title:     "legacy session",
		Model:     "test/model",
		Repo:      "fin", // already set — Migrate shouldn't need to backfill it
		Name:      "mysession",
		Temp:      true,
		StartedAt: started,
	}, "mysession_temp")

	result, err := Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if result.Renamed != 1 || result.Skipped != 0 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	es, err := os.ReadDir(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 {
		t.Fatalf("expected 1 file, got %d", len(es))
	}
	f, ok := parseFilename(es[0].Name())
	if !ok {
		t.Fatalf("renamed file %q not in current format", es[0].Name())
	}
	if f.uuid != "aaaa1111-0000-0000-0000-000000000000" || f.repo != "fin" || f.name != "mysession" || !f.temp {
		t.Errorf("unexpected parsed fields: %+v", f)
	}

	// Content must be untouched — still loadable by ID.
	sess, err := LoadByID("aaaa1111-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("LoadByID after migrate: %v", err)
	}
	if sess.Title != "legacy session" {
		t.Errorf("expected content preserved, got title %q", sess.Title)
	}
}

func TestMigrate_SkipsAlreadyCurrentFormat(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	writeTestSessionWithRepo(t, sessDir, "bbbb2222-0000-0000-0000-000000000000", time.Hour, "fin", nil)

	result, err := Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if result.Renamed != 0 || result.Skipped != 1 {
		t.Fatalf("expected 0 renamed / 1 skipped, got %+v", result)
	}
}

// TestMigrate_RootCwdDoesNotEmbedSlash guards against a session whose Cwd is
// "/" (filesystem root, no VCS) producing a repo of "/" — filepath.Base("/")
// is "/" itself, which would corrupt the filename if embedded verbatim.
func TestMigrate_RootCwdDoesNotEmbedSlash(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	started := time.Date(2026, 7, 7, 9, 46, 10, 0, time.UTC)
	writeLegacyTestSession(t, sessDir, sessionHeader{
		ID:        "cccc3333-0000-0000-0000-000000000000",
		Title:     "root cwd session",
		Model:     "test/model",
		Cwd:       "/",
		StartedAt: started,
	}, "")

	result, err := Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if result.Renamed != 1 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	es, err := os.ReadDir(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 {
		t.Fatalf("expected 1 file, got %d", len(es))
	}
	if filepath.Dir(filepath.Join(sessDir, es[0].Name())) != sessDir {
		t.Fatalf("renamed file escaped the sessions dir: %q", es[0].Name())
	}
	f, ok := parseFilename(es[0].Name())
	if !ok {
		t.Fatalf("renamed file %q not in current format", es[0].Name())
	}
	if f.repo != "" {
		t.Errorf("expected empty repo for root cwd, got %q", f.repo)
	}
}

// TestMigrate_RepairsHeaderRepoMismatch guards against the exact bug found
// when migrating real data: a file already in the current filename format,
// with a repo baked into the filename (e.g. by an earlier migration pass
// that renamed files without also updating their header), but whose header
// Repo field is still empty. Migrate must backfill the header to match the
// filename, in place, without renaming.
func TestMigrate_RepairsHeaderRepoMismatch(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	started := time.Date(2026, 7, 17, 15, 4, 5, 0, time.UTC)
	filename := buildFilename(started.Format("20060102-150405"), "dddd4444-0000-0000-0000-000000000000", "fin", "", false)
	path := filepath.Join(sessDir, filename)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(sessionHeader{
		ID:        "dddd4444-0000-0000-0000-000000000000",
		Title:     "mismatched header",
		Model:     "test/model",
		StartedAt: started,
		// Repo intentionally left unset, even though the filename has "fin".
	}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(t2.Message{Role: t2.RoleUser, Content: "test", Timestamp: started}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if result.Repaired != 1 || result.Renamed != 0 || result.Skipped != 0 || len(result.Errors) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}

	// Filename must be unchanged (already current format).
	es, err := os.ReadDir(sessDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != 1 || es[0].Name() != filename {
		t.Fatalf("expected filename unchanged (%q), got dir listing %v", filename, es)
	}

	sess, err := LoadByID("dddd4444-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("LoadByID after migrate: %v", err)
	}
	if sess.Repo != "fin" {
		t.Errorf("expected header repo backfilled to %q, got %q", "fin", sess.Repo)
	}
	if sess.Title != "mismatched header" {
		t.Errorf("expected message content preserved, got title %q", sess.Title)
	}

	// Running Migrate again must be a no-op (idempotent).
	result2, err := Migrate()
	if err != nil {
		t.Fatal(err)
	}
	if result2.Skipped != 1 || result2.Renamed != 0 || result2.Repaired != 0 {
		t.Fatalf("expected second run to be a no-op, got %+v", result2)
	}
}
