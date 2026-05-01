package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	mathrand "math/rand/v2"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meain/fin/internal/provider"
	"github.com/meain/fin/internal/tool"
	t "github.com/meain/fin/internal/types"
)

// Agent orchestrates the conversation loop between user, LLM, and tools.
type Agent struct {
	provider provider.Provider
	tools    []tool.Tool
	config   *Config
	ui       *UI
	skills   []*Skill
	messages []t.Message
	Usage    t.Usage // accumulated token usage across all turns
	OnUpdate func([]t.Message)
}

func NewAgent(p provider.Provider, config *Config, ui *UI, skills []*Skill) *Agent {
	tools := buildTools(skills)
	systemPrompt := buildSystemPrompt(config, skills)

	a := &Agent{
		provider: p,
		tools:    tools,
		config:   config,
		ui:       ui,
		skills:   skills,
		messages: []t.Message{
			{Role: t.RoleSystem, Content: systemPrompt},
		},
	}

	// Wire up the subagent tool's runner callback.
	for _, tl := range a.tools {
		if st, ok := tl.(*tool.SubagentTool); ok {
			st.RunSubagent = a.runSubagent
			break
		}
	}

	return a
}

func buildTools(skills []*Skill) []tool.Tool {
	tools := tool.BuiltinTools()

	entries := loadBuiltinSkills()
	for _, s := range skills {
		entries = append(entries, tool.SkillEntry{
			Name:          s.Name,
			Description:   s.Description,
			Compatibility: s.Compatibility,
			Dir:           s.Dir,
		})
	}
	tools = append(tools, &tool.SkillTool{Skills: entries})

	// SubagentTool added here; RunSubagent callback is wired up in NewAgent.
	tools = append(tools, &tool.SubagentTool{})

	return tools
}

func loadBuiltinSkills() []tool.SkillEntry {
	var entries []tool.SkillEntry

	dirs, err := builtinSkillsFS.ReadDir("skills")
	if err != nil {
		return entries
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		data, err := builtinSkillsFS.ReadFile("skills/" + d.Name() + "/SKILL.md")
		if err != nil {
			continue
		}
		skill, err := parseSkillMD(data)
		if err != nil {
			continue
		}
		entries = append(entries, tool.SkillEntry{
			Name:        skill.Name,
			Description: skill.Description,
			Body:        skill.Body,
		})
	}

	return entries
}

// SetMessages restores messages (e.g., from a persisted session).
func (a *Agent) SetMessages(msgs []t.Message) {
	a.messages = msgs
}

// Messages returns the current conversation.
func (a *Agent) Messages() []t.Message {
	return a.messages
}

// AddUserMessage adds a user message and runs the agent loop.
func (a *Agent) AddUserMessage(ctx context.Context, content string) error {
	a.messages = append(a.messages, t.Message{Role: t.RoleUser, Content: content, Timestamp: time.Now()})
	return a.run(ctx)
}

func (a *Agent) run(ctx context.Context) error {
	maxTurns := a.config.Settings.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	for turn := 0; turn < maxTurns; turn++ {
		if turn == 0 {
			a.ui.AssistantLabel()
		}

		req := t.CompletionRequest{
			Model:    "",
			Messages: a.messages,
			Tools:    tool.Defs(a.tools),
		}

		stream, err := a.streamWithRetry(ctx, req)
		if err != nil {
			a.ui.EndStream()
			return fmt.Errorf("completion failed: %w", err)
		}

		assistantMsg, err := a.consumeStream(stream)
		stream.Close()
		if err != nil {
			a.ui.EndStream()
			return fmt.Errorf("stream error: %w", err)
		}

		a.ui.EndStream()
		a.messages = append(a.messages, assistantMsg)
		a.save()

		if len(assistantMsg.ToolCalls) == 0 {
			return nil
		}

		// Phase 1: approve all tools sequentially (interactive)
		type approvedTool struct {
			tc   t.ToolCall
			tool tool.Tool
			args map[string]any
			err  error
		}
		items := make([]approvedTool, len(assistantMsg.ToolCalls))
		for i, tc := range assistantMsg.ToolCalls {
			tl, args, err := a.approveTool(tc)
			items[i] = approvedTool{tc: tc, tool: tl, args: args, err: err}
		}

		// Phase 2: register tools with UI, then execute in parallel
		type toolExecResult struct {
			result t.ToolResult
			err    error
		}
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
				a.ui.ToolDone(i, item.tc.Name, item.args, "", item.err)
				continue
			}
			wg.Add(1)
			go func(i int, tl tool.Tool, name string, args map[string]any) {
				defer wg.Done()
				res, err := tl.Run(ctx, args)
				results[i] = toolExecResult{result: res, err: err}
				a.ui.ToolDone(i, name, args, res.Content, err)
			}(i, item.tool, item.tc.Name, item.args)
		}
		wg.Wait()

		// Phase 3: build messages (display already happened via ToolDone)
		for i, item := range items {
			r := results[i]
			msg := t.Message{
				Role:       t.RoleTool,
				ToolCallID: item.tc.ID,
				Timestamp:  time.Now(),
			}
			if item.err != nil {
				msg.Content = "Error: " + item.err.Error()
			} else if r.err != nil {
				msg.Content = "Error: " + r.err.Error()
			} else {
				msg.Content = r.result.Content
				msg.Images = r.result.Images
				msg.SubMessages = r.result.SubMessages
			}
			a.messages = append(a.messages, msg)
		}
		a.save()
	}

	return fmt.Errorf("max turns (%d) reached", maxTurns)
}

func (a *Agent) save() {
	if a.OnUpdate != nil {
		a.OnUpdate(a.messages)
	}
}

func (a *Agent) consumeStream(stream provider.Stream) (t.Message, error) {
	msg := t.Message{Role: t.RoleAssistant, Timestamp: time.Now()}
	var contentBuf strings.Builder

	toolCalls := map[int]*t.ToolCall{}

	for {
		delta, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return msg, err
		}

		if delta.Usage != nil {
			a.Usage.InputTokens += delta.Usage.InputTokens
			a.Usage.OutputTokens += delta.Usage.OutputTokens
			a.Usage.CacheCreationInputTokens += delta.Usage.CacheCreationInputTokens
			a.Usage.CacheReadInputTokens += delta.Usage.CacheReadInputTokens
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

	return msg, nil
}

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

	if !a.shouldAutoApprove(tc.Name, args) {
		a.ui.ToolCallStart(tc.Name, args)
		if !a.ui.ToolApprovalPrompt(tc.Name, args) {
			return nil, nil, fmt.Errorf("tool call denied by user")
		}
	}

	return tl, args, nil
}

func (a *Agent) shouldAutoApprove(toolName string, args map[string]any) bool {
	tc, ok := a.config.Tools[toolName]
	if !ok {
		return false
	}

	if tc.Approval == "auto" {
		return true
	}
	if tc.Approval == "deny" {
		return false
	}

	if toolName == "shell" {
		if cmd, ok := args["command"].(string); ok {
			for _, pattern := range tc.Deny {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return false
				}
			}
			for _, pattern := range tc.Allow {
				if matched, _ := filepath.Match(pattern, cmd); matched {
					return true
				}
			}
		}
	}

	return false
}

const (
	maxRetries     = 3
	baseRetryDelay = 1 * time.Second
	maxRetryDelay  = 30 * time.Second
)

func (a *Agent) streamWithRetry(ctx context.Context, req t.CompletionRequest) (provider.Stream, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		stream, err := a.provider.StreamCompletion(ctx, req)
		if err == nil {
			return stream, nil
		}

		lastErr = err

		errStr := err.Error()
		retryable := strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "500") ||
			strings.Contains(errStr, "502") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "529")

		if !retryable || attempt == maxRetries {
			return nil, err
		}

		delay := retryDelay(attempt)
		a.ui.Info(fmt.Sprintf("retrying in %s (%s)", delay.Round(time.Millisecond), errStr))

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, lastErr
}

func retryDelay(attempt int) time.Duration {
	delayF := float64(baseRetryDelay) * math.Pow(2, float64(attempt))
	if delayF > float64(maxRetryDelay) || delayF < 0 {
		delayF = float64(maxRetryDelay)
	}
	delay := time.Duration(delayF)

	half := int64(delay / 2)
	if half <= 0 {
		return delay
	}
	jitter := time.Duration(mathrand.Int64N(half))
	return delay + jitter
}

// runSubagent spawns an isolated child agent to handle a task.
// The child gets the same tools (minus subagent) and config, but a fresh conversation.
func (a *Agent) runSubagent(ctx context.Context, task, model string) (t.ToolResult, error) {
	// Build tools without SubagentTool to prevent nesting.
	childTools := tool.BuiltinTools()
	entries := loadBuiltinSkills()
	for _, s := range a.skills {
		entries = append(entries, tool.SkillEntry{
			Name:          s.Name,
			Description:   s.Description,
			Compatibility: s.Compatibility,
			Dir:           s.Dir,
		})
	}
	childTools = append(childTools, &tool.SkillTool{Skills: entries})

	systemPrompt := buildSystemPrompt(a.config, a.skills)

	p := a.provider
	if model != "" {
		providerName, modelName := resolveModel(model, a.config)
		providerCfg, ok := a.config.Providers[providerName]
		if !ok {
			return t.ToolResult{}, fmt.Errorf("unknown provider %q", providerName)
		}
		rawProvider, err := provider.New(providerName, provider.Config{
			BaseURL:   providerCfg.BaseURL,
			APIKeyEnv: providerCfg.APIKeyEnv,
			Headers:   providerCfg.Headers,
		})
		if err != nil {
			return t.ToolResult{}, fmt.Errorf("failed to create provider for subagent: %w", err)
		}
		p = &modelInjector{provider: rawProvider, model: modelName}
	}

	childUI := NewUI(nil, OutputSilent)
	child := &Agent{
		provider: p,
		tools:    childTools,
		config:   a.config,
		ui:       childUI,
		messages: []t.Message{
			{Role: t.RoleSystem, Content: systemPrompt},
		},
	}

	if err := child.AddUserMessage(ctx, task); err != nil {
		return t.ToolResult{}, err
	}

	// Return the last assistant message content + full conversation for export.
	for i := len(child.messages) - 1; i >= 0; i-- {
		if child.messages[i].Role == t.RoleAssistant && child.messages[i].Content != "" {
			return t.ToolResult{
				Content:     child.messages[i].Content,
				SubMessages: child.messages,
			}, nil
		}
	}

	return t.ToolResult{}, fmt.Errorf("subagent produced no response")
}
