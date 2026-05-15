package render

import (
	"testing"
	"time"
)

func TestRelativeTime_Now(t *testing.T) {
	if got := RelativeTime(time.Now()); got != "now" {
		t.Errorf("expected %q, got %q", "now", got)
	}
}

func TestRelativeTime_Minutes(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-5 * time.Minute)); got != "5m" {
		t.Errorf("expected %q, got %q", "5m", got)
	}
}

func TestRelativeTime_Hours(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-3 * time.Hour)); got != "3h" {
		t.Errorf("expected %q, got %q", "3h", got)
	}
}

func TestRelativeTime_Days(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-48 * time.Hour)); got != "2d" {
		t.Errorf("expected %q, got %q", "2d", got)
	}
}
