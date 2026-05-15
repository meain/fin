package render

import "strings"

// forEachVisibleRune walks s emitting visible runes (skipping ANSI escape
// sequences). Returning false from fn stops iteration. ANSI escape bytes
// are passed to fn with visible=false so callers that need to preserve
// formatting can write them through.
func forEachVisibleRune(s string, fn func(r rune, visible bool) bool) {
	inEsc := false
	for _, r := range s {
		if inEsc {
			if !fn(r, false) {
				return
			}
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == '\033' {
			inEsc = true
			if !fn(r, false) {
				return
			}
			continue
		}
		if !fn(r, true) {
			return
		}
	}
}

// VisibleLen returns the number of visible (non-ANSI-escape) characters in s.
func VisibleLen(s string) int {
	n := 0
	forEachVisibleRune(s, func(_ rune, visible bool) bool {
		if visible {
			n++
		}
		return true
	})
	return n
}

// Truncate trims s (which may contain ANSI codes) so the total visible width
// (including the trailing "…") does not exceed maxVisible. ANSI escape bytes
// inside the kept prefix are preserved; a trailing Reset is appended after
// the ellipsis when truncation occurs.
func Truncate(s string, maxVisible int) string {
	if maxVisible <= 0 {
		return ""
	}
	if VisibleLen(s) <= maxVisible {
		return s
	}

	cutoff := maxVisible - 1
	if cutoff < 0 {
		cutoff = 0
	}

	var out strings.Builder
	visible := 0
	truncated := false
	forEachVisibleRune(s, func(r rune, isVisible bool) bool {
		if !isVisible {
			out.WriteRune(r)
			return true
		}
		if visible >= cutoff {
			out.WriteString("…" + Reset)
			truncated = true
			return false
		}
		out.WriteRune(r)
		visible++
		return true
	})
	if !truncated {
		return s
	}
	return out.String()
}
