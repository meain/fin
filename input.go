package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/term"
)

// inputMux multiplexes stdin between term.Terminal (for ReadLine) and a
// capture buffer (during agent execution). A background goroutine pumps
// raw bytes from stdin into a channel. In normal mode, Read() pulls from
// that channel. In capture mode, a separate goroutine drains the channel
// into a buffer and watches for Esc to cancel the agent. When capture
// stops, buffered bytes are replayed so ReadLine sees them.
type inputMux struct {
	ch chan byte // raw stdin bytes

	mu     sync.Mutex
	inject bytes.Buffer // bytes to serve before reading channel

	// capture state
	capMu   sync.Mutex
	capStop chan struct{} // closed to signal capture goroutine to exit
	capDone chan struct{} // closed when capture goroutine exits
	capBuf  bytes.Buffer
}

func newInputMux() *inputMux {
	m := &inputMux{
		ch: make(chan byte, 4096),
	}
	go m.pump()
	return m
}

func (m *inputMux) pump() {
	buf := make([]byte, 256)
	for {
		n, err := os.Stdin.Read(buf)
		for i := 0; i < n; i++ {
			m.ch <- buf[i]
		}
		if err != nil {
			close(m.ch)
			return
		}
	}
}

// Read implements io.Reader. term.Terminal uses this.
func (m *inputMux) Read(p []byte) (int, error) {
	m.mu.Lock()
	if m.inject.Len() > 0 {
		n, _ := m.inject.Read(p)
		m.mu.Unlock()
		return n, nil
	}
	m.mu.Unlock()

	// Block for at least one byte
	b, ok := <-m.ch
	if !ok {
		return 0, io.EOF
	}
	p[0] = b
	n := 1

	// Non-blocking drain of any additional available bytes
	for n < len(p) {
		select {
		case b, ok = <-m.ch:
			if !ok {
				return n, nil
			}
			p[n] = b
			n++
		default:
			return n, nil
		}
	}
	return n, nil
}

// StartCapture begins capturing input. Typed bytes are buffered.
// Standalone Esc (not part of an escape sequence) calls cancelFn.
// Call StopCapture to end capture and replay buffered bytes.
func (m *inputMux) StartCapture(cancelFn func()) {
	m.capMu.Lock()
	m.capStop = make(chan struct{})
	m.capDone = make(chan struct{})
	m.capBuf.Reset()
	m.capMu.Unlock()

	go m.captureLoop(cancelFn)
}

func (m *inputMux) captureLoop(cancelFn func()) {
	defer close(m.capDone)

	for {
		select {
		case <-m.capStop:
			return
		case b, ok := <-m.ch:
			if !ok {
				return
			}
			if b == 0x03 { // Ctrl+C
				cancelFn()
				return
			}
			if b == 0x1b {
				// Distinguish standalone Esc from escape sequences (Esc [ ...)
				timer := time.NewTimer(50 * time.Millisecond)
				select {
				case <-m.capStop:
					timer.Stop()
					return
				case next, ok := <-m.ch:
					timer.Stop()
					if !ok {
						cancelFn()
						return
					}
					if next == '[' {
						// Escape sequence — buffer both bytes
						m.capBuf.WriteByte(b)
						m.capBuf.WriteByte(next)
					} else {
						// Standalone Esc followed by another char
						cancelFn()
						m.capBuf.WriteByte(next)
						return
					}
				case <-timer.C:
					// Standalone Esc (nothing followed within 50ms)
					cancelFn()
					return
				}
			} else {
				m.capBuf.WriteByte(b)
			}
		}
	}
}

// StopCapture ends capture mode and injects buffered bytes so the next
// ReadLine sees them as pre-typed input.
func (m *inputMux) StopCapture() {
	m.capMu.Lock()
	if m.capStop != nil {
		close(m.capStop)
		<-m.capDone
	}
	data := m.capBuf.Bytes()
	m.capMu.Unlock()

	if len(data) > 0 {
		m.mu.Lock()
		m.inject.Write(data)
		m.mu.Unlock()
	}
}

// Terminal wraps x/term to provide line editing and safe concurrent output.
type Terminal struct {
	inner    *term.Terminal
	oldState *term.State
	mux      *inputMux
	rawFd    int
}

func NewTerminal() (*Terminal, error) {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("failed to set raw mode: %w", err)
	}

	mux := newInputMux()

	screen := struct {
		io.Reader
		io.Writer
	}{mux, os.Stderr}

	inner := term.NewTerminal(screen, "")

	return &Terminal{
		inner:    inner,
		oldState: oldState,
		mux:      mux,
		rawFd:    fd,
	}, nil
}

// Restore puts the terminal back to its original state.
func (t *Terminal) Restore() {
	term.Restore(t.rawFd, t.oldState)
}

// ReadLine shows a prompt and reads a line with editing support.
func (t *Terminal) ReadLine(prompt string) (string, error) {
	t.inner.SetPrompt(prompt)
	return t.inner.ReadLine()
}

// Write outputs text while preserving any in-progress input line.
func (t *Terminal) Write(p []byte) (int, error) {
	return t.inner.Write(p)
}

// WriteString is a convenience wrapper.
func (t *Terminal) WriteString(s string) {
	t.Write([]byte(s))
}

// StartCapture begins capturing input during agent execution.
func (t *Terminal) StartCapture(cancelFn func()) {
	t.mux.StartCapture(cancelFn)
}

// StopCapture ends capture and replays buffered input.
func (t *Terminal) StopCapture() {
	t.mux.StopCapture()
}
