// Package ui is the terminal implementation of agent.UIWriter. It owns
// every formatting decision: ANSI codes, cursor moves, terminal width
// detection, raw-mode approval prompts. None of these leak back into
// agent or any other consumer.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/meain/fin/internal/agent"
	"github.com/meain/fin/internal/input"
	"github.com/meain/fin/internal/render"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
	"golang.org/x/term"
)

var stdout = os.Stdout

// OutputMode controls how much the UI displays.
type OutputMode int

const (
	OutputDefault OutputMode = iota // tool names + streamed text + tool detail (new default)
	OutputMinimal                   // same as old default: tool names + streamed text + tool detail
	OutputDebug                     // default + token usage
	OutputQuiet                     // only final response text (stdout)
	OutputSilent                    // no output at all (for subagents)
)

// ParseOutputMode maps a config/flag string to an OutputMode. Unknown
// values fall back to OutputDefault.
func ParseOutputMode(s string) OutputMode {
	switch s {
	case "minimal":
		return OutputMinimal
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
	uiSessionInfo  // session resumed/created (shown in default + debug)
	uiRetry        // retry attempt (shown in default + debug)
	uiDebug        // only shown in debug mode
	uiToolFinalize // delayed collapse of an expanded tool block once its hold elapses
)

// UIEvent is a message sent to the UI goroutine. Structured payload types
// (SessionInfoData, RetryData, DebugEvent) live in the agent package — the
// agent is their producer.
type UIEvent struct {
	Kind    UIEventKind
	Text    string
	Name    string
	Args    map[string]any
	Result  t.ToolResult
	Err     error
	Index   int
	Total   int
	Reply   chan bool
	Session *agent.SessionInfoData
	Retry   *agent.RetryData
	Debug   agent.DebugEvent
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

	// outputBuf holds the last few streamed output lines for tools shown
	// expanded (currently: shell, while running, in OutputDefault mode).
	outputBuf []string

	// renderedRows is how many terminal rows this tool's block occupied on
	// the last redraw, used to compute cursor-up distance for the next one.
	renderedRows int

	// previewDuration is how long a synthetic preview stream (write, edit —
	// tools whose content is fully known upfront) takes to finish revealing
	// its lines. The collapse hold waits at least this long so the reveal
	// isn't cut short.
	previewDuration time.Duration
}

// UI handles terminal output via a single goroutine that processes events.
type UI struct {
	term   *input.Terminal
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

// New constructs a UI bound to the given terminal (or nil) and mode. When
// mode is OutputSilent, returns a stub UI that no-ops on every method.
func New(t *input.Terminal, mode OutputMode, piped bool) *UI {
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

// ToolOutput updates a running tool's streaming line count and, for tools
// rendered expanded, appends the new output line to the live scrollback.
func (u *UI) ToolOutput(idx int, line string, total int) {
	u.send(UIEvent{Kind: uiToolOutput, Index: idx, Text: line, Total: total})
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

func (u *UI) SessionInfo(data agent.SessionInfoData) {
	u.send(UIEvent{Kind: uiSessionInfo, Session: &data})
}

func (u *UI) Retry(data agent.RetryData) {
	u.send(UIEvent{Kind: uiRetry, Retry: &data})
}

func (u *UI) Debug(ev agent.DebugEvent) {
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
			tl := &u.toolLines[ev.Index]
			tl.lines = ev.Total
			if u.isExpanded(*tl) {
				tl.outputBuf = append(tl.outputBuf, ev.Text)
				if len(tl.outputBuf) > scrollbackLines {
					tl.outputBuf = tl.outputBuf[len(tl.outputBuf)-scrollbackLines:]
				}
			}
			u.redrawAllTools()
		}

	case uiToolApproval:
		u.handleToolApproval(ev)

	case uiInfo:
		if u.piped || (u.mode != OutputDefault && u.mode != OutputMinimal && u.mode != OutputDebug) {
			return
		}
		u.write(fmt.Sprintf("%s%s%s\n", render.Dim, ev.Text, render.Reset))

	case uiError:
		u.write(fmt.Sprintf("%s%serror: %s%s\n", render.Bold, render.Red, ev.Text, render.Reset))

	case uiSessionInfo:
		if u.mode != OutputDebug {
			return
		}
		if ev.Session != nil {
			s := ev.Session
			if s.Resumed {
				u.write(fmt.Sprintf("%sresumed session %s (%s)%s\n",
					render.Dim, s.Label, s.StartedAt.Format("2006-01-02 15:04"), render.Reset))
			} else {
				u.write(fmt.Sprintf("%snew session [%s]%s\n", render.Dim, s.Label, render.Reset))
			}
		}

	case uiRetry:
		if ev.Retry != nil {
			r := ev.Retry
			u.write(fmt.Sprintf("%s%sretrying in %s (attempt %d/%d: %s)%s\n",
				render.Bold, render.Yellow, render.FormatElapsed(r.Delay), r.Attempt, r.MaxRetries, r.Err, render.Reset))
		}

	case uiDebug:
		u.renderDebug(ev.Debug)

	case uiToolFinalize:
		u.finalizeToolDone(ev.Index)
	}
}

// --- Debug rendering ---

func (u *UI) renderDebug(ev agent.DebugEvent) {
	if ev == nil {
		return
	}
	switch d := ev.(type) {
	case agent.DebugStartup:
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
	case agent.DebugTurnStart:
		u.debugLine(fmt.Sprintf("turn %d/%d | %d messages", d.Turn, d.MaxTurns, d.Messages))
	case agent.DebugTurnDone:
		s := render.FormatUsage(d.Usage)
		if d.TTFT > time.Millisecond {
			s += fmt.Sprintf(" | ttft: %s", render.FormatElapsed(d.TTFT))
		}
		if d.Elapsed > 0 {
			s += fmt.Sprintf(" | %s", render.FormatElapsed(d.Elapsed))
		}
		if s != "" {
			u.debugLine(s)
		}
	case agent.DebugToolArgs:
		argsJSON, _ := json.Marshal(d.ToolArgs)
		u.debugLine(fmt.Sprintf("  %s %s", d.ToolName, string(argsJSON)))
	case agent.DebugMessageCount:
		u.debugLine(fmt.Sprintf("%d messages", d.Messages))
	case agent.DebugSummary:
		s := "total: " + render.FormatUsage(d.Usage)
		s += fmt.Sprintf(" | %d messages", d.Messages)
		u.debugLine(s)
	}
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

	fmt.Fprintf(stdout, "\r\033[2K%s%s%s %s(%d lines)%s", render.Yellow, name, render.Reset, render.Dim, lines, render.Reset)
	stdout.Sync()
	u.hasProgress = true
}

// --- Parallel tool status lines ---

// scrollbackLines is how many trailing output lines are kept visible while
// a tool's expanded view is streaming (only tools that stream output, e.g.
// shell, ever populate this).
const scrollbackLines = 6

// minExpandedHold is the minimum time an expanded block with content (a
// preview or streamed output) stays on screen before collapsing. Fast tools
// like edit/write can otherwise finish inside a single terminal refresh,
// making the expanded view an imperceptible blip.
const minExpandedHold = 400 * time.Millisecond

// previewLineDelay/previewMaxTotal control the synthetic "streaming" reveal
// for tools whose full content is known upfront (write, edit) rather than
// produced incrementally like shell's real output. Lines are revealed one at
// a time so they behave like every other tool instead of flashing in fully
// formed, capped so large files don't take forever to reveal.
const (
	previewLineDelay = 60 * time.Millisecond
	previewMaxTotal  = 1200 * time.Millisecond
)

// previewDelay returns the per-line delay for animating n preview lines,
// shrinking (down to a 10ms floor) so the total reveal never exceeds
// previewMaxTotal.
func previewDelay(n int) time.Duration {
	if n <= 0 {
		return 0
	}
	d := previewLineDelay
	if time.Duration(n)*d > previewMaxTotal {
		d = previewMaxTotal / time.Duration(n)
	}
	if d < 10*time.Millisecond {
		d = 10 * time.Millisecond
	}
	return d
}

// streamPreview feeds lines into idx's scrollback one at a time via the same
// ToolOutput path shell output uses, so tools with fully-known-upfront
// content (write, edit) visibly stream instead of appearing all at once.
func (u *UI) streamPreview(idx int, lines []string, delay time.Duration) {
	for i, l := range lines {
		time.Sleep(delay)
		u.ToolOutput(idx, l, i+1)
	}
}

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

	tl := toolLineState{
		name:    ev.Name,
		args:    ev.Args,
		running: true,
		start:   time.Now(),
	}
	// Tools whose content is fully known upfront (write, edit) stream it into
	// the scrollback one line at a time in the background, exactly like
	// shell's real output, instead of showing it all at once.
	if u.mode == OutputDefault {
		if preview := tool.PreviewFor(ev.Name, ev.Args); len(preview) > 0 {
			delay := previewDelay(len(preview))
			tl.previewDuration = time.Duration(len(preview)) * delay
			go u.streamPreview(ev.Index, preview, delay)
		}
	}
	u.toolLines[ev.Index] = tl

	u.redrawAllTools()
}

func (u *UI) handleToolDone(ev UIEvent) {
	if u.mode == OutputQuiet || u.piped {
		return
	}

	if ev.Index < 0 || ev.Index >= len(u.toolLines) {
		return
	}

	tl := &u.toolLines[ev.Index]
	tl.result = ev.Result
	tl.err = ev.Err

	// Give the expanded preview/stream a moment to actually be visible before
	// collapsing (only applies to tools that showed or are still streaming
	// something — plain header-only tools like read collapse immediately).
	// Scheduled as a delayed event rather than a blocking sleep, so the
	// single UI goroutine keeps processing the in-flight streamPreview
	// events (and anything else) in real time instead of stalling.
	if u.mode == OutputDefault && (len(tl.outputBuf) > 0 || tl.previewDuration > 0) {
		hold := minExpandedHold
		if tl.previewDuration > hold {
			hold = tl.previewDuration
		}
		if remaining := hold - time.Since(tl.start); remaining > 0 {
			idx := ev.Index
			time.AfterFunc(remaining, func() {
				u.send(UIEvent{Kind: uiToolFinalize, Index: idx})
			})
			return
		}
	}

	u.finalizeToolDone(ev.Index)
}

// finalizeToolDone flips a tool from running (expanded) to done (collapsed)
// and redraws. Called immediately from handleToolDone when no hold is
// needed, or later via a uiToolFinalize event once the hold elapses.
func (u *UI) finalizeToolDone(idx int) {
	if idx < 0 || idx >= len(u.toolLines) {
		return
	}

	u.toolLines[idx].running = false

	// Redraw: expanded blocks collapse to a single summary line the moment
	// running flips false, since blockLines only expands while running.
	u.redrawAllTools()

	// If all tools are done, clear the batch state.
	if !u.hasRunningTools() {
		u.toolLines = nil
	}
}

// isExpanded reports whether tl should render as a live multi-line block
// (full label header + any streaming scrollback) instead of a single
// collapsed status line. Every tool renders this way while running, in the
// default output mode — not just ones that stream output.
func (u *UI) isExpanded(tl toolLineState) bool {
	return u.mode == OutputDefault && tl.running
}

// blockLines returns the terminal lines to render for one tool's current
// state: a multi-line expanded block (full, untruncated label plus any
// streamed output) while running, or a single collapsed status line once
// it's done. All tools go through the same rendering path.
func (u *UI) blockLines(tl toolLineState) []string {
	if u.isExpanded(tl) {
		lines := make([]string, 0, 1+len(tl.outputBuf))
		lines = append(lines, fmt.Sprintf("%s%s%s%s", render.Bold, render.Yellow, toolLabel(tl.name, tl.args), render.Reset))
		// -2 for the "  " indent, -5 margin of error so slightly-off
		// width calculations (wide runes, etc.) don't still wrap.
		maxOutput := getTermWidth() - 2 - 5
		for _, o := range tl.outputBuf {
			o = render.Truncate(o, maxOutput)
			lines = append(lines, fmt.Sprintf("  %s%s%s", render.Dim, o, render.Reset))
		}
		return lines
	}
	return []string{u.collapsedLine(tl)}
}

// collapsedLine renders a tool's single-line summary: "…" placeholder
// details while running, or elapsed time / line count / error once resolved.
func (u *UI) collapsedLine(tl toolLineState) string {
	label := toolLabel(tl.name, tl.args)
	elapsedStr := render.FormatElapsed(time.Since(tl.start))

	resultInfo := ""
	if tl.running && tl.lines > 0 {
		resultInfo = fmt.Sprintf("(%d lines) ", tl.lines)
	} else if !tl.running && tl.err == nil {
		if lines := strings.Count(tl.result.Content, "\n"); lines > 0 {
			resultInfo = fmt.Sprintf("(%d lines) ", lines)
		}
	}

	var suffix string
	labelColor := render.Yellow
	if tl.err != nil {
		labelColor = render.Red
		suffix = fmt.Sprintf(" %s(error) %s%s", render.Dim, elapsedStr, render.Reset)
	} else {
		suffix = fmt.Sprintf(" %s%s%s%s", render.Dim, resultInfo, elapsedStr, render.Reset)
	}

	suffixVisible := render.VisibleLen(suffix)
	maxLabel := getTermWidth() - suffixVisible - 1
	label = render.Truncate(label, maxLabel)

	return fmt.Sprintf("%s%s%s%s%s", render.Bold, labelColor, label, render.Reset, suffix)
}

// rowsFor returns how many terminal rows a rendered line occupies once the
// terminal wraps it, so cursor-movement math stays correct for long lines.
func rowsFor(s string) int {
	w := getTermWidth()
	if w <= 0 {
		w = 80
	}
	vl := render.VisibleLen(s)
	if vl == 0 {
		return 1
	}
	return (vl + w - 1) / w
}

// redrawAllTools redraws every announced tool's block in the current batch.
// It moves the cursor up by the total rows rendered last time, clears to
// end of screen, then reprints each tool's current block, recording the new
// row counts for next time. Tools that haven't started yet (zero value,
// empty name) are skipped since nothing was ever printed for them.
func (u *UI) redrawAllTools() {
	if len(u.toolLines) == 0 {
		return
	}

	up := 0
	for i := range u.toolLines {
		up += u.toolLines[i].renderedRows
	}
	if up > 0 {
		fmt.Fprintf(stdout, "\033[%dA", up)
	}
	fmt.Fprint(stdout, "\r\033[0J")

	for i := range u.toolLines {
		tl := &u.toolLines[i]
		if tl.name == "" {
			break
		}
		rows := 0
		for _, l := range u.blockLines(*tl) {
			// Raw mode disables OPOST, so a bare "\n" won't return the
			// cursor to column 0 — write "\r\n" explicitly like the other
			// cursor-movement writes in this function do.
			fmt.Fprint(stdout, l+"\r\n")
			rows += rowsFor(l)
		}
		tl.renderedRows = rows
	}
	stdout.Sync()
}

func (u *UI) refreshToolLines() {
	if u.hasRunningTools() {
		u.redrawAllTools()
	}
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
	u.write(fmt.Sprintf("%s%s%s\n", render.Yellow, toolLabel(name, args), render.Reset))
}

// --- Approval prompt ---

func (u *UI) handleToolApproval(ev UIEvent) {
	var approved bool
	if u.term != nil {
		u.term.WriteString(fmt.Sprintf("%s%sallow %s? [y/N]%s ", render.Bold, render.Yellow, ev.Name, render.Reset))
		line, err := u.term.ReadLine("")
		if err == nil {
			line = strings.TrimSpace(strings.ToLower(line))
			approved = line == "y" || line == "yes"
		}
	} else {
		u.write(fmt.Sprintf("%s%sallow %s? [y/N]%s ", render.Bold, render.Yellow, ev.Name, render.Reset))
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
	u.write(fmt.Sprintf("%s│ %s%s\n", render.DimFg, text, render.Reset))
}

func getTermWidth() int {
	w, _, _ := term.GetSize(int(os.Stdout.Fd()))
	if w <= 0 {
		w = 80
	}
	return w - 1 // leave 1 col to prevent terminal line wrap
}

// toolLabel renders a tool name plus its structured ToolLabel as a terminal
// line ("read agent.go", "shell $ ls", "edit foo.go (-3 +5)"). Label primary
// and detail come from tool.LabelFor; ANSI wrapping happens here so the same
// label can be rendered as plain text by other consumers.
func toolLabel(name string, args map[string]any) string {
	label := tool.LabelFor(name, args)
	switch {
	case label.Primary != "" && label.Detail != "":
		return fmt.Sprintf("%s%s %s%s (%s)", name, render.Reset, render.Dim, label.Primary, label.Detail)
	case label.Primary != "":
		return name + render.Reset + " " + render.Dim + label.Primary
	default:
		return name
	}
}
