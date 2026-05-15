package session

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/meain/fin/internal/config"
	t "github.com/meain/fin/internal/types"
)

// Match holds a session with its relevance score.
type Match struct {
	Session Session
	Score   float64
}

// stopWords are common words excluded from keyword extraction.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "it": true,
	"in": true, "on": true, "at": true, "to": true, "for": true,
	"of": true, "and": true, "or": true, "but": true, "not": true,
	"with": true, "this": true, "that": true, "was": true, "are": true,
	"be": true, "as": true, "by": true, "from": true, "i": true,
	"my": true, "me": true, "we": true, "you": true, "do": true,
	"how": true, "what": true, "why": true, "can": true, "will": true,
	"have": true, "has": true, "had": true, "get": true, "got": true,
	"its": true, "so": true, "up": true, "if": true, "no": true,
	"just": true, "about": true, "out": true, "use": true, "also": true,
}

// extractKeywords pulls meaningful words (>2 chars, not stop words, deduped)
// from a query string.
func extractKeywords(query string) []string {
	words := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := map[string]bool{}
	var keywords []string
	for _, w := range words {
		if len(w) > 2 && !stopWords[w] && !seen[w] {
			keywords = append(keywords, w)
			seen[w] = true
		}
	}
	return keywords
}

// scoreSession computes a relevance score for a session against the given
// keywords. Tuning constants come from cfg.Settings.Matching: title hits
// weighted by TitleWeight, content hits per keyword capped at ContentCap,
// and a recency bonus (up to +RecencyBonus) via exponential decay over
// RecencyDecayDay days.
func scoreSession(sess Session, keywords []string, mc config.MatchingConfig) float64 {
	if len(keywords) == 0 {
		return 0
	}

	title := strings.ToLower(sess.Title)

	var sb strings.Builder
	for _, msg := range sess.Messages {
		if msg.Role == t.RoleUser || msg.Role == t.RoleAssistant {
			sb.WriteString(strings.ToLower(msg.Content))
			sb.WriteByte(' ')
		}
	}
	content := sb.String()

	var raw float64
	for _, kw := range keywords {
		if strings.Contains(title, kw) {
			raw += mc.TitleWeight
		}
		count := strings.Count(content, kw)
		if count > mc.ContentCap {
			count = mc.ContentCap
		}
		raw += float64(count)
	}

	decayHours := float64(mc.RecencyDecayDay) * 24
	ageHours := time.Since(LastMessageTime(sess)).Hours()
	recency := math.Exp(-ageHours / decayHours)
	raw *= 1 + recency*mc.RecencyBonus

	return raw / float64(len(keywords))
}

// maxMatchScan caps how many recent sessions FindMatching scans. Beyond
// this the cost of parsing JSONL bodies dominates and recency makes older
// hits unlikely to be useful.
const maxMatchScan = 100

// FindMatching searches the most recent sessions and returns those whose
// score is at or above minScore, sorted by score descending. limit caps
// how many sessions are searched (0 = use maxMatchScan). The effective
// scan size is always min(limit, maxMatchScan).
func FindMatching(query string, limit int, minScore float64, mc config.MatchingConfig) []Match {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	es, err := entries()
	if err != nil || len(es) == 0 {
		return nil
	}

	if limit <= 0 || limit > maxMatchScan {
		limit = maxMatchScan
	}
	if len(es) > limit {
		es = es[:limit]
	}

	paths := make([]string, len(es))
	for i, e := range es {
		paths[i] = e.path
	}
	sessions := parseFiles(paths)

	var matches []Match
	for _, sess := range sessions {
		score := scoreSession(sess, keywords, mc)
		if score >= minScore {
			matches = append(matches, Match{Session: sess, Score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })

	return matches
}
