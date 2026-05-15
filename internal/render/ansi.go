// Package render holds pure formatting helpers — ANSI styling, text width
// math, duration/time formatting, token usage formatting. No I/O. Safe for
// any consumer (terminal UI, HTML export, future web renderer) to import.
package render

// ANSI escape sequences. Variables (not consts) so Disable can zero them
// when output is not a terminal.
var (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Magenta = "\033[35m"
	DimFg   = "\033[38;5;243m" // muted gray for debug lines
)

// Disable clears every ANSI escape, so all subsequent format strings emit
// plain text. Idempotent.
func Disable() {
	Reset = ""
	Bold = ""
	Dim = ""
	Red = ""
	Green = ""
	Yellow = ""
	Magenta = ""
	DimFg = ""
}
