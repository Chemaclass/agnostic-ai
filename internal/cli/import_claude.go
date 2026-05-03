package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const claudeOnlyConfig = `version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - claude

on-unsupported: warn
`

type importCounts struct{ rules, agents, skills, hooks int }

// importFromClaude scaffolds an agnostic-ai project by reading existing
// Claude Code config (CLAUDE.md and .claude/) under root. Refuses if
// agnostic.config.yaml already exists.
func importFromClaude(root string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if err := ensureSourceDirs(root); err != nil {
		return err
	}

	c := importCounts{}
	var err error
	if c.rules, err = importClaudeRules(root); err != nil {
		return err
	}
	if c.agents, err = importClaudeAgents(root); err != nil {
		return err
	}
	if c.skills, err = importClaudeSkills(root); err != nil {
		return err
	}
	if c.hooks, err = importClaudeHooks(root); err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, []byte(claudeOnlyConfig), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("imported %d rules, %d agents, %d skills, %d hooks\n",
		c.rules, c.agents, c.skills, c.hooks)
	return nil
}

func ensureSourceDirs(root string) error {
	for _, d := range []string{"agents", "skills", "rules", "hooks"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// importClaudeRules splits CLAUDE.md on `## ` headings into one rule file
// per section. Without headings it writes a single rule named after the
// project directory.
func importClaudeRules(root string) (int, error) {
	src := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}

	sections := splitH2Sections(string(data))
	if len(sections) == 0 {
		body := strings.TrimSpace(string(data))
		if body == "" {
			return 0, nil
		}
		name := projectSlug(root)
		path := filepath.Join(root, "rules", name+".md")
		if err := writeRule(path, name, body); err != nil {
			return 0, err
		}
		return 1, nil
	}
	for _, s := range sections {
		path := filepath.Join(root, "rules", s.slug+".md")
		if err := writeRule(path, s.slug, s.body); err != nil {
			return 0, err
		}
	}
	return len(sections), nil
}

type h2Section struct{ slug, body string }

var h2HeadingRE = regexp.MustCompile(`(?m)^##[ \t]+(.+?)[ \t]*$`)

// splitH2Sections returns one section per `## heading`. Slug collisions
// are deduplicated with -2, -3 suffixes. Content before the first heading
// is discarded.
func splitH2Sections(s string) []h2Section {
	idx := h2HeadingRE.FindAllStringSubmatchIndex(s, -1)
	if len(idx) == 0 {
		return nil
	}
	out := make([]h2Section, 0, len(idx))
	used := map[string]int{}
	for i, m := range idx {
		title := s[m[2]:m[3]]
		base := slugify(title)
		if base == "" {
			base = fmt.Sprintf("section-%d", i+1)
		}
		slug := base
		if n, exists := used[base]; exists {
			used[base] = n + 1
			slug = fmt.Sprintf("%s-%d", base, n+1)
		} else {
			used[base] = 1
		}
		bodyStart := m[1]
		bodyEnd := len(s)
		if i+1 < len(idx) {
			bodyEnd = idx[i+1][0]
		}
		body := strings.TrimSpace(s[bodyStart:bodyEnd])
		out = append(out, h2Section{slug: slug, body: body})
	}
	return out
}

var nonAlphaNumRE = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lowercases and collapses non-alphanumeric runs into single
// hyphens. Leading/trailing hyphens trimmed.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = nonAlphaNumRE.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// projectSlug returns the basename of root, slugified. Falls back to
// "project" for unresolvable paths.
func projectSlug(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "project"
	}
	s := slugify(filepath.Base(abs))
	if s == "" {
		return "project"
	}
	return s
}

func writeRule(path, name, body string) error {
	fm := fmt.Sprintf("---\nname: %s\n---\n\n", name)
	if err := os.WriteFile(path, []byte(fm+body+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// importClaudeAgents copies .claude/agents/*.md byte-for-byte to agents/.
func importClaudeAgents(root string) (int, error) {
	src := filepath.Join(root, ".claude", "agents")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return count, fmt.Errorf("read agent %s: %w", e.Name(), err)
		}
		dst := filepath.Join(root, "agents", e.Name())
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// importClaudeSkills copies each .claude/skills/<name>/SKILL.md to
// skills/<name>/SKILL.md. Other files inside the skill dir are ignored.
func importClaudeSkills(root string) (int, error) {
	src := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(src, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read skill %s: %w", e.Name(), err)
		}
		dstDir := filepath.Join(root, "skills", e.Name())
		if err := os.MkdirAll(dstDir, 0o755); err != nil {
			return count, fmt.Errorf("mkdir %s: %w", dstDir, err)
		}
		dst := filepath.Join(dstDir, "SKILL.md")
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

type claudeHookCommand struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

type claudeHookGroup struct {
	Matcher string              `json:"matcher"`
	Hooks   []claudeHookCommand `json:"hooks"`
}

type claudeSettings struct {
	Hooks map[string][]claudeHookGroup `json:"hooks"`
}

// importClaudeHooks reads .claude/settings.json and writes one yaml per
// hook command into hooks/<event>-<group>-<index>.yaml.
func importClaudeHooks(root string) (int, error) {
	src := filepath.Join(root, ".claude", "settings.json")
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}
	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return 0, fmt.Errorf("parse %s: %w", src, err)
	}
	if len(s.Hooks) == 0 {
		return 0, nil
	}
	events := make([]string, 0, len(s.Hooks))
	for e := range s.Hooks {
		events = append(events, e)
	}
	sort.Strings(events)

	count := 0
	for _, event := range events {
		for gi, g := range s.Hooks[event] {
			for hi, h := range g.Hooks {
				name := fmt.Sprintf("%s-%d-%d", strings.ToLower(event), gi+1, hi+1)
				doc := map[string]any{
					"name":    name,
					"event":   event,
					"matcher": g.Matcher,
					"command": h.Command,
				}
				raw, err := yaml.Marshal(doc)
				if err != nil {
					return count, fmt.Errorf("marshal hook %s: %w", name, err)
				}
				path := filepath.Join(root, "hooks", name+".yaml")
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					return count, fmt.Errorf("write %s: %w", path, err)
				}
				count++
			}
		}
	}
	return count, nil
}
