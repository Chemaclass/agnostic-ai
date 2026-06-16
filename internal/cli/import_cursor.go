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

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importFromCursor reads existing Cursor config (.cursor/rules/*.mdc)
// under root and writes specs into the configured source directories.
func importFromCursor(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules); err != nil {
		return err
	}
	n, err := importCursorRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	summaryf("imported %d rules\n", n)
	printImportNextSteps(root, "cursor")
	return nil
}

// importCursorRules translates each .cursor/rules/**/*.mdc into a matching
// <dstDir>/**/<name>.md, walking the tree so nested rule directories
// (which Cursor reads recursively) are preserved. Frontmatter keys
// (description, globs, alwaysApply, plus any custom keys) pass through
// verbatim; a name field derived from the filename is injected when missing.
func importCursorRules(root, dstDir string) (int, error) {
	src := filepath.Join(root, ".cursor", "rules")
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, fmt.Errorf("%s: %w", src, err)
	}

	count := 0
	walkErr := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".mdc") {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		name := strings.TrimSuffix(d.Name(), ".mdc")
		translated, err := translateCursorRule(name, data)
		if err != nil {
			return fmt.Errorf("translate %s: %w", rel, err)
		}
		dst := filepath.Join(dstDir, strings.TrimSuffix(rel, ".mdc")+".md")
		if err := importMkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("%s: %w", filepath.Dir(dst), err)
		}
		if err := importWriteFile(dst, translated, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
		count++
		return nil
	})
	if walkErr != nil {
		return count, walkErr
	}
	return count, nil
}

// translateCursorRule rewrites a .mdc file as an agnostic rule. Frontmatter
// is parsed, a name field is injected if absent, and the result is
// re-marshaled. Body is preserved verbatim except for the agnostic-ai
// provenance header, which is stripped when present so it does not
// roundtrip back into the source spec.
func translateCursorRule(name string, data []byte) ([]byte, error) {
	data = []byte(header.Strip(string(data)))
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
