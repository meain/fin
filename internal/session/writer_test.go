package session

import (
	"fmt"
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
