package render

import (
	"fmt"
	"time"
)

// FormatElapsed prints sub-second durations as "Nms" and longer durations as
// "X.Ys".
func FormatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// RelativeTime renders an absolute time as a coarse relative string ("now",
// "5m", "3h", "2d"). Used in session listings and match prompts.
func RelativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
