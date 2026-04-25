package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	anthropicAPIVersion = "2023-06-01"
	anthropicMaxTokens  = 8192
)

type anthropicProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newAnthropicProvider(apiKey, baseURL string, httpClient *http.Client) Provider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &anthropicProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// --- Anthropic wire types ---

type anthRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	System    string        `json:"system,omitempty"`
	Messages  []anthMessage `json:"messages"`
	Tools     []anthTool    `json:"tools,omitempty"`
	Stream    bool          `json:"stream"`
}

type anthMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // string or []anthContentBlock
}

type anthContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	ID        string `json:"id,omitempty"`
	Name      string `json:"name,omitempty"`
	Input     any    `json:"input,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   any              `json:"content,omitempty"`
	IsError   bool             `json:"is_error,omitempty"`
	Source    *anthImageSource `json:"source,omitempty"`
}

type anthImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded
}

type anthTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// SSE event types
type anthContentBlockStart struct {
	Index        int `json:"index"`
	ContentBlock struct {
		Type  string `json:"type"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Text  string `json:"text,omitempty"`
		Input any    `json:"input,omitempty"`
	} `json:"content_block"`
}

type anthContentBlockDelta struct {
	Index int `json:"index"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text,omitempty"`
		PartialJSON string `json:"partial_json,omitempty"`
	} `json:"delta"`
}

type anthErrorEvent struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// --- Conversion: Message -> anthMessage ---

func messagesToAnthropic(msgs []Message) (system string, anthMsgs []anthMessage) {
	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			system = m.Content

		case RoleUser:
			anthMsgs = append(anthMsgs, anthMessage{Role: "user", Content: m.Content})

		case RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var blocks []anthContentBlock
				if m.Content != "" {
					blocks = append(blocks, anthContentBlock{Type: "text", Text: m.Content})
				}
				for _, tc := range m.ToolCalls {
					var input any
					if tc.Arguments != "" {
						_ = json.Unmarshal([]byte(tc.Arguments), &input)
					}
					if input == nil {
						input = map[string]any{}
					}
					blocks = append(blocks, anthContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Name,
						Input: input,
					})
				}
				anthMsgs = append(anthMsgs, anthMessage{Role: "assistant", Content: blocks})
			} else {
				anthMsgs = append(anthMsgs, anthMessage{Role: "assistant", Content: m.Content})
			}

		case RoleTool:
			var block anthContentBlock
			if len(m.Images) > 0 {
				// Tool result with images: use content array with image blocks
				var contentBlocks []anthContentBlock
				for _, img := range m.Images {
					contentBlocks = append(contentBlocks, anthContentBlock{
						Type: "image",
						Source: &anthImageSource{
							Type:      "base64",
							MediaType: img.MediaType,
							Data:      img.Data,
						},
					})
				}
				if m.Content != "" {
					contentBlocks = append(contentBlocks, anthContentBlock{
						Type: "text",
						Text: m.Content,
					})
				}
				block = anthContentBlock{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   contentBlocks,
					IsError:   strings.HasPrefix(m.Content, "Error: "),
				}
			} else {
				block = anthContentBlock{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   &m.Content,
					IsError:   strings.HasPrefix(m.Content, "Error: "),
				}
			}
			// Merge consecutive tool results into one user message
			if len(anthMsgs) > 0 {
				last := &anthMsgs[len(anthMsgs)-1]
				if last.Role == "user" {
					if blocks, ok := last.Content.([]anthContentBlock); ok {
						last.Content = append(blocks, block)
						continue
					}
				}
			}
			anthMsgs = append(anthMsgs, anthMessage{
				Role:    "user",
				Content: []anthContentBlock{block},
			})
		}
	}
	return
}

func toolDefsToAnthropic(tools []ToolDef) []anthTool {
	out := make([]anthTool, len(tools))
	for i, t := range tools {
		out[i] = anthTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		}
	}
	return out
}

// --- StreamCompletion ---

func (p *anthropicProvider) StreamCompletion(ctx context.Context, req CompletionRequest) (Stream, error) {
	system, anthMsgs := messagesToAnthropic(req.Messages)
	anthTools := toolDefsToAnthropic(req.Tools)

	body := anthRequest{
		Model:     req.Model,
		MaxTokens: anthropicMaxTokens,
		System:    system,
		Messages:  anthMsgs,
		Tools:     anthTools,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	url := strings.TrimSuffix(p.baseURL, "/") + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(b))
	}

	return &anthropicStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// --- Stream ---

type anthropicStream struct {
	reader     *bufio.Reader
	body       io.ReadCloser
	done       bool
	toolCallID string // current tool call being accumulated
}

func (s *anthropicStream) Close() {
	if s.body != nil {
		s.body.Close()
	}
}

func (s *anthropicStream) Recv() (StreamDelta, error) {
	for {
		if s.done {
			return StreamDelta{}, io.EOF
		}

		eventType, data, err := readSSEEvent(s.reader)
		if err != nil {
			if err == io.EOF {
				s.done = true
			}
			return StreamDelta{}, err
		}
		if eventType == "" || data == "" {
			continue
		}

		switch eventType {
		case "content_block_start":
			var ev anthContentBlockStart
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			if ev.ContentBlock.Type == "tool_use" {
				s.toolCallID = ev.ContentBlock.ID
				return StreamDelta{
					ToolCalls: []ToolCallDelta{{
						Index: ev.Index,
						ID:    ev.ContentBlock.ID,
						Name:  ev.ContentBlock.Name,
					}},
				}, nil
			}

		case "content_block_delta":
			var ev anthContentBlockDelta
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				continue
			}
			switch ev.Delta.Type {
			case "text_delta":
				return StreamDelta{Content: ev.Delta.Text}, nil
			case "input_json_delta":
				return StreamDelta{
					ToolCalls: []ToolCallDelta{{
						Index:     ev.Index,
						Arguments: ev.Delta.PartialJSON,
					}},
				}, nil
			}

		case "content_block_stop":
			s.toolCallID = ""

		case "message_stop":
			s.done = true
			return StreamDelta{}, io.EOF

		case "error":
			var ev anthErrorEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				return StreamDelta{}, fmt.Errorf("anthropic stream error: %s", data)
			}
			return StreamDelta{}, fmt.Errorf("anthropic error: %s: %s", ev.Error.Type, ev.Error.Message)
		}
	}
}

// readSSEEvent reads a single SSE event (event: + data: lines).
func readSSEEvent(r *bufio.Reader) (eventType, data string, err error) {
	var dataLines []string

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				// process last line
			} else if err == io.EOF {
				if eventType != "" || len(dataLines) > 0 {
					return eventType, strings.Join(dataLines, "\n"), nil
				}
				return "", "", io.EOF
			} else {
				return "", "", err
			}
		}

		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if eventType != "" || len(dataLines) > 0 {
				return eventType, strings.Join(dataLines, "\n"), nil
			}
			continue
		}

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
}
