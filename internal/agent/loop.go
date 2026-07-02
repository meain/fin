package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meain/fin/internal/provider"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
)

// approvedTool pairs a resolved tool call with the parsed args (or the
// approval error). Carried through the parallel-execution phase.
type approvedTool struct {
	tc   t.ToolCall
	tool tool.Tool
	args map[string]any
	err  error
}

type toolExecResult struct {
	result t.ToolResult
	err    error
}

// run iterates the conversation loop until the model emits an assistant
// message with no tool calls, the context is cancelled, or max turns is
// reached.
func (a *Agent) run(ctx context.Context) error {
	maxTurns := a.config.Settings.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	for turn := 0; turn < maxTurns; turn++ {
		a.ui.Debug(DebugTurnStart{Turn: turn + 1, MaxTurns: maxTurns, Messages: len(a.messages)})

		done, err := a.runTurn(ctx)
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}

	return fmt.Errorf("max turns (%d) reached", maxTurns)
}

// runTurn runs one iteration: stream, consume, dispatch tool calls. Returns
// done=true when the assistant emitted no tool calls (conversation ends).
func (a *Agent) runTurn(ctx context.Context) (bool, error) {
	a.fixDanglingToolCalls()

	req := t.CompletionRequest{
		Messages: a.messages,
		Tools:    tool.Defs(a.tools),
	}

	turnStart := time.Now()
	stream, err := a.streamWithRetry(ctx, req)
	if err != nil {
		a.ui.EndStream()
		return false, err
	}

	streamStart := time.Now()
	assistantMsg, ttft, err := a.consumeStream(stream, streamStart)
	stream.Close()
	if err != nil {
		a.ui.EndStream()
		return false, fmt.Errorf("stream error: %w", err)
	}

	a.ui.EndStream()
	a.messages = append(a.messages, assistantMsg)
	a.save()

	a.ui.Debug(DebugTurnDone{Usage: assistantMsg.Usage, TTFT: ttft, Elapsed: time.Since(turnStart)})

	if len(assistantMsg.ToolCalls) == 0 {
		return true, nil
	}

	items := a.approveAll(assistantMsg.ToolCalls)
	for _, item := range items {
		if item.err == nil {
			a.ui.Debug(DebugToolArgs{ToolName: item.tc.Name, ToolArgs: item.args})
		}
	}

	results := a.runToolsParallel(ctx, items)

	if summary := detectCompactSummary(items, results); summary != "" {
		a.applyCompact(summary)
		return false, nil
	}

	a.appendToolResults(items, results)
	a.save()
	return false, nil
}

// approveAll runs the approval prompt for each tool call sequentially.
// The phase is intentionally serial — interactive prompts can't safely run
// in parallel.
func (a *Agent) approveAll(calls []t.ToolCall) []approvedTool {
	items := make([]approvedTool, len(calls))
	for i, tc := range calls {
		tl, args, err := a.approveTool(tc)
		items[i] = approvedTool{tc: tc, tool: tl, args: args, err: err}
	}
	return items
}

// runToolsParallel registers approved tools with the UI then executes them
// concurrently. Tool callbacks (shell streaming) get a per-call wrapper.
func (a *Agent) runToolsParallel(ctx context.Context, items []approvedTool) []toolExecResult {
	results := make([]toolExecResult, len(items))

	for i, item := range items {
		if item.err != nil {
			results[i] = toolExecResult{err: item.err}
		} else {
			a.ui.ToolStart(i, len(items), item.tc.Name, item.args)
		}
	}

	var wg sync.WaitGroup
	for i, item := range items {
		if item.err != nil {
			a.ui.ToolDone(i, item.tc.Name, item.args, t.ToolResult{}, item.err)
			continue
		}
		wg.Add(1)
		go func(i int, tl tool.Tool, name string, args map[string]any) {
			defer wg.Done()
			if _, ok := tl.(*tool.ShellTool); ok {
				tl = &tool.ShellTool{OnOutput: func(lines int) {
					a.ui.ToolOutput(i, lines)
				}}
			}
			res, err := tl.Run(ctx, args)
			results[i] = toolExecResult{result: res, err: err}
			a.ui.ToolDone(i, name, args, res, err)
		}(i, item.tool, item.tc.Name, item.args)
	}
	wg.Wait()
	return results
}

// detectCompactSummary returns the first compact-tool summary string from
// successful results, or "" when none applies.
func detectCompactSummary(items []approvedTool, results []toolExecResult) string {
	for i, item := range items {
		if item.tc.Name == "compact" && item.err == nil && results[i].err == nil {
			if s, _ := item.args["summary"].(string); s != "" {
				return s
			}
		}
	}
	return ""
}

// applyCompact replaces the conversation with just the system message plus
// a user message containing the compact summary, then notifies the host
// via OnCompact (typically used to rotate the session writer).
func (a *Agent) applyCompact(summary string) {
	systemMsg := a.messages[0]
	a.messages = []t.Message{
		systemMsg,
		{Role: t.RoleUser, Content: "Summary of previous conversation:\n\n" + summary, Timestamp: time.Now()},
	}
	if a.OnCompact != nil {
		a.OnCompact()
	}
	a.save()
	a.ui.Info("conversation compacted")
}

// fixDanglingToolCalls scans for assistant messages whose tool calls have no
// corresponding tool_result in the following messages (e.g. process killed
// between the two saves). Injects a synthetic "Cancelled by user" result so
// Anthropic doesn't reject the conversation with a 400.
func (a *Agent) fixDanglingToolCalls() {
	fixed := make([]t.Message, 0, len(a.messages))
	for i, msg := range a.messages {
		fixed = append(fixed, msg)
		if msg.Role != t.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		// Collect tool_result IDs that immediately follow this message.
		coveredIDs := map[string]bool{}
		for j := i + 1; j < len(a.messages) && a.messages[j].Role == t.RoleTool; j++ {
			coveredIDs[a.messages[j].ToolCallID] = true
		}
		// Inject a synthetic result for any tool call without a result.
		for _, tc := range msg.ToolCalls {
			if !coveredIDs[tc.ID] {
				fixed = append(fixed, t.Message{
					Role:       t.RoleTool,
					ToolCallID: tc.ID,
					Content:    "Cancelled by user",
					Timestamp:  time.Now(),
				})
			}
		}
	}
	a.messages = fixed
}

// appendToolResults builds tool-role messages from the parallel execution
// results and appends them in the same order as the original tool calls.
func (a *Agent) appendToolResults(items []approvedTool, results []toolExecResult) {
	for i, item := range items {
		r := results[i]
		msg := t.Message{
			Role:       t.RoleTool,
			ToolCallID: item.tc.ID,
			Timestamp:  time.Now(),
		}
		switch {
		case item.err != nil:
			msg.Content = errorWithContext(item.tool, item.tc.Name, item.args, item.err)
		case r.err != nil:
			errMsg := errorWithContext(item.tool, item.tc.Name, item.args, r.err)
			if r.result.Content != "" {
				msg.Content = r.result.Content + "\n" + errMsg
			} else {
				msg.Content = errMsg
			}
		default:
			msg.Content = a.maybeSpillOutput(item.tc.Name, item.tc.ID, r.result.Content)
			msg.Images = r.result.Images
			msg.SubMessages = r.result.SubMessages
		}
		a.messages = append(a.messages, msg)
	}
}

const defaultMaxOutputBytes = 40000

// maybeSpillOutput checks if content exceeds the configured max_output_bytes
// for the tool (defaulting to defaultMaxOutputBytes). If so, writes the full
// output to /tmp/fin/<id>.txt and returns a truncated version with a pointer.
func (a *Agent) maybeSpillOutput(toolName, callID, content string) string {
	limit := defaultMaxOutputBytes
	if cfg, ok := a.config.Tools[toolName]; ok && cfg.MaxOutputBytes > 0 {
		limit = cfg.MaxOutputBytes
	}
	if len(content) <= limit {
		return content
	}

	dir := filepath.Join(os.TempDir(), "fin", a.sessionID)
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, callID+".txt")
	_ = os.WriteFile(path, []byte(content), 0644)

	truncated := content[:limit]
	return fmt.Sprintf("%s\n\n[Output truncated at %d bytes. Full output spilled to %s — use the Read tool with offset and limit parameters to read it in chunks (e.g. limit=500) to avoid hitting the same output limit again.]",
		truncated, limit, path)
}

// errorWithContext formats a tool error with the tool name and primary arg
// (e.g. "Error (edit /path/to/file): ...") so failures are easier to trace.
func errorWithContext(tl tool.Tool, name string, args map[string]any, err error) string {
	context := name
	if p, ok := tl.(tool.PrimaryArgProvider); ok {
		if primary := p.PrimaryArg(args); primary != "" {
			context = name + " " + primary
		}
	}
	return fmt.Sprintf("Error (%s): %s", context, err.Error())
}

// consumeStream drains a stream, accumulating text, tool-call fragments,
// and usage into one assistant Message. ttft is the time from streamStart
// to the first non-empty content or tool-call delta.
func (a *Agent) consumeStream(stream provider.Stream, streamStart time.Time) (t.Message, time.Duration, error) {
	msg := t.Message{Role: t.RoleAssistant, Model: a.model, Timestamp: time.Now()}
	var contentBuf strings.Builder
	var msgUsage t.Usage
	var ttft time.Duration

	toolCalls := map[int]*t.ToolCall{}

	for {
		delta, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return msg, ttft, err
		}

		if ttft == 0 && (delta.Content != "" || len(delta.ToolCalls) > 0) {
			ttft = time.Since(streamStart)
		}

		if delta.Usage != nil {
			a.Usage.InputTokens += delta.Usage.InputTokens
			a.Usage.OutputTokens += delta.Usage.OutputTokens
			a.Usage.CacheCreationInputTokens += delta.Usage.CacheCreationInputTokens
			a.Usage.CacheReadInputTokens += delta.Usage.CacheReadInputTokens

			msgUsage.InputTokens += delta.Usage.InputTokens
			msgUsage.OutputTokens += delta.Usage.OutputTokens
			msgUsage.CacheCreationInputTokens += delta.Usage.CacheCreationInputTokens
			msgUsage.CacheReadInputTokens += delta.Usage.CacheReadInputTokens
		}

		if delta.Content != "" {
			a.ui.StreamText(delta.Content)
			contentBuf.WriteString(delta.Content)
		}

		for _, tcd := range delta.ToolCalls {
			tc, exists := toolCalls[tcd.Index]
			if !exists {
				tc = &t.ToolCall{ID: tcd.ID, Name: tcd.Name}
				toolCalls[tcd.Index] = tc
			}
			if tcd.ID != "" {
				tc.ID = tcd.ID
			}
			if tcd.Name != "" {
				tc.Name = tcd.Name
			}
			tc.Arguments += tcd.Arguments
			a.ui.ToolCallProgress(tc.Name, tc.Arguments)
		}
	}

	msg.Content = contentBuf.String()
	if msgUsage.InputTokens > 0 || msgUsage.OutputTokens > 0 {
		msg.Usage = &msgUsage
	}

	if len(toolCalls) > 0 {
		maxIdx := 0
		for idx := range toolCalls {
			if idx > maxIdx {
				maxIdx = idx
			}
		}
		for i := 0; i <= maxIdx; i++ {
			if tc, ok := toolCalls[i]; ok {
				msg.ToolCalls = append(msg.ToolCalls, *tc)
			}
		}
	}

	return msg, ttft, nil
}

// approveTool looks up a tool by name, parses its JSON arguments, and
// prompts the user when auto-approval doesn't apply.
func (a *Agent) approveTool(tc t.ToolCall) (tool.Tool, map[string]any, error) {
	tl := tool.Find(a.tools, tc.Name)
	if tl == nil {
		return nil, nil, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return nil, nil, fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	if !a.approval.AutoApprove(tc.Name, args) {
		a.ui.ToolCallStart(tc.Name, args)
		if !a.ui.ToolApprovalPrompt(tc.Name, args) {
			return nil, nil, fmt.Errorf("tool call denied by user")
		}
	}

	return tl, args, nil
}
