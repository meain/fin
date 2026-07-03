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

// tabWidth is the column width a terminal expands a tab character to,
// matching common terminal defaults.
const tabWidth = 8

// advanceCol returns the terminal column after printing r, starting from
// col. Tabs advance to the next tab stop; everything else advances by one
// column.
func advanceCol(col int, r rune) int {
	if r == '\t' {
		return col + tabWidth - col%tabWidth
	}
	return col + 1
}

// VisibleLen returns the visible (non-ANSI-escape) width of s in terminal
// columns, expanding tabs to the next tab stop rather than counting them as
// a single character.
func VisibleLen(s string) int {
	col := 0
	forEachVisibleRune(s, func(r rune, visible bool) bool {
		if visible {
			col = advanceCol(col, r)
		}
		return true
	})
	return col
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
	col := 0
	truncated := false
	forEachVisibleRune(s, func(r rune, isVisible bool) bool {
		if !isVisible {
			out.WriteRune(r)
			return true
		}
		next := advanceCol(col, r)
		if next > cutoff {
			out.WriteString("…" + Reset)
			truncated = true
			return false
		}
		col = next
		out.WriteRune(r)
		return true
	})
	if !truncated {
		return s
	}
	return out.String()
}
