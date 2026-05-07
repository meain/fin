package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

var stderr = os.Stderr

// ANSI escape codes — variables so they can be cleared by disableColors().
var (
	reset   = "\033[0m"
	bold    = "\033[1m"
	dim     = "\033[2m"
	red     = "\033[31m"
	green   = "\033[32m"
	yellow  = "\033[33m"
	magenta = "\033[35m"
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
}

// OutputMode controls how much the UI displays.
type OutputMode int

const (
	OutputDefault OutputMode = iota // tool names + streamed text + tool detail
	OutputQuiet                     // only final response text (stdout)
	OutputSilent                    // no output at all (for subagents)
)

func parseOutputMode(s string) OutputMode {
	switch s {
	case "quiet":
		return OutputQuiet
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
)

// UIEvent is a message sent to the UI goroutine.
type UIEvent struct {
	Kind   UIEventKind
	Text   string
	Name   string
	Args   map[string]any
	Result string
	Err    error
	Index  int       // tool index in parallel batch
	Total  int       // total tools in batch
	Reply  chan bool  // for approval response
}

// toolLineState tracks one tool in a parallel batch.
type toolLineState struct {
	name    string
	args    map[string]any
	running bool
	result  string
	err     error
	start   time.Time
	lines   int // streaming line count (updated during execution)
}

// UI handles terminal output via a single goroutine that processes events.
type UI struct {
	term   *Terminal
	mode   OutputMode
	events chan UIEvent
	done   chan struct{}

	// State managed exclusively by the run() goroutine:
	wroteText         bool
	hasProgress       bool
	lastProgressLines int
	toolLines         []toolLineState
}

func NewUI(t *Terminal, mode OutputMode) *UI {
	if mode == OutputSilent {
		return &UI{mode: mode}
	}
	u := &UI{
		term:   t,
		mode:   mode,
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
func (u *UI) ToolDone(idx int, name string, args map[string]any, result string, err error) {
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
		if u.mode == OutputQuiet {
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
		if u.mode != OutputDefault {
			return
		}
		u.write(fmt.Sprintf("%s%s%s\n", dim, ev.Text, reset))

	case uiError:
		u.write(fmt.Sprintf("%s%serror: %s%s\n", bold, red, ev.Text, reset))
	}
}

// --- Tool progress (streaming args) ---

func (u *UI) handleToolProgress(name, argsSoFar string) {
	if u.mode == OutputQuiet {
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

	fmt.Fprintf(stderr, "\r\033[2K%s%s%s %s(%d lines)%s", yellow, name, reset, dim, lines, reset)
	stderr.Sync()
	u.hasProgress = true
}

// --- Parallel tool status lines ---

func (u *UI) handleToolStart(ev UIEvent) {
	if u.mode == OutputQuiet {
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
			fmt.Fprint(stderr, "\033[2K\r")
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
	if u.mode == OutputQuiet {
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
		fmt.Fprintf(stderr, "\033[%dA", linesUp)
	}
	fmt.Fprint(stderr, "\033[2K\r")

	tl := u.toolLines[idx]
	elapsed := time.Since(tl.start)
	label := toolLabel(tl.name, tl.args)

	elapsedStr := formatElapsed(elapsed)
	resultInfo := ""
	if tl.running && tl.lines > 0 {
		resultInfo = fmt.Sprintf("(%d lines) ", tl.lines)
	} else if !tl.running && tl.err == nil {
		lines := strings.Count(tl.result, "\n")
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

	fmt.Fprintf(stderr, "%s%s%s%s", bold, labelColor, label, reset+suffix)

	if linesUp > 0 {
		fmt.Fprintf(stderr, "\033[%dB\r", linesUp)
	}
	stderr.Sync()
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
		fmt.Fprint(stderr, "\033[2K\r")
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
		fmt.Fprint(stderr, s)
	}
}

func (u *UI) ensureNewline() {
	if u.wroteText {
		u.write("\n")
		u.wroteText = false
	}
}

func getTermWidth() int {
	w, _, _ := term.GetSize(int(os.Stderr.Fd()))
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
