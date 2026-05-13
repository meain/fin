package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	t "github.com/meain/fin/internal/types"
)

type openaiProvider struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func newOpenAIProvider(apiKey, baseURL string, httpClient *http.Client) Provider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &openaiProvider{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// --- OpenAI wire types ---

type oaiRequest struct {
	Model    string       `json:"model"`
	Messages []oaiMessage `json:"messages"`
	Tools    []oaiTool    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string        `json:"content,omitempty"`
			ToolCalls []oaiToolCall `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// --- Conversion ---

func messagesToOpenAI(msgs []t.Message) []oaiMessage {
	out := make([]oaiMessage, 0, len(msgs))
	for _, m := range msgs {
		om := oaiMessage{
			Role:       string(m.Role),
			ToolCallID: m.ToolCallID,
		}

		// Build content: use content parts array when images present
		if len(m.Images) > 0 && m.Role != t.RoleTool {
			parts := make([]oaiContentPart, 0, len(m.Images)+1)
			if m.Content != "" {
				parts = append(parts, oaiContentPart{Type: "text", Text: m.Content})
			}
			for _, img := range m.Images {
				parts = append(parts, oaiContentPart{
					Type: "image_url",
					ImageURL: &oaiImageURL{
						URL: "data:" + img.MediaType + ";base64," + img.Data,
					},
				})
			}
			om.Content = parts
		} else {
			om.Content = m.Content
		}

		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      tc.Name,
					Arguments: tc.Arguments,
				},
			})
		}
		out = append(out, om)

		// Tool results with images: OpenAI tool messages only accept string
		// content, so inject images as a follow-up user message
		if m.Role == t.RoleTool && len(m.Images) > 0 {
			parts := make([]oaiContentPart, 0, len(m.Images)+1)
			parts = append(parts, oaiContentPart{
				Type: "text",
				Text: "[Images from tool result]",
			})
			for _, img := range m.Images {
				parts = append(parts, oaiContentPart{
					Type: "image_url",
					ImageURL: &oaiImageURL{
						URL: "data:" + img.MediaType + ";base64," + img.Data,
					},
				})
			}
			out = append(out, oaiMessage{
				Role:    "user",
				Content: parts,
			})
		}
	}
	return out
}

func toolDefsToOpenAI(tools []t.ToolDef) []oaiTool {
	out := make([]oaiTool, len(tools))
	for i, td := range tools {
		out[i] = oaiTool{Type: "function"}
		out[i].Function.Name = td.Name
		out[i].Function.Description = td.Description
		out[i].Function.Parameters = td.Parameters
	}
	return out
}

// --- StreamCompletion ---

func (p *openaiProvider) StreamCompletion(ctx context.Context, req t.CompletionRequest) (Stream, error) {
	body := oaiRequest{
		Model:    req.Model,
		Messages: messagesToOpenAI(req.Messages),
		Tools:    toolDefsToOpenAI(req.Tools),
		Stream:   true,
	}
	if len(body.Tools) == 0 {
		body.Tools = nil
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal openai request: %w", err)
	}

	url := strings.TrimSuffix(p.baseURL, "/") + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, &APIError{Provider: "openai", StatusCode: resp.StatusCode, Body: b}
	}

	return &openaiStream{
		reader: bufio.NewReader(resp.Body),
		body:   resp.Body,
	}, nil
}

// --- Stream ---

type openaiStream struct {
	reader *bufio.Reader
	body   io.ReadCloser
	done   bool
}

func (s *openaiStream) Close() {
	if s.body != nil {
		s.body.Close()
	}
}

func (s *openaiStream) Recv() (t.StreamDelta, error) {
	for {
		if s.done {
			return t.StreamDelta{}, io.EOF
		}

		line, err := s.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.done = true
			}
			return t.StreamDelta{}, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			s.done = true
			return t.StreamDelta{}, io.EOF
		}

		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if len(chunk.Choices) == 0 {
			continue
		}

		delta := t.StreamDelta{
			Content: chunk.Choices[0].Delta.Content,
		}
		for _, tc := range chunk.Choices[0].Delta.ToolCalls {
			delta.ToolCalls = append(delta.ToolCalls, t.ToolCallDelta{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		if delta.Content != "" || len(delta.ToolCalls) > 0 {
			return delta, nil
		}
	}
}
