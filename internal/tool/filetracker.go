package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileTracker records the mtime of files the model has read, so write/edit
// tools can reject modifications to files the model hasn't seen or that have
// changed on disk since the last read.
type FileTracker struct {
	mu    sync.Mutex
	reads map[string]time.Time // abs path → mtime at last read
}

func newFileTracker() *FileTracker {
	return &FileTracker{reads: make(map[string]time.Time)}
}

// RecordRead stores the current mtime of path. Called after a successful read.
func (ft *FileTracker) RecordRead(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	info, err := os.Stat(abs)
	if err != nil {
		return
	}
	ft.mu.Lock()
	ft.reads[abs] = info.ModTime()
	ft.mu.Unlock()
}

// CheckWrite returns an error if path exists but the model hasn't read it, or
// if the file changed on disk since the last read. Returns nil for new files.
func (ft *FileTracker) CheckWrite(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	info, err := os.Stat(abs)
	if os.IsNotExist(err) {
		return nil // new file — no read required
	}
	if err != nil {
		return nil
	}

	ft.mu.Lock()
	readAt, ok := ft.reads[abs]
	ft.mu.Unlock()

	if !ok {
		return fmt.Errorf("you must read %s before editing it — call the read tool on this path first, then retry the edit", path)
	}
	if !info.ModTime().Equal(readAt) {
		return fmt.Errorf("file %s has changed on disk since you last read it — call the read tool on this path again, then retry the edit", path)
	}
	return nil
}
