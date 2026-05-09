package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	t "github.com/meain/fin/internal/types"
	"golang.org/x/term"
)

var stdout = os.Stdout

// ANSI escape codes — variables so they can be cleared by disableColors().
var (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
	dimFg   = "\033[38;5;243m" // muted gray for debug lines
)

// disableColors clears all ANSI escape code variables.
func disableColors() {
	reset = ""
	bold = ""
	dim = ""
	red = ""
	green = ""
	yellow = ""
	magenta = ""
	dimFg = ""
}

// OutputMode controls how much the UI displays.
type OutputMode int

const (
	OutputDefault OutputMode = iota // tool names + streamed text + tool detail
	OutputDebug                     // default + token usage
	OutputQuiet                     // only final response text (stdout)
	OutputSilent                    // no output at all (for subagents)
)

func parseOutputMode(s string) OutputMode {
	switch s {
	case "quiet":
		return OutputQuiet
	case "debug":
		return OutputDebug
	default:
		return OutputDefault
	}
}

// UIEventKind identifies the type of UI event.
type UIEventKind int

const (
	uiStreamText UIEventKind = iota
	uiEndStream
	uiToolProgress // streaming tool-call argument progress
	uiToolStart    // tool about to execute (shows status line)
	uiToolDone     // tool finished (updates its status line)
	uiToolApproval // interactive approval prompt
	uiToolOutput   // streaming output line count update
	uiInfo
	uiError
	uiSessionInfo // session resumed/created (shown in default + debug)
	uiRetry       // retry attempt (shown in default + debug)
	uiDebug // only shown in debug mode
)

// DebugEvent is implemented by all structured debug payloads.
type DebugEvent interface {
	debugEvent()
}

type DebugStartup struct {
	Model       string
	Skills      []string
	SessionID   string
	PromptChars int
}

type DebugTurnStart struct {
	Turn     int
	MaxTurns int
	Messages int
}

type DebugTurnDone struct {
	Usage   *t.Usage
	TTFT    time.Duration
	Elapsed time.Duration
}

type DebugToolArgs struct {
	ToolName string
	ToolArgs map[string]any
}

type DebugMessageCount struct {
	Messages int
}

type DebugSummary struct {
	Usage    *t.Usage
	Messages int
}

func (DebugStartup) debugEvent()      {}
func (DebugTurnStart) debugEvent()    {}
func (DebugTurnDone) debugEvent()     {}
func (DebugToolArgs) debugEvent()     {}
func (DebugMessageCount) debugEvent() {}
func (DebugSummary) debugEvent()      {}

// SessionInfoData carries session status for the UI to render.
type SessionInfoData struct {
	Resumed   bool
	Label     string    // session ID or name
	StartedAt time.Time // only relevant for resumed sessions
}

// RetryData carries structured retry information for the UI to render.
type RetryData struct {
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Err        error
}

// UIEvent is a message sent to the UI goroutine.
type UIEvent struct {
	Kind   UIEventKind
	Text   string
	Name   string
	Args   map[string]any
	Result t.ToolResult
	Err    error
	Index  int        // tool index in parallel batch
	Total  int        // total tools in batch
	Reply  chan bool   // for approval response
	Session *SessionInfoData // structured session info (uiSessionInfo only)
	Retry   *RetryData      // structured retry payload (uiRetry only)
	Debug   DebugEvent      // structured debug payload (uiDebug only)
}

// toolLineState tracks one tool in a parallel batch.
type toolLineState struct {
	name    string
	args    map[string]any
	running bool
	result  t.ToolResult
	err     error
	start   time.Time
	lines   int // streaming line count (updated during execution)
}

// UI handles terminal output via a single goroutine that processes events.
type UI struct {
	term   *Terminal
	mode   OutputMode
	piped  bool // stdout is not a terminal; only stream response text to stdout
	events chan UIEvent
	done   chan struct{}

	// State managed exclusively by the run() goroutine:
	wroteText         bool
	hasProgress       bool
	lastProgressLines int
	toolLines         []toolLineState
}

func NewUI(t *Terminal, mode OutputMode, piped bool) *UI {
	if mode == OutputSilent {
		return &UI{mode: mode}
	}
	u := &UI{
		term:   t,
		mode:   mode,
		piped:  piped,
		events: make(chan UIEvent, 64),
		done:   make(chan struct{}),
	}
	go u.run()
	return u
}

// Close shuts down the UI goroutine. Safe to call multiple times.
func (u *UI) Close() {
	if u.events == nil {
		return
	}
	close(u.events)
	<-u.done
	u.events = nil
}

func (u *UI) send(ev UIEvent) {
	if u.events == nil {
		return
	}
	u.events <- ev
}

// sendSync sends an event and waits for a bool reply.
func (u *UI) sendSync(ev UIEvent) bool {
	if u.events == nil {
		return false
	}
	u.events <- ev
	return <-ev.Reply
}

// --- Public API (sends events) ---

func (u *UI) StreamText(text string) {
	if u.mode == OutputSilent {
		return
	}
	u.send(UIEvent{Kind: uiStreamText, Text: text})
}

func (u *UI) EndStream() {
	u.send(UIEvent{Kind: uiEndStream})
}

func (u *UI) ToolCallProgress(name, argsSoFar string) {
	if u.mode == OutputSilent {
		return
	}
	u.send(UIEvent{Kind: uiToolProgress, Name: name, Text: argsSoFar})
}

// ToolStart registers a tool as running in the parallel batch.
func (u *UI) ToolStart(idx, total int, name string, args map[string]any) {
	u.send(UIEvent{Kind: uiToolStart, Index: idx, Total: total, Name: name, Args: args})
}

// ToolDone marks a tool as completed and updates its status line.
func (u *UI) ToolDone(idx int, name string, args map[string]any, result t.ToolResult, err error) {
	u.send(UIEvent{Kind: uiToolDone, Index: idx, Name: name, Args: args, Result: result, Err: err})
}

// ToolOutput updates a running tool's streaming line count.
func (u *UI) ToolOutput(idx, lines int) {
	u.send(UIEvent{Kind: uiToolOutput, Index: idx, Total: lines})
}

// ToolCallStart shows a tool being invoked (used for approval display).
func (u *UI) ToolCallStart(name string, args map[string]any) {
	u.send(UIEvent{Kind: uiToolStart, Index: -1, Total: 1, Name: name, Args: args})
}

// ToolApprovalPrompt asks the user to approve a tool call. Blocks until answered.
func (u *UI) ToolApprovalPrompt(name string, args map[string]any) bool {
	if u.events == nil {
		return false
	}
	reply := make(chan bool, 1)
	return u.sendSync(UIEvent{Kind: uiToolApproval, Name: name, Args: args, Reply: reply})
}

func (u *UI) Info(msg string) {
	u.send(UIEvent{Kind: uiInfo, Text: msg})
}

func (u *UI) Error(msg string) {
	u.send(UIEvent{Kind: uiError, Text: msg})
}

func (u *UI) SessionInfo(data SessionInfoData) {
	u.send(UIEvent{Kind: uiSessionInfo, Session: &data})
}

func (u *UI) Retry(data RetryData) {
	u.send(UIEvent{Kind: uiRetry, Retry: &data})
}

func (u *UI) Debug(ev DebugEvent) {
	if u.mode != OutputDebug {
		return
	}
	u.send(UIEvent{Kind: uiDebug, Debug: ev})
}

// --- Event loop ---

func (u *UI) run() {
	defer close(u.done)

	var ticker *time.Ticker
	var tickCh <-chan time.Time

	for {
		select {
		case ev, ok := <-u.events:
			if !ok {
				if ticker != nil {
					ticker.Stop()
				}
				return
			}
			u.handleEvent(ev)

			// Start/stop ticker based on whether we have running tools.
			hasRunning := u.hasRunningTools()
			if hasRunning && ticker == nil {
				ticker = time.NewTicker(200 * time.Millisecond)
				tickCh = ticker.C
			} else if !hasRunning && ticker != nil {
				ticker.Stop()
				ticker = nil
				tickCh = nil
			}

		case <-tickCh:
			u.refreshToolLines()
		}
	}
}

func (u *UI) hasRunningTools() bool {
	for _, tl := range u.toolLines {
		if tl.running {
			return true
		}
	}
	return false
}

func (u *UI) handleEvent(ev UIEvent) {
	switch ev.Kind {
	case uiStreamText:
		if u.mode == OutputQuiet || u.piped {
			fmt.Fprint(os.Stdout, ev.Text)
			return
		}
		if ev.Text != "" {
			u.wroteText = true
		}
		u.write(ev.Text)

	case uiEndStream:
		u.ensureNewline()

	case uiToolProgress:
		u.handleToolProgress(ev.Name, ev.Text)

	case uiToolStart:
		u.handleToolStart(ev)

	case uiToolDone:
		u.handleToolDone(ev)

	case uiToolOutput:
		if ev.Index >= 0 && ev.Index < len(u.toolLines) && u.toolLines[ev.Index].running {
			u.toolLines[ev.Index].lines = ev.Total
			u.updateToolLine(ev.Index)
		}

	case uiToolApproval:
		u.handleToolApproval(ev)

	case uiInfo:
		if u.piped || (u.mode != OutputDefault && u.mode != OutputDebug) {
			return
		}
		u.write(fmt.Sprintf("%s%s%s\n", dim, ev.Text, reset))

	case uiError:
		u.write(fmt.Sprintf("%s%serror: %s%s\n", bold, red, ev.Text, reset))

	case uiSessionInfo:
		if u.mode != OutputDebug {
			return
		}
		if ev.Session != nil {
			s := ev.Session
			if s.Resumed {
				u.write(fmt.Sprintf("%sresumed session %s (%s)%s\n",
					dim, s.Label, s.StartedAt.Format("2006-01-02 15:04"), reset))
			} else {
				u.write(fmt.Sprintf("%snew session [%s]%s\n", dim, s.Label, reset))
			}
		}

	case uiRetry:
		if ev.Retry != nil {
			r := ev.Retry
			u.write(fmt.Sprintf("%s%sretrying in %s (attempt %d/%d: %s)%s\n",
				bold, yellow, formatElapsed(r.Delay), r.Attempt, r.MaxRetries, r.Err, reset))
		}

	case uiDebug:
		u.renderDebug(ev.Debug)
	}
}

// --- Debug rendering ---

func (u *UI) renderDebug(ev DebugEvent) {
	if ev == nil {
		return
	}
	switch d := ev.(type) {
	case DebugStartup:
		parts := []string{d.Model}
		sid := d.SessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		parts = append(parts, sid)
		parts = append(parts, fmt.Sprintf("%d skills", len(d.Skills)))
		if d.PromptChars > 0 {
			parts = append(parts, fmt.Sprintf("%d char prompt", d.PromptChars))
		}
		u.debugLine(strings.Join(parts, " | "))
	case DebugTurnStart:
		u.debugLine(fmt.Sprintf("turn %d/%d | %d messages", d.Turn, d.MaxTurns, d.Messages))
	case DebugTurnDone:
		s := formatUsage(d.Usage)
		if d.TTFT > time.Millisecond {
			s += fmt.Sprintf(" | ttft: %s", formatElapsed(d.TTFT))
		}
		if d.Elapsed > 0 {
			s += fmt.Sprintf(" | %s", formatElapsed(d.Elapsed))
		}
		if s != "" {
			u.debugLine(s)
		}
	case DebugToolArgs:
		argsJSON, _ := json.Marshal(d.ToolArgs)
		u.debugLine(fmt.Sprintf("  %s %s", d.ToolName, string(argsJSON)))
	case DebugMessageCount:
		u.debugLine(fmt.Sprintf("%d messages", d.Messages))
	case DebugSummary:
		s := "total: " + formatUsage(d.Usage)
		s += fmt.Sprintf(" | %d messages", d.Messages)
		u.debugLine(s)
	}
}

func formatUsage(u *t.Usage) string {
	if u == nil {
		return ""
	}
	totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	s := fmt.Sprintf("%d in, %d out", totalIn, u.OutputTokens)
	if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
		hitPct := 0.0
		if totalIn > 0 {
			hitPct = float64(u.CacheReadInputTokens) / float64(totalIn) * 100
		}
		s += fmt.Sprintf(" | cache: %.0f%%", hitPct)
	}
	return s
}

// --- Tool progress (streaming args) ---

func (u *UI) handleToolProgress(name, argsSoFar string) {
	if u.mode == OutputQuiet || u.piped {
		return
	}

	lines := strings.Count(argsSoFar, "\\n") + strings.Count(argsSoFar, "\n")
	if lines == 0 {
		return
	}
	if lines == u.lastProgressLines {
		return
	}
	u.lastProgressLines = lines

	if !u.hasProgress {
		u.ensureNewline()
	}

	fmt.Fprintf(stdout, "\r\033[2K%s%s%s %s(%d lines)%s", yellow, name, reset, dim, lines, reset)
	stdout.Sync()
	u.hasProgress = true
}

// --- Parallel tool status lines ---

func (u *UI) handleToolStart(ev UIEvent) {
	if u.mode == OutputQuiet || u.piped {
		return
	}

	// Index -1 means this is a pre-approval display (old ToolCallStart), not a batch.
	if ev.Index == -1 {
		u.renderToolCallPreApproval(ev.Name, ev.Args)
		return
	}

	// First tool in batch: clear progress, allocate lines.
	if ev.Index == 0 {
		if u.hasProgress {
			fmt.Fprint(stdout, "\033[2K\r")
			u.hasProgress = false
			u.lastProgressLines = 0
		} else {
			u.ensureNewline()
		}
		u.toolLines = make([]toolLineState, ev.Total)
	}

	u.toolLines[ev.Index] = toolLineState{
		name:    ev.Name,
		args:    ev.Args,
		running: true,
		start:   time.Now(),
	}

	// Print the initial status line.
	suffix := fmt.Sprintf(" %s…%s", dim, reset)
	suffixVis := visibleLen(suffix)
	maxLabel := getTermWidth() - suffixVis
	label := truncateVisible(toolLabel(ev.Name, ev.Args), maxLabel)
	line := fmt.Sprintf("%s%s%s%s", bold, yellow, label, reset+suffix)
	u.write(line + "\n")
}

func (u *UI) handleToolDone(ev UIEvent) {
	if u.mode == OutputQuiet || u.piped {
		return
	}

	if ev.Index < 0 || ev.Index >= len(u.toolLines) {
		return
	}

	tl := &u.toolLines[ev.Index]
	tl.running = false
	tl.result = ev.Result
	tl.err = ev.Err

	// Update the line in-place.
	u.updateToolLine(ev.Index)

	// If all tools are done, clear the batch state.
	if !u.hasRunningTools() {
		u.toolLines = nil
	}
}

func (u *UI) updateToolLine(idx int) {
	if idx < 0 || idx >= len(u.toolLines) {
		return
	}

	// Calculate distance from cursor (cursor is after the last tool line).
	linesUp := len(u.toolLines) - idx
	if linesUp > 0 {
		fmt.Fprintf(stdout, "\033[%dA", linesUp)
	}
	fmt.Fprint(stdout, "\033[2K\r")

	tl := u.toolLines[idx]
	elapsed := time.Since(tl.start)
	label := toolLabel(tl.name, tl.args)

	elapsedStr := formatElapsed(elapsed)
	resultInfo := ""
	if tl.running && tl.lines > 0 {
		resultInfo = fmt.Sprintf("(%d lines) ", tl.lines)
	} else if !tl.running && tl.err == nil {
		lines := strings.Count(tl.result.Content, "\n")
		if lines > 0 {
			resultInfo = fmt.Sprintf("(%d lines) ", lines)
		}
	}

	// Build suffix (always shown) and determine space for label
	var suffix string
	labelColor := yellow
	if tl.err != nil {
		labelColor = red
		suffix = fmt.Sprintf(" %s(error) %s%s", dim, elapsedStr, reset)
	} else {
		suffix = fmt.Sprintf(" %s%s%s%s", dim, resultInfo, elapsedStr, reset)
	}

	// Truncate label to fit: width - suffix_visible - 1 (space after label)
	suffixVisible := visibleLen(suffix)
	maxLabel := getTermWidth() - suffixVisible - 1
	label = truncateVisible(label, maxLabel)

	fmt.Fprintf(stdout, "%s%s%s%s", bold, labelColor, label, reset+suffix)

	if linesUp > 0 {
		fmt.Fprintf(stdout, "\033[%dB\r", linesUp)
	}
	stdout.Sync()
}

func (u *UI) refreshToolLines() {
	for i, tl := range u.toolLines {
		if tl.running {
			u.updateToolLine(i)
		}
	}
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		ms := d.Milliseconds()
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// --- Pre-approval display (for tools that need confirmation) ---

func (u *UI) renderToolCallPreApproval(name string, args map[string]any) {
	if u.hasProgress {
		fmt.Fprint(stdout, "\033[2K\r")
		u.hasProgress = false
		u.lastProgressLines = 0
	} else {
		u.ensureNewline()
	}
	u.write(fmt.Sprintf("%s%s%s\n", yellow, toolLabel(name, args), reset))
}

// --- Approval prompt ---

func (u *UI) handleToolApproval(ev UIEvent) {
	var approved bool
	if u.term != nil {
		u.term.WriteString(fmt.Sprintf("%s%sallow %s? [y/N]%s ", bold, yellow, ev.Name, reset))
		line, err := u.term.ReadLine("")
		if err == nil {
			line = strings.TrimSpace(strings.ToLower(line))
			approved = line == "y" || line == "yes"
		}
	} else {
		u.write(fmt.Sprintf("%s%sallow %s? [y/N]%s ", bold, yellow, ev.Name, reset))
		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(strings.ToLower(input))
		approved = input == "y" || input == "yes"
	}
	ev.Reply <- approved
}

// --- Helpers ---

func (u *UI) write(s string) {
	if u.term != nil {
		u.term.WriteString(s)
	} else {
		fmt.Fprint(os.Stdout, s)
	}
}

func (u *UI) ensureNewline() {
	if u.hasProgress {
		fmt.Fprint(stdout, "\033[2K\r")
		u.hasProgress = false
		u.lastProgressLines = 0
	}
	if u.wroteText {
		u.write("\n")
		u.wroteText = false
	}
}

// debugLine writes a debug line with a │ prefix to distinguish from tool output.
func (u *UI) debugLine(text string) {
	u.ensureNewline()
	u.write(fmt.Sprintf("%s│ %s%s\n", dimFg, text, reset))
}

func getTermWidth() int {
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if w <= 0 {
		w = 80
	}
	return w - 1 // leave 1 col to prevent terminal line wrap
}

// visibleLen returns the number of visible (non-ANSI-escape) characters in s.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

// truncateVisible truncates s (which may contain ANSI codes) so the total
// visible width (including the trailing "…") does not exceed maxVisible.
func truncateVisible(s string, maxVisible int) string {
	if maxVisible <= 0 {
		return ""
	}

	// First pass: count visible chars to see if truncation is needed.
	visibleTotal := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			continue
		}
		visibleTotal++
	}
	if visibleTotal <= maxVisible {
		return s
	}

	// Need to truncate: keep maxVisible-1 visible chars + "…"
	cutoff := maxVisible - 1
	if cutoff < 0 {
		cutoff = 0
	}
	var out strings.Builder
	visible := 0
	inEsc = false
	for _, r := range s {
		if inEsc {
			out.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			out.WriteRune(r)
			continue
		}
		if visible >= cutoff {
			out.WriteString("…" + reset)
			return out.String()
		}
		out.WriteRune(r)
		visible++
	}
	return out.String()
}

// toolLabel returns a short description like "read agent.go" or "shell $ ls".
func toolLabel(name string, args map[string]any) string {
	switch name {
	case "shell":
		if cmd, ok := args["command"].(string); ok {
			cmd = strings.ReplaceAll(cmd, "\n", `\n`)
			return name + reset + " " + dim + "$ " + cmd
		}
	case "read":
		if path, ok := args["path"].(string); ok {
			offset, hasOffset := args["offset"].(float64)
			limit, hasLimit := args["limit"].(float64)
			if hasOffset || hasLimit {
				if hasOffset && hasLimit {
					return fmt.Sprintf("%s%s %s%s (%d:%d)", name, reset, dim, path, int(offset), int(offset)+int(limit))
				} else if hasOffset {
					return fmt.Sprintf("%s%s %s%s (%d:)", name, reset, dim, path, int(offset))
				} else {
					return fmt.Sprintf("%s%s %s%s (:%d)", name, reset, dim, path, int(limit))
				}
			}
			return name + reset + " " + dim + path
		}
	case "write":
		if path, ok := args["path"].(string); ok {
			return name + reset + " " + dim + path
		}
	case "edit":
		if path, ok := args["path"].(string); ok {
			old, _ := args["old_string"].(string)
			nw, _ := args["new_string"].(string)
			oldLines := strings.Count(old, "\n") + 1
			newLines := strings.Count(nw, "\n") + 1
			return fmt.Sprintf("%s%s %s%s (-%d +%d)", name, reset, dim, path, oldLines, newLines)
		}
	case "use_skill":
		if skill, ok := args["name"].(string); ok {
			return name + reset + " " + dim + skill
		}
	case "subagent":
		if task, ok := args["task"].(string); ok {
			display := task
			if len(display) > 60 {
				display = display[:60] + "…"
			}
			return name + reset + " " + dim + display
		}
	case "compact":
		return name
	}
	return name
}
