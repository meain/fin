// Package agent orchestrates the conversation loop between user, LLM, and
// tools. It is UI-agnostic: all rendering happens behind the UIWriter
// interface defined in this package.
package agent

import (
	"context"
	"time"

	"github.com/meain/fin/internal/approval"
	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/prompt"
	"github.com/meain/fin/internal/provider"
	"github.com/meain/fin/internal/skill"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
)

// Agent runs an LLM conversation loop with tool support.
type Agent struct {
	provider  provider.Provider
	model     string
	tools     []tool.Tool
	config    *config.Config
	approval  *approval.Approval
	ui        UIWriter
	skills    []*skill.Skill
	sessionID string
	enabled   map[string]bool

	messages []t.Message
	Usage    t.Usage

	// QueueCh, when set, is drained between turns of run() so messages
	// queued externally (via `fin -q`) get injected into the conversation
	// as soon as the current turn finishes, rather than only after the
	// entire multi-turn run completes.
	QueueCh <-chan string

	// Callbacks. OnUpdate fires after every message append. OnCompact fires
	// when the compact tool produces a summary — caller typically rotates
	// the session writer.
	OnUpdate  func([]t.Message)
	OnCompact func()
}

// New constructs an agent and assembles its tool list and system prompt.
// ui may be nil; in that case a no-op UIWriter is used (suitable for tests
// or batch-mode callers).
func New(p provider.Provider, model string, cfg *config.Config, app *approval.Approval, ui UIWriter, skills []*skill.Skill, sessionID string, enabled map[string]bool) *Agent {
	if ui == nil {
		ui = nullUI{}
	}
	tools := BuildTools(skills, enabled, true)
	systemPrompt := prompt.BuildSystem(cfg, skills, sessionID, enabled)

	a := &Agent{
		provider:  p,
		model:     model,
		tools:     tools,
		config:    cfg,
		approval:  app,
		ui:        ui,
		skills:    skills,
		sessionID: sessionID,
		enabled:   enabled,
		messages: []t.Message{
			{Role: t.RoleSystem, Content: systemPrompt},
		},
	}

	// Wire up the subagent tool's runner callback if present.
	for _, tl := range a.tools {
		if st, ok := tl.(*tool.SubagentTool); ok {
			st.RunSubagent = a.runSubagent
			break
		}
	}

	return a
}

// SetMessages restores messages (e.g., from a persisted session).
func (a *Agent) SetMessages(msgs []t.Message) { a.messages = msgs }

// Messages returns the current conversation.
func (a *Agent) Messages() []t.Message { return a.messages }

// AddUserMessage appends a user message and runs one or more turns until
// the model produces a response without tool calls, the context is
// cancelled, or max turns is reached.
func (a *Agent) AddUserMessage(ctx context.Context, content string) error {
	a.messages = append(a.messages, t.Message{Role: t.RoleUser, Content: content, Timestamp: time.Now()})
	return a.run(ctx)
}

func (a *Agent) save() {
	if a.OnUpdate != nil {
		a.OnUpdate(a.messages)
	}
}
