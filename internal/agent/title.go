package agent

import (
	"context"
	"fmt"
	"io"
	"strings"

	t "github.com/meain/fin/internal/types"
)

// GenerateTitle asks the secondary model to produce a short session title
// from the last user message. parentTitle, when non-empty, is included so
// forked sessions get a title that reflects both origin and new direction.
// Returns "" when no secondary model is configured or there is no user message.
func (a *Agent) GenerateTitle(ctx context.Context, parentTitle string) (string, error) {
	tm := a.config.Models.Secondary
	if tm == "" {
		return "", nil
	}

	// Use the last user message, not the first. For new sessions both are the
	// same (only one user message exists at title-generation time). For forks,
	// SetMessages copies the parent history first, so the first user message
	// is the parent's opener — the last is the actual fork prompt.
	var userMsg string
	for i := len(a.messages) - 1; i >= 0; i-- {
		if a.messages[i].Role == t.RoleUser {
			userMsg = a.messages[i].Content
			break
		}
	}
	if userMsg == "" {
		return "", fmt.Errorf("no user message")
	}

	if len(userMsg) > 300 {
		userMsg = userMsg[:300]
	}

	prompt := "Generate a concise 3-7 word title describing what the user is asking about. Use sentence case (only first word and proper nouns capitalized). Do not answer the question. Reply with ONLY the title, no quotes or punctuation."
	if parentTitle != "" {
		prompt += "\n\nThis is a fork of a session titled: " + parentTitle
	}
	prompt += "\n\nUser: " + userMsg

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
