// Package jsonui is a machine-readable implementation of agent.UIWriter.
// It emits one JSON object per line to stdout (newline-delimited JSON) and,
// for tool approvals, blocks reading a single JSON line from stdin. It is the
// contract a GUI frontend (e.g. the SwiftUI fin-ui app) speaks: no ANSI, no
// terminal assumptions, everything is structured events.
//
// Event stream (stdout, one object per line, discriminated by "t"):
//
//	{"t":"session","resumed":bool,"label":str,"started_at":rfc3339}
//	{"t":"text","text":str}                      // streamed assistant delta
//	{"t":"end"}                                  // end of an assistant text block
//	{"t":"tool_start","idx":n,"total":n,"name":str,"args":{...}}
//	{"t":"tool_output","idx":n,"line":str,"total":n}
//	{"t":"tool_done","idx":n,"name":str,"args":{...},"result":str,"error":str}
//	{"t":"approval","name":str,"args":{...}}     // then blocks on stdin
//	{"t":"info","text":str}
//	{"t":"error","text":str}
//	{"t":"retry","attempt":n,"max":n,"delay_ms":n,"error":str}
//
// Approval replies (stdin, one object per approval request):
//
//	{"approve":true}   or   {"approve":false}
//
// On stdin EOF or a malformed reply the request is denied (fail safe).
package jsonui

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"

	"github.com/meain/fin/internal/agent"
	t "github.com/meain/fin/internal/types"
)

// UI implements agent.UIWriter by streaming JSON events.
type UI struct {
	mu  sync.Mutex // serializes writes to stdout (tools run in parallel)
	enc *json.Encoder
	in  *bufio.Reader

	approvalMu sync.Mutex // serializes approval round-trips
}

// New constructs a JSON UI bound to stdout/stdin.
func New() *UI {
	enc := json.NewEncoder(os.Stdout)
	return &UI{
		enc: enc,
		in:  bufio.NewReader(os.Stdin),
	}
}

// Close is a no-op; present so callers can treat it like the terminal UI.
func (u *UI) Close() {}

// emit writes a single event object as one JSON line.
func (u *UI) emit(ev map[string]any) {
	u.mu.Lock()
	defer u.mu.Unlock()
	_ = u.enc.Encode(ev) // Encoder appends the trailing newline
}

func (u *UI) StreamText(text string) {
	if text == "" {
		return
	}
	u.emit(map[string]any{"t": "text", "text": text})
}

func (u *UI) EndStream() {
	u.emit(map[string]any{"t": "end"})
}

// ToolCallProgress (partial argument streaming) is intentionally dropped: the
// GUI renders tools once they fully start, and progress spam bloats the stream.
func (u *UI) ToolCallProgress(name, argsSoFar string) {}

func (u *UI) ToolStart(idx, total int, name string, args map[string]any) {
	u.emit(map[string]any{
		"t": "tool_start", "idx": idx, "total": total,
		"name": name, "args": args,
	})
}

func (u *UI) ToolDone(idx int, name string, args map[string]any, result t.ToolResult, err error) {
	ev := map[string]any{
		"t": "tool_done", "idx": idx, "name": name,
		"args": args, "result": result.Content,
	}
	if err != nil {
		ev["error"] = err.Error()
	}
	u.emit(ev)
}

func (u *UI) ToolOutput(idx int, line string, total int) {
	u.emit(map[string]any{"t": "tool_output", "idx": idx, "line": line, "total": total})
}

func (u *UI) ToolCallStart(name string, args map[string]any) {
	u.emit(map[string]any{"t": "tool_start", "idx": -1, "total": 1, "name": name, "args": args})
}

// ToolApprovalPrompt emits an approval request and blocks until the frontend
// writes a decision on stdin. Denies on EOF or malformed input.
func (u *UI) ToolApprovalPrompt(name string, args map[string]any) bool {
	u.approvalMu.Lock()
	defer u.approvalMu.Unlock()

	u.emit(map[string]any{"t": "approval", "name": name, "args": args})

	line, err := u.in.ReadBytes('\n')
	if len(line) == 0 && err != nil {
		return false
	}
	var reply struct {
		Approve bool `json:"approve"`
	}
	if err := json.Unmarshal(line, &reply); err != nil {
		return false
	}
	return reply.Approve
}

func (u *UI) Info(msg string) {
	u.emit(map[string]any{"t": "info", "text": msg})
}

func (u *UI) Error(msg string) {
	u.emit(map[string]any{"t": "error", "text": msg})
}

func (u *UI) SessionInfo(data agent.SessionInfoData) {
	u.emit(map[string]any{
		"t": "session", "resumed": data.Resumed,
		"label": data.Label, "started_at": data.StartedAt.Format(time.RFC3339),
	})
}

func (u *UI) Retry(data agent.RetryData) {
	ev := map[string]any{
		"t": "retry", "attempt": data.Attempt, "max": data.MaxRetries,
		"delay_ms": data.Delay.Milliseconds(),
	}
	if data.Err != nil {
		ev["error"] = data.Err.Error()
	}
	u.emit(ev)
}

// Debug events are dropped from the JSON stream (GUI has no use for them).
func (u *UI) Debug(ev agent.DebugEvent) {}
