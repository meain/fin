package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFIFOReader_FramingAndLargeMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q.fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, stop := startFIFOReader(ctx, path)

	big := strings.Repeat("x", 200<<10) // well past bufio.Scanner's 64 KiB limit
	multi := "review this\n\nline two\nline three"
	send := func(b []byte) {
		t.Helper()
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(b); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	recv := func() string {
		t.Helper()
		select {
		case m := <-ch:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for queued message")
			return ""
		}
	}

	send(encodeQueueMessage(big))
	if got := recv(); got != big {
		t.Errorf("big message: got %d bytes, want %d", len(got), len(big))
	}
	send(encodeQueueMessage(multi))
	if got := recv(); got != multi {
		t.Errorf("multi-line message: got %q, want %q", got, multi)
	}
	send([]byte("plain line\n"))
	if got := recv(); got != "plain line" {
		t.Errorf("raw line: got %q", got)
	}

	// Messages written before stop are still delivered, then the channel closes.
	send(encodeQueueMessage("late"))
	stop()
	var rest []string
	for m := range ch {
		rest = append(rest, m)
	}
	if len(rest) != 1 || rest[0] != "late" {
		t.Errorf("after stop: got %q, want [late]", rest)
	}
}
