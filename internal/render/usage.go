package render

import (
	"fmt"

	t "github.com/meain/fin/internal/types"
)

// FormatUsage renders token-usage counters and a cache-hit percentage. Returns
// the empty string when u is nil.
func FormatUsage(u *t.Usage) string {
	if u == nil {
		return ""
	}
	totalIn := u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
	s := fmt.Sprintf("%d in, %d out", totalIn, u.OutputTokens)
	if u.CacheReadInputTokens > 0 || u.CacheCreationInputTokens > 0 {
		hitPct := 0.0
		if totalIn > 0 {
			hitPct = float64(u.CacheReadInputTokens) / float64(totalIn) * 100
		}
		s += fmt.Sprintf(" | cache: %.0f%%", hitPct)
	}
	return s
}
