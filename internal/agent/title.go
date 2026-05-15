package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	t "github.com/meain/fin/internal/types"
)

// GenerateTitle asks the secondary model to produce a short session title
// from the first user message. Returns "" when no secondary model is
// configured or when the conversation has no user message yet.
func (a *Agent) GenerateTitle(ctx context.Context) (string, error) {
	tm := a.config.Models.Secondary
	if tm == "" {
		return "", nil
	}

	var userMsg string
	for _, m := range a.messages {
		if m.Role == t.RoleUser {
			userMsg = m.Content
			break
		}
	}
	if userMsg == "" {
		return "", fmt.Errorf("no user message")
	}

	if len(userMsg) > 300 {
		userMsg = userMsg[:300]
	}

	prompt := "Generate a concise 3-7 word title describing what the user is asking about. Use sentence case (only first word and proper nouns capitalized). Do not answer the question. Reply with ONLY the title, no quotes or punctuation.\n\nUser: " + userMsg

	p, _, err := newProviderForModel(a.config, tm)
	if err != nil {
		return "", err
	}

	stream, err := p.StreamCompletion(ctx, t.CompletionRequest{
		Messages: []t.Message{{Role: t.RoleUser, Content: prompt}},
	})
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var buf strings.Builder
	for {
		delta, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", err
		}
		buf.WriteString(delta.Content)
	}

	title := strings.TrimSpace(buf.String())
	title = strings.Trim(title, `"'.`)
	title = strings.Join(strings.Fields(title), " ")
	if len(title) > 50 {
		title = title[:50]
	}
	return title, nil
}
