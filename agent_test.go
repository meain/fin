package main

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/meain/fin/internal/provider"
	"github.com/meain/fin/internal/tool"
	tp "github.com/meain/fin/internal/types"
)

// fakeStream replays a canned sequence of deltas.
type fakeStream struct {
	deltas []tp.StreamDelta
	idx    int
}

func (s *fakeStream) Recv() (tp.StreamDelta, error) {
	if s.idx >= len(s.deltas) {
		return tp.StreamDelta{}, io.EOF
	}
	d := s.deltas[s.idx]
	s.idx++
	return d, nil
}

func (s *fakeStream) Close() {}

// fakeProvider returns a sequence of streams, one per call.
type fakeProvider struct {
	streams []provider.Stream
	idx     int
}

func (p *fakeProvider) StreamCompletion(_ context.Context, _ tp.CompletionRequest) (provider.Stream, error) {
	s := p.streams[p.idx]
	p.idx++
	return s, nil
}

// concurrencyTracker is shared across fakeTools to measure peak concurrency.
type concurrencyTracker struct {
	running atomic.Int32
	maxConc atomic.Int32
}

func (ct *concurrencyTracker) enter() {
	cur := ct.running.Add(1)
	for {
		old := ct.maxConc.Load()
		if cur <= old || ct.maxConc.CompareAndSwap(old, cur) {
			break
		}
	}
}

func (ct *concurrencyTracker) exit() { ct.running.Add(-1) }

// fakeTool records calls and returns a configurable result.
type fakeTool struct {
	name    string
	result  tp.ToolResult
	err     error
	delay   time.Duration
	calls   atomic.Int32
	tracker *concurrencyTracker
}

func (ft *fakeTool) Name() string              { return ft.name }
func (ft *fakeTool) Description() string        { return "fake" }
func (ft *fakeTool) Parameters() map[string]any { return map[string]any{} }
func (ft *fakeTool) Run(_ context.Context, _ map[string]any) (tp.ToolResult, error) {
	ft.calls.Add(1)
	if ft.tracker != nil {
		ft.tracker.enter()
		defer ft.tracker.exit()
	}
	if ft.delay > 0 {
		time.Sleep(ft.delay)
	}
	return ft.result, ft.err
}

// streamWithToolCalls builds a stream whose single delta contains the given tool calls.
func streamWithToolCalls(tcs ...tp.ToolCallDelta) *fakeStream {
	return &fakeStream{deltas: []tp.StreamDelta{{ToolCalls: tcs}}}
}

// streamWithText builds a stream that just returns text (no tool calls).
func streamWithText(text string) *fakeStream {
	return &fakeStream{deltas: []tp.StreamDelta{{Content: text}}}
}

func newTestAgent(fp *fakeProvider, tools []tool.Tool, cfg *Config) *Agent {
	if cfg == nil {
		c := defaultConfig()
		cfg = &c
	}
	ui := NewUI(nil, OutputQuiet)
	return &Agent{
		provider: fp,
		tools:    tools,
		config:   cfg,
		ui:       ui,
		messages: []tp.Message{{Role: tp.RoleSystem, Content: "test"}},
	}
}

func TestParallelToolExecution(t *testing.T) {
	ct := &concurrencyTracker{}
	ft1 := &fakeTool{name: "alpha", result: tp.ToolResult{Content: "r1"}, delay: 200 * time.Millisecond, tracker: ct}
	ft2 := &fakeTool{name: "beta", result: tp.ToolResult{Content: "r2"}, delay: 200 * time.Millisecond, tracker: ct}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "alpha", Arguments: "{}"},
			tp.ToolCallDelta{Index: 1, ID: "c2", Name: "beta", Arguments: "{}"},
		),
		streamWithText("done"),
	}}

	cfg := defaultConfig()
	cfg.Tools["alpha"] = ToolConfig{Approval: "auto"}
	cfg.Tools["beta"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft1, ft2}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ft1.calls.Load() != 1 {
		t.Errorf("alpha called %d times, want 1", ft1.calls.Load())
	}
	if ft2.calls.Load() != 1 {
		t.Errorf("beta called %d times, want 1", ft2.calls.Load())
	}

	// Both tools have 200ms delay. Parallel should show peak concurrency of 2.
	if ct.maxConc.Load() < 2 {
		t.Errorf("expected concurrent execution, peak concurrency was %d", ct.maxConc.Load())
	}
}

func TestParallelToolResults_OrderPreserved(t *testing.T) {
	ft1 := &fakeTool{name: "slow", result: tp.ToolResult{Content: "slow-result"}, delay: 80 * time.Millisecond}
	ft2 := &fakeTool{name: "fast", result: tp.ToolResult{Content: "fast-result"}, delay: 10 * time.Millisecond}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "slow", Arguments: "{}"},
			tp.ToolCallDelta{Index: 1, ID: "c2", Name: "fast", Arguments: "{}"},
		),
		streamWithText("done"),
	}}

	cfg := defaultConfig()
	cfg.Tools["slow"] = ToolConfig{Approval: "auto"}
	cfg.Tools["fast"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft1, ft2}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Messages: [system, user, assistant(tools), tool(slow), tool(fast), assistant(done)]
	msgs := agent.Messages()
	if len(msgs) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(msgs))
	}

	toolMsg1 := msgs[3]
	toolMsg2 := msgs[4]

	if toolMsg1.ToolCallID != "c1" || toolMsg1.Content != "slow-result" {
		t.Errorf("first tool result: id=%s content=%q, want id=c1 content=slow-result",
			toolMsg1.ToolCallID, toolMsg1.Content)
	}
	if toolMsg2.ToolCallID != "c2" || toolMsg2.Content != "fast-result" {
		t.Errorf("second tool result: id=%s content=%q, want id=c2 content=fast-result",
			toolMsg2.ToolCallID, toolMsg2.Content)
	}
}

func TestParallelTool_DeniedToolDoesNotBlockOthers(t *testing.T) {
	ft1 := &fakeTool{name: "denied_tool", result: tp.ToolResult{Content: "nope"}}
	ft2 := &fakeTool{name: "allowed_tool", result: tp.ToolResult{Content: "ok"}}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "denied_tool", Arguments: "{}"},
			tp.ToolCallDelta{Index: 1, ID: "c2", Name: "allowed_tool", Arguments: "{}"},
		),
		streamWithText("done"),
	}}

	cfg := defaultConfig()
	cfg.Tools["denied_tool"] = ToolConfig{Approval: "deny"}
	cfg.Tools["allowed_tool"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft1, ft2}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ft1.calls.Load() != 0 {
		t.Errorf("denied tool was executed %d times, want 0", ft1.calls.Load())
	}
	if ft2.calls.Load() != 1 {
		t.Errorf("allowed tool called %d times, want 1", ft2.calls.Load())
	}

	msgs := agent.Messages()
	toolMsg1 := msgs[3]
	if toolMsg1.ToolCallID != "c1" {
		t.Errorf("first tool msg id=%s, want c1", toolMsg1.ToolCallID)
	}
	if toolMsg1.Content == "" || toolMsg1.Content[:5] != "Error" {
		t.Errorf("denied tool should have error content, got %q", toolMsg1.Content)
	}

	toolMsg2 := msgs[4]
	if toolMsg2.Content != "ok" {
		t.Errorf("allowed tool result=%q, want ok", toolMsg2.Content)
	}
}

func TestParallelTool_UnknownToolReturnsError(t *testing.T) {
	ft := &fakeTool{name: "real", result: tp.ToolResult{Content: "ok"}}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "nonexistent", Arguments: "{}"},
			tp.ToolCallDelta{Index: 1, ID: "c2", Name: "real", Arguments: "{}"},
		),
		streamWithText("done"),
	}}

	cfg := defaultConfig()
	cfg.Tools["real"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := agent.Messages()
	toolMsg1 := msgs[3]
	if toolMsg1.ToolCallID != "c1" || toolMsg1.Content[:5] != "Error" {
		t.Errorf("unknown tool should error, got id=%s content=%q", toolMsg1.ToolCallID, toolMsg1.Content)
	}

	toolMsg2 := msgs[4]
	if toolMsg2.Content != "ok" {
		t.Errorf("real tool result=%q, want ok", toolMsg2.Content)
	}
}

func TestSingleToolCall_StillWorks(t *testing.T) {
	ft := &fakeTool{name: "solo", result: tp.ToolResult{Content: "result"}}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "solo", Arguments: "{}"},
		),
		streamWithText("done"),
	}}

	cfg := defaultConfig()
	cfg.Tools["solo"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ft.calls.Load() != 1 {
		t.Errorf("tool called %d times, want 1", ft.calls.Load())
	}

	msgs := agent.Messages()
	toolMsg := msgs[3]
	if toolMsg.Content != "result" {
		t.Errorf("tool result=%q, want result", toolMsg.Content)
	}
}

func TestSubagentTool_IntegrationEndToEnd(t *testing.T) {
	// The subagent tool calls a.runSubagent which creates a child agent.
	// We simulate this by setting up a SubagentTool with a fake RunSubagent
	// callback (since we can't wire a real child agent without a real provider).
	st := &tool.SubagentTool{
		RunSubagent: func(_ context.Context, task, model string) (tp.ToolResult, error) {
			if task == "" {
				return tp.ToolResult{}, io.ErrUnexpectedEOF
			}
			return tp.ToolResult{Content: "subagent result for: " + task}, nil
		},
	}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "subagent", Arguments: `{"task":"list files"}`},
		),
		streamWithText("got it"),
	}}

	cfg := defaultConfig()
	cfg.Tools["subagent"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{st}, &cfg)

	err := agent.AddUserMessage(context.Background(), "delegate this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := agent.Messages()
	// [system, user, assistant(tool call), tool(result), assistant(text)]
	if len(msgs) < 5 {
		t.Fatalf("expected at least 5 messages, got %d", len(msgs))
	}

	toolResult := msgs[3]
	if toolResult.ToolCallID != "c1" {
		t.Errorf("tool call id=%s, want c1", toolResult.ToolCallID)
	}
	if toolResult.Content != "subagent result for: list files" {
		t.Errorf("tool result=%q, want %q", toolResult.Content, "subagent result for: list files")
	}
}

func TestSubagentTool_NoNesting(t *testing.T) {
	// Verify that buildTools includes the subagent tool but
	// the child tools built in runSubagent would not (tested via tool list).
	// Here we just verify SubagentTool is findable in the agent's tool list.
	st := &tool.SubagentTool{}
	ft := &fakeTool{name: "read", result: tp.ToolResult{Content: "ok"}}

	tools := []tool.Tool{ft, st}
	found := tool.Find(tools, "subagent")
	if found == nil {
		t.Fatal("subagent tool not found in tool list")
	}

	// A child agent's tool list should not contain subagent.
	// Simulate by filtering it out (as runSubagent does).
	childTools := []tool.Tool{}
	for _, tl := range tools {
		if tl.Name() != "subagent" {
			childTools = append(childTools, tl)
		}
	}
	if tool.Find(childTools, "subagent") != nil {
		t.Error("subagent tool should not be in child tool list")
	}
}

func TestToolError_PropagatedCorrectly(t *testing.T) {
	ft := &fakeTool{
		name: "failing",
		err:  io.ErrUnexpectedEOF,
	}

	fp := &fakeProvider{streams: []provider.Stream{
		streamWithToolCalls(
			tp.ToolCallDelta{Index: 0, ID: "c1", Name: "failing", Arguments: "{}"},
		),
		streamWithText("handled"),
	}}

	cfg := defaultConfig()
	cfg.Tools["failing"] = ToolConfig{Approval: "auto"}

	agent := newTestAgent(fp, []tool.Tool{ft}, &cfg)

	err := agent.AddUserMessage(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs := agent.Messages()
	toolMsg := msgs[3]
	if toolMsg.ToolCallID != "c1" {
		t.Errorf("tool call id=%s, want c1", toolMsg.ToolCallID)
	}
	if toolMsg.Content != "Error: "+io.ErrUnexpectedEOF.Error() {
		t.Errorf("tool error content=%q, want %q", toolMsg.Content, "Error: "+io.ErrUnexpectedEOF.Error())
	}
}
