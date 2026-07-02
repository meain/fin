package tool

import "strings"

// Previewer is implemented by tools whose full content is known before
// execution starts (write, edit) so the live expanded view can show a
// preview immediately when the tool starts running. Tools that only
// produce output as they run (shell) stream it via their own OnOutput
// callback instead and don't need this.
type Previewer interface {
	Preview(args map[string]any) []string
}

// PreviewFor looks up a previewer for the given tool name and computes its
// preview lines. Tools without one return nil (no scrollback content —
// only the header line is shown while running).
func PreviewFor(name string, args map[string]any) []string {
	if p, ok := previewers[name]; ok {
		return p.Preview(args)
	}
	return nil
}

// previewers is the static dispatch table used by PreviewFor.
var previewers = map[string]Previewer{
	"write": &WriteTool{},
	"edit":  &EditTool{},
}

// previewLines splits s into lines for preview display, returning nil for
// empty input.
func previewLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
