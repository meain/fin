package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill represents a discovered agent skill.
type Skill struct {
	// Frontmatter fields
	Name          string            `yaml:"name"`
	Description   string            `yaml:"description"`
	License       string            `yaml:"license,omitempty"`
	Compatibility string            `yaml:"compatibility,omitempty"`
	Metadata      map[string]string `yaml:"metadata,omitempty"`
	AllowedTools  string            `yaml:"allowed-tools,omitempty"`

	// Resolved at load time
	Dir  string // absolute path to the skill directory
	Body string // markdown body (loaded on activation)
}

var frontmatterRe = regexp.MustCompile(`(?s)\A---\n(.+?)\n---\n(.*)`)

// parseSkillMD parses a SKILL.md file into frontmatter and body.
func parseSkillMD(data []byte) (*Skill, error) {
	matches := frontmatterRe.FindSubmatch(data)
	if matches == nil {
		return nil, fmt.Errorf("no YAML frontmatter found")
	}

	var skill Skill
	if err := yaml.Unmarshal(matches[1], &skill); err != nil {
		return nil, fmt.Errorf("invalid frontmatter: %w", err)
	}

	if skill.Name == "" {
		return nil, fmt.Errorf("skill name is required")
	}
	if skill.Description == "" {
		return nil, fmt.Errorf("skill description is required")
	}

	skill.Body = strings.TrimSpace(string(matches[2]))
	return &skill, nil
}

// DiscoverSkills finds all skills in the standard locations.
// It returns skills with only name/description populated (body is deferred).
func DiscoverSkills(config *Config) []*Skill {
	var skills []*Skill
	seen := map[string]bool{}

	// 1. Project-level: .agents/skills/ in cwd and parent dirs
	if dir, err := os.Getwd(); err == nil {
		for {
			skillsDir := filepath.Join(dir, agentsDir, skillsDirName)
			for _, s := range scanSkillsDir(skillsDir) {
				if !seen[s.Name] {
					seen[s.Name] = true
					skills = append(skills, s)
				}
			}

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 2. User-level: ~/.agents/skills/
	if h := homeDir(); h != "" {
		userDir := filepath.Join(h, agentsDir, skillsDirName)
		for _, s := range scanSkillsDir(userDir) {
			if !seen[s.Name] {
				seen[s.Name] = true
				skills = append(skills, s)
			}
		}
	}

	return skills
}

// scanSkillsDir scans a directory for skill subdirectories containing SKILL.md.
func scanSkillsDir(dir string) []*Skill {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var skills []*Skill
	for _, e := range entries {
		// Follow symlinks: stat the entry to check if the target is a directory
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}

		skillPath := filepath.Join(dir, e.Name(), skillFile)
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}

		skill, err := parseSkillMD(data)
		if err != nil {
			continue
		}

		// Validate: name must match directory name
		if skill.Name != e.Name() {
			continue
		}

		skill.Dir = filepath.Join(dir, e.Name())
		// Clear body for discovery phase (progressive disclosure)
		skill.Body = ""
		skills = append(skills, skill)
	}

	return skills
}

// LoadSkillBody reads and returns the full SKILL.md body for a skill.
func LoadSkillBody(skill *Skill) (string, error) {
	skillPath := filepath.Join(skill.Dir, skillFile)
	data, err := os.ReadFile(skillPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", skillPath, err)
	}

	parsed, err := parseSkillMD(data)
	if err != nil {
		return "", err
	}

	return parsed.Body, nil
}

// LoadSkillFile reads a file relative to the skill directory.
func LoadSkillFile(skill *Skill, relPath string) (string, error) {
	// Prevent path traversal
	cleaned := filepath.Clean(relPath)
	if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("invalid path: must be relative to skill directory")
	}

	absPath := filepath.Join(skill.Dir, cleaned)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", relPath, err)
	}

	return string(data), nil
}
