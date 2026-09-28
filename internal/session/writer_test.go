package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	t2 "github.com/meain/fin/internal/types"
)

// Concurrent SetTitle+Save alongside the main save loop must never trip the
// mtime conflict guard or lose messages.
func TestWriter_ConcurrentSaveNoConflict(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{
		id:       "concurrent-id",
		started:  time.Now(),
		filepath: filepath.Join(dir, "s.jsonl"),
	}

	var msgs []t2.Message
	for i := 0; i < 200; i++ {
		msgs = append(msgs, t2.Message{Role: t2.RoleUser, Content: fmt.Sprintf("m%d", i)})
	}

	var wg sync.WaitGroup
	errs := make(chan error, 400)
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= len(msgs); i++ {
			if err := w.Save(msgs[:i]); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			w.SetTitle(fmt.Sprintf("title %d", i))
			if err := w.Save(msgs[:1]); err != nil {
				errs <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("Save: %v", err)
	}

	if err := w.Save(msgs); err != nil {
		t.Fatalf("final Save: %v", err)
	}
	sess, err := readFile(w.filepath)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != len(msgs) {
		t.Errorf("got %d messages on disk, want %d", len(sess.Messages), len(msgs))
	}
}

// An empty session file must never be picked up by -c or cause
// WriterForExisting to target an unrelated session.
func TestEmptySessionFileIsSkipped(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	realID := "aaaa1111-0000-0000-0000-000000000000"
	writeTestSession(t, sessDir, realID, 2*time.Hour, 5)
	empty := filepath.Join(sessDir, buildFilename("20990101-000000", "bbbb2222-0000-0000-0000-000000000000", "", "", false))
	if err := os.WriteFile(empty, nil, 0644); err != nil {
		t.Fatal(err)
	}

	if _, err := readFile(empty); err == nil {
		t.Error("readFile on an empty file should fail")
	}

	sess, err := LoadLast()
	if err != nil {
		t.Fatalf("LoadLast: %v", err)
	}
	if sess.ID != realID {
		t.Fatalf("LoadLast picked %q, want %q", sess.ID, realID)
	}

	w := WriterForExisting(&Session{})
	if w.ID() == "" {
		t.Error("WriterForExisting with empty ID should generate a new ID")
	}
	if err := w.Save([]t2.Message{{Role: t2.RoleUser, Content: "new"}}); err != nil {
		t.Fatal(err)
	}
	orig, err := LoadByID(realID)
	if err != nil {
		t.Fatal(err)
	}
	if len(orig.Messages) != 5 {
		t.Errorf("real session has %d messages after unrelated save, want 5", len(orig.Messages))
	}
}

// A partial ".jsonl.tmp" left behind by a failed rewrite must not shadow the
// real session file.
func TestLoadByName_IgnoresLeftoverTmp(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, ".local", "share", "fin", "sessions")
	if err := os.MkdirAll(sessDir, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	w := NewWriter("", "test/model", "proj", false, nil)
	msgs := []t2.Message{
		{Role: t2.RoleUser, Content: "a"},
		{Role: t2.RoleAssistant, Content: "b"},
		{Role: t2.RoleUser, Content: "c"},
		{Role: t2.RoleAssistant, Content: "d"},
	}
	if err := w.Save(msgs); err != nil {
		t.Fatal(err)
	}
	partial := `{"id":"` + w.ID() + `","name":"proj"}` + "\n" + `{"role":"user","content":"a"}` + "\n"
	if err := os.WriteFile(w.filepath+".tmp", []byte(partial), 0644); err != nil {
		t.Fatal(err)
	}

	sess, err := LoadByName("proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) != 4 {
		t.Errorf("LoadByName loaded %d messages, want 4 (picked the .tmp file?)", len(sess.Messages))
	}
}
