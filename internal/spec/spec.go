package spec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

type Kind string

const (
	KindAgent Kind = "agent"
	KindSkill Kind = "skill"
	KindRule  Kind = "rule"
	KindHook  Kind = "hook"
)

type Entry struct {
	Kind Kind
	Name string
	Path string
	Meta map[string]any
	Body string
}

func LoadAll(root string, cfg *config.Config) ([]Entry, error) {
	var all []Entry

	loaders := []struct {
		dir   string
		kind  Kind
		ext   string
		parse func(string) (Entry, error)
	}{
		{filepath.Join(root, cfg.Sources.Agents), KindAgent, ".md", parseMarkdown},
		{filepath.Join(root, cfg.Sources.Skills), KindSkill, ".md", parseMarkdown},
		{filepath.Join(root, cfg.Sources.Rules), KindRule, ".md", parseMarkdown},
		{filepath.Join(root, cfg.Sources.Hooks), KindHook, ".yaml", parseYAML},
	}

	for _, l := range loaders {
		entries, err := walkDir(l.dir, l.ext, l.kind, l.parse)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
	}
	return all, nil
}

func Filter(entries []Entry, kind Kind) []Entry {
	var out []Entry
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func walkDir(dir, ext string, kind Kind, parse func(string) (Entry, error)) ([]Entry, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
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
	return entries, err
}

func parseMarkdown(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	meta, body := splitFrontmatter(data)
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
		return Entry{}, err
	}
	var meta map[string]any
	if err := yaml.Unmarshal(data, &meta); err != nil {
		return Entry{}, err
	}
	name, _ := meta["name"].(string)
	return Entry{
		Path: path,
		Name: name,
		Meta: meta,
	}, nil
}

func splitFrontmatter(data []byte) (map[string]any, string) {
	delim := []byte("---")
	if !bytes.HasPrefix(data, delim) {
		return map[string]any{}, string(data)
	}
	rest := data[len(delim):]
	if i := bytes.Index(rest, []byte("\n---")); i >= 0 {
		yamlPart := rest[:i]
		body := rest[i+len("\n---"):]
		body = bytes.TrimLeft(body, "\r\n")
		var meta map[string]any
		if err := yaml.Unmarshal(yamlPart, &meta); err != nil {
			return map[string]any{}, string(data)
		}
		if meta == nil {
			meta = map[string]any{}
		}
		return meta, string(body)
	}
	return map[string]any{}, string(data)
}
