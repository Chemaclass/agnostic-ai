package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const cursorOnlyConfig = `version: 1

sources:
  agents: agents
  skills: skills
  rules: rules
  hooks: hooks

targets:
  - cursor

on-unsupported: warn
`

// importFromCursor scaffolds an agnostic-ai project by reading existing
// Cursor config (.cursor/rules/*.mdc) under root. Refuses if
// agnostic.config.yaml already exists.
func importFromCursor(root string) error {
	cfgPath := filepath.Join(root, "agnostic.config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return fmt.Errorf("agnostic.config.yaml already exists")
	}
	if err := ensureSourceDirs(root); err != nil {
		return err
	}

	n, err := importCursorRules(root)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, []byte(cursorOnlyConfig), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfgPath, err)
	}
	fmt.Printf("imported %d rules\n", n)
	return nil
}

// importCursorRules translates each .cursor/rules/*.mdc into rules/<name>.md.
// Frontmatter keys (description, globs, alwaysApply, plus any custom keys)
// pass through verbatim; a name field derived from the filename is injected
// when missing.
func importCursorRules(root string) (int, error) {
	src := filepath.Join(root, ".cursor", "rules")
	entries, err := os.ReadDir(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}

	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".mdc") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			return count, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		name := strings.TrimSuffix(e.Name(), ".mdc")
		translated, err := translateCursorRule(name, data)
		if err != nil {
			return count, fmt.Errorf("translate %s: %w", e.Name(), err)
		}
		dst := filepath.Join(root, "rules", name+".md")
		if err := os.WriteFile(dst, translated, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dst, err)
		}
		count++
	}
	return count, nil
}

// translateCursorRule rewrites a .mdc file as an agnostic rule. Frontmatter
// is parsed, a name field is injected if absent, and the result is
// re-marshaled. Body is preserved verbatim.
func translateCursorRule(name string, data []byte) ([]byte, error) {
	meta, body := splitMdcFrontmatter(data)
	if _, ok := meta["name"]; !ok {
		meta["name"] = name
	}

	var fm bytes.Buffer
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(meta); err != nil {
		return nil, fmt.Errorf("marshal frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}

	var out bytes.Buffer
	out.WriteString("---\n")
	out.Write(fm.Bytes())
	out.WriteString("---\n\n")
	out.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		out.WriteString("\n")
	}
	return out.Bytes(), nil
}

// splitMdcFrontmatter mirrors spec.splitFrontmatter for the import path.
// Returns an empty map (never nil) when no valid frontmatter is present so
// callers can always inject keys.
func splitMdcFrontmatter(data []byte) (map[string]any, string) {
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
