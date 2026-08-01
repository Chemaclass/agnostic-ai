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

// importFromCursor reads existing Cursor config (.cursor/rules/*.mdc,
// .cursor/agents/*.md, .cursor/skills/<name>/ folders,
// .cursor/commands/*.md) under root and writes specs into the
// configured source directories.
func importFromCursor(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules, src.Agents, src.Skills, src.Commands); err != nil {
		return err
	}
	rules, err := importCursorRules(root, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	agents, err := importCursorMarkdownFiles(filepath.Join(root, ".cursor", "agents"), filepath.Join(root, src.Agents))
	if err != nil {
		return err
	}
	skills, err := importCursorSkills(root, filepath.Join(root, src.Skills))
	if err != nil {
		return err
	}
	commands, err := importCursorMarkdownFiles(filepath.Join(root, ".cursor", "commands"), filepath.Join(root, src.Commands))
	if err != nil {
		return err
	}
	summaryf("imported %d rules, %d agents, %d skills, %d commands\n", rules, agents, skills, commands)
	printImportNextSteps(root, "cursor")
	return nil
}

// importCursorSkills copies each `.cursor/skills/<name>/` directory tree
// byte-for-byte into `<dstDir>/<name>/` via importSkillFolders.
func importCursorSkills(root, dstDir string) (int, error) {
	return importSkillFolders(filepath.Join(root, ".cursor", "skills"), dstDir)
}

// importCursorMarkdownFiles copies every top-level `*.md` in src
// byte-for-byte into dstDir, stripping the agnostic-ai provenance
// header when present. Covers Cursor's flat per-file surfaces:
// `.cursor/agents/*.md` (subagents) and `.cursor/commands/*.md`.
func importCursorMarkdownFiles(src, dstDir string) (int, error) {
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
		srcPath := filepath.Join(src, e.Name())
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return count, fmt.Errorf("read %s: %w", srcPath, err)
		}
		out := header.Strip(string(data))
		dstPath := filepath.Join(dstDir, e.Name())
		if err := importWriteFile(dstPath, []byte(out), 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", dstPath, err)
		}
		count++
	}
	return count, nil
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
	normalizeCursorRuleMeta(meta)

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

// normalizeCursorRuleMeta strips values that carry no scoping intent so
// a generated rule round-trips to the same source a hand-authored rule
// would produce (#429). A catch-all `globs` (hand-authored, or left over
// from a sync predating #536, when the adapter itself used to fill an
// empty globs with `**/*`) is dropped, as is an empty `description`;
// carrying either back makes import->sync non-idempotent on the source.
// Nil-valued keys are dropped too.
func normalizeCursorRuleMeta(meta map[string]any) {
	if g, ok := meta["globs"].(string); ok && isCatchAllGlobs(g) {
		delete(meta, "globs")
	}
	for k, v := range meta {
		if v == nil {
			delete(meta, k)
		}
	}
}

// isCatchAllGlobs reports whether g targets every file, matching the
// scope-routing convention (empty, `**/*`, or `*`). Such a glob carries
// no scoping intent regardless of whether a human wrote it directly or
// it is a leftover from a pre-#536 sync.
func isCatchAllGlobs(g string) bool {
	switch strings.TrimSpace(g) {
	case "", "**/*", "*":
		return true
	}
	return false
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
