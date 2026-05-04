// Package spec loads and parses agnostic-ai source specs from disk.
//
// Specs come in five kinds (agent, skill, rule, hook, mcp). Markdown specs
// use YAML frontmatter; hook and mcp specs are pure YAML.
package spec

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// Kind names a spec category.
type Kind string

// Spec kinds.
const (
	KindAgent Kind = "agent"
	KindSkill Kind = "skill"
	KindRule  Kind = "rule"
	KindHook  Kind = "hook"
	KindMCP   Kind = "mcp"
)

// AllKinds is the canonical ordering used by adapters that emit
// per-kind sections.
var AllKinds = []Kind{KindAgent, KindSkill, KindRule, KindHook, KindMCP}

// Entry is a single loaded spec.
type Entry struct {
	Kind Kind
	Name string
	Path string
	Meta map[string]any
	Body string
}

// Description returns the entry's description from frontmatter, or "" if
// the field is missing or not a string.
func (e Entry) Description() string {
	d, _ := e.Meta["description"].(string)
	return d
}

// Globs returns the entry's globs frontmatter as a string, or "" if
// missing or not a string.
func (e Entry) Globs() string {
	g, _ := e.Meta["globs"].(string)
	return g
}

// Bundle is a pre-bucketed view of loaded entries. Adapters consume this
// directly to avoid repeated Filter calls.
type Bundle struct {
	Agents []Entry
	Skills []Entry
	Rules  []Entry
	Hooks  []Entry
	MCPs   []Entry
}

// NewBundle groups a flat slice of entries by kind. Useful in tests and
// when adapting external sources of Entry slices.
func NewBundle(entries []Entry) Bundle {
	var b Bundle
	for _, e := range entries {
		switch e.Kind {
		case KindAgent:
			b.Agents = append(b.Agents, e)
		case KindSkill:
			b.Skills = append(b.Skills, e)
		case KindRule:
			b.Rules = append(b.Rules, e)
		case KindHook:
			b.Hooks = append(b.Hooks, e)
		case KindMCP:
			b.MCPs = append(b.MCPs, e)
		}
	}
	return b
}

// All returns every entry in canonical kind order.
func (b Bundle) All() []Entry {
	out := make([]Entry, 0, len(b.Agents)+len(b.Skills)+len(b.Rules)+len(b.Hooks)+len(b.MCPs))
	out = append(out, b.Agents...)
	out = append(out, b.Skills...)
	out = append(out, b.Rules...)
	out = append(out, b.Hooks...)
	out = append(out, b.MCPs...)
	return out
}

// Has reports whether the bundle contains any entry of the given kind.
func (b Bundle) Has(k Kind) bool {
	switch k {
	case KindAgent:
		return len(b.Agents) > 0
	case KindSkill:
		return len(b.Skills) > 0
	case KindRule:
		return len(b.Rules) > 0
	case KindHook:
		return len(b.Hooks) > 0
	case KindMCP:
		return len(b.MCPs) > 0
	}
	return false
}

// LoadBundle walks the source directories and returns a pre-bucketed Bundle.
func LoadBundle(root string, cfg *config.Config) (Bundle, error) {
	var b Bundle
	loaders := []struct {
		dir   string
		ext   string
		kind  Kind
		parse func(string) (Entry, error)
		into  *[]Entry
	}{
		{filepath.Join(root, cfg.Sources.Agents), ".md", KindAgent, parseMarkdown, &b.Agents},
		{filepath.Join(root, cfg.Sources.Skills), ".md", KindSkill, parseMarkdown, &b.Skills},
		{filepath.Join(root, cfg.Sources.Rules), ".md", KindRule, parseMarkdown, &b.Rules},
		{filepath.Join(root, cfg.Sources.Hooks), ".yaml", KindHook, parseYAML, &b.Hooks},
		{filepath.Join(root, cfg.Sources.MCPs), ".yaml", KindMCP, parseYAML, &b.MCPs},
	}
	for _, l := range loaders {
		entries, err := walkDir(l.dir, l.ext, l.kind, l.parse)
		if err != nil {
			return Bundle{}, fmt.Errorf("load %s: %w", l.kind, err)
		}
		*l.into = entries
	}
	return b, nil
}

// LoadAll is a convenience wrapper that returns a flat slice. Prefer
// LoadBundle in new code.
func LoadAll(root string, cfg *config.Config) ([]Entry, error) {
	b, err := LoadBundle(root, cfg)
	if err != nil {
		return nil, err
	}
	return b.All(), nil
}

// Filter returns the subset of entries with the given kind.
func Filter(entries []Entry, kind Kind) []Entry {
	out := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func walkDir(dir, ext string, kind Kind, parse func(string) (Entry, error)) ([]Entry, error) {
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ext {
			return nil
		}
		entry, err := parse(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		entry.Kind = kind
		if entry.Name == "" {
			entry.Name = strings.TrimSuffix(d.Name(), ext)
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func parseMarkdown(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	meta, body := splitFrontmatter(normalizeLineEndings(data))
	name, _ := meta["name"].(string)
	return Entry{
		Path: path,
		Name: name,
		Meta: meta,
		Body: body,
	}, nil
}

func parseYAML(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read: %w", err)
	}
	var meta map[string]any
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return Entry{}, fmt.Errorf("parse yaml: %w", err)
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path: path,
		Name: name,
		Meta: meta,
	}, nil
}

// normalizeLineEndings converts CRLF to LF so the frontmatter splitter
// works on Windows-authored files.
func normalizeLineEndings(b []byte) []byte {
	if !bytes.Contains(b, []byte("\r\n")) {
		return b
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func splitFrontmatter(data []byte) (map[string]any, string) {
	const delim = "---"
	empty := map[string]any{}

	if !bytes.HasPrefix(data, []byte(delim)) {
		return empty, string(data)
	}
	rest := data[len(delim):]
	idx := bytes.Index(rest, []byte("\n"+delim))
	if idx < 0 {
		return empty, string(data)
	}
	yamlPart := rest[:idx]
	body := bytes.TrimLeft(rest[idx+len("\n"+delim):], "\n")

	var meta map[string]any
	if err := yaml.Unmarshal(yamlPart, &meta); err != nil {
		return empty, string(data)
	}
	if meta == nil {
		meta = empty
	}
	return meta, string(body)
}
