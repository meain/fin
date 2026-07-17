package session

import "testing"

func TestBuildParseFilename_RoundTrip(t *testing.T) {
	cases := []struct {
		name string
		f    filenameFields
	}{
		{"full", filenameFields{timestamp: "20260717-150405", uuid: "e2caa380-0373-45ba-8f0b-ffe8c86184d7", repo: "fin", name: "mysession", temp: true}},
		{"no repo", filenameFields{timestamp: "20260717-150405", uuid: "e2caa380-0373-45ba-8f0b-ffe8c86184d7", name: "mysession"}},
		{"no name", filenameFields{timestamp: "20260717-150405", uuid: "e2caa380-0373-45ba-8f0b-ffe8c86184d7", repo: "fin"}},
		{"bare", filenameFields{timestamp: "20260717-150405", uuid: "e2caa380-0373-45ba-8f0b-ffe8c86184d7"}},
		{"repo with hyphen and underscore", filenameFields{timestamp: "20260717-150405", uuid: "e2caa380-0373-45ba-8f0b-ffe8c86184d7", repo: "control-plane_backend"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			filename := buildFilename(c.f.timestamp, c.f.uuid, c.f.repo, c.f.name, c.f.temp)
			got, ok := parseFilename(filename)
			if !ok {
				t.Fatalf("parseFilename(%q) returned ok=false", filename)
			}
			if got != c.f {
				t.Errorf("round trip mismatch: got %+v, want %+v", got, c.f)
			}
		})
	}
}

func TestParseFilename_Unrecognized(t *testing.T) {
	cases := []string{
		"20260717-150405_e2caa380-0373-45ba-8f0b-ffe8c86184d7.jsonl", // legacy format
		"not-a-session-file.jsonl",
		"20260717-150405_e2caa380-0373-45ba-8f0b-ffe8c86184d7_mysession_temp.jsonl", // legacy named+temp
	}
	for _, filename := range cases {
		if _, ok := parseFilename(filename); ok {
			t.Errorf("parseFilename(%q) unexpectedly returned ok=true", filename)
		}
	}
}
