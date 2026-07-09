// Package skill loads agent skill definitions from project and user
// directories. Skills are markdown files with YAML frontmatter that the
// agent can activate at runtime.
package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/meain/fin/internal/config"
	"github.com/meain/fin/internal/fsutil"
	"gopkg.in/yaml.v3"
)

// Skill is a discovered agent skill. Name, Description, and Dir are
// populated on discovery; Body is loaded on activation via LoadBody.
type Skill struct {
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  string            `yaml:"allowed-tools,omitempty"`

	Dir  string
	Body string
}

var frontmatterRe = regexp.MustCompile(`(?s)\A---\n(.+?)\n---\n(.*)`)

// ParseMD parses a SKILL.md file's bytes into a Skill.
func ParseMD(data []byte) (*Skill, error) {
	matches := frontmatterRe.FindSubmatch(data)
	if matches == nil {
		return nil, fmt.Errorf("no YAML frontmatter found")
	}

	var s Skill
	if err := yaml.Unmarshal(matches[1], &s); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}

	if s.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if s.Description == "" {
		return nil, fmt.Errorf("skill description is required")
	}

	s.Body = strings.TrimSpace(string(matches[2]))
	return &s, nil
}

// Discover finds all skills in the standard locations: project .agents/skills/
// directories walked up from cwd, then ~/.agents/skills/, then any extra
// directories configured via settings.skills_dirs (each expected to directly
// contain <name>/SKILL.md subdirectories, like a skills/ folder). Returns
// skills with Body cleared (progressive disclosure). Earlier locations win on
// name collisions.
func Discover(cfg *config.Config) []*Skill {
	var skills []*Skill
	seen := map[string]bool{}

	add := func(found []*Skill) {
		for _, s := range found {
			if !seen[s.Name] {
				seen[s.Name] = true
				skills = append(skills, s)
			}
		}
	}

	fsutil.WalkUpFromCwd(func(dir string) {
		skillsDir := filepath.Join(dir, config.AgentsDir, config.SkillsDirName)
		add(scanDir(skillsDir))
	})

	if h := config.HomeDir(); h != "" {
		userDir := filepath.Join(h, config.AgentsDir, config.SkillsDirName)
		add(scanDir(userDir))
	}

	if cfg != nil {
		for _, dir := range cfg.Settings.SkillsDirs {
			add(scanDir(fsutil.ExpandHome(dir)))
		}
	}

	return skills
}

// scanDir reads one skills/ directory, following symlinks. Subdirectories
// whose Name field doesn't match their directory name are rejected.
func scanDir(dir string) []*Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []*Skill
	for _, e := range entries {
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, e.Name(), config.SkillFile)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}

		s, err := ParseMD(data)
		if err != nil {
			continue
		}

		if s.Name != e.Name() {
			continue
		}

		s.Dir = filepath.Join(dir, e.Name())
		s.Body = ""
		skills = append(skills, s)
	}

	return skills
}
