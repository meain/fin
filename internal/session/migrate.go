package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/fsutil"
)

// MigrateResult summarizes a Migrate run.
type MigrateResult struct {
	Renamed  int // files renamed to the current filename format
	Repaired int // files already in the current format whose header Repo was backfilled to match the filename
	Skipped  int // files already fully up to date
	Errors   []string
}

// Migrate brings every session file in the sessions directory up to date
// with the current filename format (see filename.go) and, where the header's
// Repo is empty, backfills it so header and filename agree.
//
// Repo is taken from the header if already set there; otherwise, for files
// already in the current filename format, from the filename's own repo
// field (repairing a header left behind by an earlier migration pass);
// otherwise best-effort backfilled by running fsutil.RepoRoot against the
// header's Cwd, if that directory still exists on disk. If none of that
// yields a repo, it's left empty, same as any other session with an unknown
// repo.
func Migrate() (MigrateResult, error) {
	dir := config.SessionPath()
	es, err := os.ReadDir(dir)
	if err != nil {
		return MigrateResult{}, err
	}

	var result MigrateResult
	for _, e := range es {
		if e.IsDir() || filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		path := filepath.Join(dir, e.Name())

		fnameFields, alreadyCurrent := parseFilename(e.Name())
		h, rest, err := readHeaderAndRest(path)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}

		repo := h.Repo
		switch {
		case repo != "":
			// already set, nothing to backfill
		case alreadyCurrent && fnameFields.repo != "":
			repo = fnameFields.repo
		case h.Cwd != "":
			if info, statErr := os.Stat(h.Cwd); statErr == nil && info.IsDir() {
				repo = fsutil.RepoName(h.Cwd)
			}
		}
		headerNeedsUpdate := repo != h.Repo

		newName := e.Name()
		if !alreadyCurrent {
			newName = buildFilename(h.StartedAt.Format("20060102-150405"), h.ID, repo, h.Name, h.Temp)
		}

		if alreadyCurrent && !headerNeedsUpdate {
			result.Skipped++
			continue
		}

		newPath := filepath.Join(dir, newName)
		if newName != e.Name() {
			if _, statErr := os.Stat(newPath); statErr == nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: target %s already exists, skipped", e.Name(), newName))
				continue
			}
		}

		if headerNeedsUpdate {
			h.Repo = repo
			headerJSON, err := json.Marshal(h)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", e.Name(), err))
				continue
			}
			content := append(headerJSON, '\n')
			content = append(content, rest...)
			if err := os.WriteFile(newPath, content, 0644); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("%s: write failed: %v", e.Name(), err))
				continue
			}
			if newPath != path {
				if err := os.Remove(path); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: removing old file after migrate failed: %v", e.Name(), err))
					continue
				}
			}
		} else if err := os.Rename(path, newPath); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: rename failed: %v", e.Name(), err))
			continue
		}

		if alreadyCurrent {
			result.Repaired++
		} else {
			result.Renamed++
		}
	}
	return result, nil
}
