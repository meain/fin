package agent

import (
	"time"

	t "github.com/meain/fin/internal/types"
)

// UIWriter is the only contract between the agent and any UI implementation.
// Payloads are structured data — no ANSI escapes, no pre-formatted strings,
// no terminal-width assumptions. A future TUI or web frontend can implement
// this interface without modifying the agent.
type UIWriter interface {
	StreamText(text string)
	EndStream()
	ToolCallProgress(name, argsSoFar string)
	ToolStart(idx, total int, name string, args map[string]any)
	ToolDone(idx int, name string, args map[string]any, result t.ToolResult, err error)
	ToolOutput(idx int, line string, total int)
	ToolCallStart(name string, args map[string]any)
	ToolApprovalPrompt(name string, args map[string]any) bool
	Info(msg string)
	Error(msg string)
	SessionInfo(data SessionInfoData)
	Retry(data RetryData)
	Debug(ev DebugEvent)
}

// DebugEvent is implemented by all structured debug payloads.
type DebugEvent interface {
	debugEvent()
}

// DebugStartup is emitted once when the agent starts.
type DebugStartup struct {
	Model       string
	Skills      []string
	SessionID   string
	PromptChars int
}

// DebugTurnStart is emitted at the top of each agent loop iteration.
type DebugTurnStart struct {
	Turn     int
	MaxTurns int
	Messages int
}

// DebugTurnDone is emitted when the model stream finishes for a turn.
type DebugTurnDone struct {
	Usage   *t.Usage
	TTFT    time.Duration
	Elapsed time.Duration
}

// DebugToolArgs is emitted for each approved tool call before execution.
type DebugToolArgs struct {
	ToolName string
	ToolArgs map[string]any
}

// DebugMessageCount is emitted after each save with the current message count.
type DebugMessageCount struct {
	Messages int
}

// DebugSummary is emitted once at shutdown with cumulative stats.
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

// SessionInfoData carries session lifecycle data for the UI to render.
type SessionInfoData struct {
	Resumed   bool
	Label     string
	StartedAt time.Time
}

// RetryData carries retry information for the UI to render.
type RetryData struct {
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Err        error
}

// nullUI is a no-op UIWriter used by subagents (and any caller that wants to
// suppress output entirely without depending on the concrete UI package).
type nullUI struct{}

func (nullUI) StreamText(string)                                         {}
func (nullUI) EndStream()                                                {}
func (nullUI) ToolCallProgress(string, string)                           {}
func (nullUI) ToolStart(int, int, string, map[string]any)                {}
func (nullUI) ToolDone(int, string, map[string]any, t.ToolResult, error) {}
func (nullUI) ToolOutput(int, string, int)                               {}
func (nullUI) ToolCallStart(string, map[string]any)                      {}
func (nullUI) ToolApprovalPrompt(string, map[string]any) bool            { return false }
func (nullUI) Info(string)                                               {}
func (nullUI) Error(string)                                              {}
func (nullUI) SessionInfo(SessionInfoData)                               {}
func (nullUI) Retry(RetryData)                                           {}
func (nullUI) Debug(DebugEvent)                                          {}
