package main

import (
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	t "github.com/meain/fin/internal/types"
)

// SessionMatch holds a session with its relevance score.
type SessionMatch struct {
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

// extractKeywords pulls meaningful words from a query string.
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

// scoreSession computes a relevance score for a session against the given keywords.
// Title matches are weighted 3x; content matches are capped at 5 occurrences per keyword.
// A recency bonus (up to +50%) is applied via exponential decay over one week.
func scoreSession(sess Session, keywords []string) float64 {
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
			raw += 3.0
		}
		count := strings.Count(content, kw)
		if count > 5 {
			count = 5
		}
		raw += float64(count)
	}

	// Recency bonus: exponential decay over 1 week
	const decayHours = 7 * 24.0
	ageHours := time.Since(lastMessageTime(sess)).Hours()
	recency := math.Exp(-ageHours / decayHours)
	raw *= 1 + recency*0.5

	return raw / float64(len(keywords))
}

// maxMatchScan caps how many recent sessions are scanned by FindMatchingSessions.
// Beyond this the cost of parsing JSONL bodies dominates and recency makes older
// hits unlikely to be useful anyway.
const maxMatchScan = 100

// FindMatchingSessions searches the most recent sessions and returns those
// whose score is at or above minScore, sorted by score descending.
// limit caps how many sessions are searched (0 = use maxMatchScan).
// The effective scan size is always min(limit, maxMatchScan).
func FindMatchingSessions(query string, limit int, minScore float64) []SessionMatch {
	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil
	}

	entries, err := sessionEntries()
	if err != nil || len(entries) == 0 {
		return nil
	}

	if limit <= 0 || limit > maxMatchScan {
		limit = maxMatchScan
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}

	paths := make([]string, len(entries))
	for i, e := range entries {
		paths[i] = e.path
	}
	sessions := parseSessionFiles(paths)

	var matches []SessionMatch
	for _, sess := range sessions {
		score := scoreSession(sess, keywords)
		if score >= minScore {
			matches = append(matches, SessionMatch{Session: sess, Score: score})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}
