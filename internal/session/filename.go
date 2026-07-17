package session

import "strings"

// filenameSep separates the positional fields of a session filename. Chosen
// over a single "_" because repo and session names may themselves contain
// "_" or "-"; a triple-dash is assumed not to appear in real repo/session
// names, so splitting on it is unambiguous even when those individual fields
// aren't.
const filenameSep = "---"

// filenameFields holds the positional fields encoded in a session filename.
type filenameFields struct {
	timestamp string
	uuid      string
	repo      string
	name      string
	temp      bool
}

// buildFilename encodes id/repo/name/temp into the current filename format:
//
//	<timestamp>---<uuid>---<repo>---<name>---<temp>.jsonl
//
// Unset optional fields (repo, name) are encoded as empty segments; temp is
// literally "temp" or empty. This keeps the field count fixed (5) so parsing
// never has to guess which optional suffixes are present.
func buildFilename(timestamp, id, repo, name string, temp bool) string {
	tempMarker := ""
	if temp {
		tempMarker = "temp"
	}
	return strings.Join([]string{timestamp, id, repo, name, tempMarker}, filenameSep) + ".jsonl"
}

// parseFilename parses a session filename in the current format. ok is false
// for anything that doesn't match — notably filenames written before this
// format existed, which callers should handle with a legacy fallback.
func parseFilename(filename string) (f filenameFields, ok bool) {
	base := strings.TrimSuffix(filename, ".jsonl")
	parts := strings.Split(base, filenameSep)
	if len(parts) != 5 {
		return filenameFields{}, false
	}
	return filenameFields{
		timestamp: parts[0],
		uuid:      parts[1],
		repo:      parts[2],
		name:      parts[3],
		temp:      parts[4] == "temp",
	}, true
}
