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
	"time"
)

// Agent orchestrates the conversation loop between user, LLM, and tools.
type Agent struct {
	provider Provider
	tools    []Tool
	config   *Config
	ui       *UI
	messages []Message
	OnUpdate func([]Message) // called after each turn so the session can be saved
}

func NewAgent(provider Provider, config *Config, ui *UI, skills []*Skill) *Agent {
	tools := AllTools(skills)
	systemPrompt := buildSystemPrompt(config, skills)

	return &Agent{
		provider: provider,
		tools:    tools,
		config:   config,
		ui:       ui,
		messages: []Message{
			{Role: RoleSystem, Content: systemPrompt},
		},
	}
}

// SetMessages restores messages (e.g., from a persisted session).
func (a *Agent) SetMessages(msgs []Message) {
	a.messages = msgs
}

// Messages returns the current conversation.
func (a *Agent) Messages() []Message {
	return a.messages
}

// AddUserMessage adds a user message and runs the agent loop.
func (a *Agent) AddUserMessage(ctx context.Context, content string) error {
	a.messages = append(a.messages, Message{Role: RoleUser, Content: content, Timestamp: time.Now()})
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

		req := CompletionRequest{
			Model:    "", // set by caller via provider
			Messages: a.messages,
			Tools:    ToolDefsFrom(a.tools),
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

		// No tool calls — done
		if len(assistantMsg.ToolCalls) == 0 {
			return nil
		}

		// Execute tool calls
		for _, tc := range assistantMsg.ToolCalls {
			result, err := a.executeTool(ctx, tc)
			msg := Message{
				Role:       RoleTool,
				ToolCallID: tc.ID,
				Timestamp:  time.Now(),
			}
			if err != nil {
				msg.Content = "Error: " + err.Error()
			} else {
				msg.Content = result.Content
				msg.Images = result.Images
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

// consumeStream reads the full streaming response, printing text and accumulating tool calls.
func (a *Agent) consumeStream(stream Stream) (Message, error) {
	msg := Message{Role: RoleAssistant, Timestamp: time.Now()}
	var contentBuf strings.Builder

	// Track in-progress tool calls by index
	toolCalls := map[int]*ToolCall{}

	for {
		delta, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return msg, err
		}

		if delta.Content != "" {
			a.ui.StreamText(delta.Content)
			contentBuf.WriteString(delta.Content)
		}

		for _, tcd := range delta.ToolCalls {
			tc, exists := toolCalls[tcd.Index]
			if !exists {
				tc = &ToolCall{ID: tcd.ID, Name: tcd.Name}
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

	// Flatten tool calls map to slice in index order
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

func (a *Agent) executeTool(ctx context.Context, tc ToolCall) (ToolResult, error) {
	tool := FindTool(a.tools, tc.Name)
	if tool == nil {
		return ToolResult{}, fmt.Errorf("unknown tool: %s", tc.Name)
	}

	var args map[string]any
	if tc.Arguments != "" {
		if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
			return ToolResult{}, fmt.Errorf("invalid tool arguments: %w", err)
		}
	}
	if args == nil {
		args = map[string]any{}
	}

	a.ui.ToolCallStart(tc.Name, args)

	// Check approval
	if !a.shouldAutoApprove(tc.Name, args) {
		if !a.ui.ToolApprovalPrompt(tc.Name, args) {
			return ToolResult{}, fmt.Errorf("tool call denied by user")
		}
	}

	result, err := tool.Run(ctx, args)
	a.ui.ToolCallResult(result.Content, err)
	return result, err
}

func (a *Agent) shouldAutoApprove(toolName string, args map[string]any) bool {
	tc, ok := a.config.Tools[toolName]
	if !ok {
		return false // unknown tool → confirm
	}

	if tc.Approval == "auto" {
		return true
	}
	if tc.Approval == "deny" {
		return false
	}

	// For shell, check allow/deny patterns
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

// streamWithRetry wraps StreamCompletion with exponential backoff on retryable errors.
func (a *Agent) streamWithRetry(ctx context.Context, req CompletionRequest) (Stream, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		stream, err := a.provider.StreamCompletion(ctx, req)
		if err == nil {
			return stream, nil
		}

		lastErr = err

		// Only retry on rate limits or server errors
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
