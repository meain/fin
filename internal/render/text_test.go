package render

import "testing"

func TestVisibleLen_Tabs(t *testing.T) {
	cases := []struct {
		name string
		s    string
		want int
	}{
		{"no tab", "abc", 3},
		{"single tab at start", "\tabc", 8 + 3},
		{"tab mid-line", "ab\tcd", 8 + 2},
		{"two tabs", "\t\tx", 16 + 1},
		{"tab at exact stop", "12345678\tx", 16 + 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VisibleLen(c.s); got != c.want {
				t.Errorf("VisibleLen(%q) = %d, want %d", c.s, got, c.want)
			}
		})
	}
}

// TestVisibleLen_TabAlignmentDependsOnStartColumn documents why callers must
// measure a line together with any leading indent rather than measuring the
// indent and content separately: a tab's expansion depends on the absolute
// column it starts from, so "indent+content" is not the same width as
// len(indent)+VisibleLen(content) whenever content's own column lands
// exactly on a tab stop.
func TestVisibleLen_TabAlignmentDependsOnStartColumn(t *testing.T) {
	indent := "  "         // 2 columns
	content := "123456\tx" // tab at local column 6

	combined := VisibleLen(indent + content)
	separate := len(indent) + VisibleLen(content)

	if combined == separate {
		t.Fatalf("expected combined (%d) and separate (%d) measurements to diverge for this case", combined, separate)
	}
	// indent(2) + "123456"(6) = col 8, an exact tab stop, so the tab jumps a
	// full 8 columns to 16, then "x" -> 17.
	if combined != 17 {
		t.Errorf("combined VisibleLen = %d, want 17", combined)
	}
}

func TestTruncate_Tabs(t *testing.T) {
	s := "\tfmt.Println(\"hi\")"
	// tab expands to 8 columns, so with maxVisible=8 nothing after the tab fits.
	got := Truncate(s, 8)
	if VisibleLen(got) > 8 {
		t.Errorf("Truncate(%q, 8) = %q, visible width %d exceeds 8", s, got, VisibleLen(got))
	}
}
