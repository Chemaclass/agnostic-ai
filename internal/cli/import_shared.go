package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// extractLeadingItalic returns the inner text of a `_..._` paragraph
// at the start of body (no surrounding whitespace) and the body with
// that paragraph plus its trailing blank line stripped. ok is false
// when body does not begin with a single-line italic paragraph.
//
// Several adapters render `description:` as a leading italic paragraph
// in the emitted markdown. Importers use this helper to reverse that
// transformation so a description round-trips through frontmatter
// rather than accumulating as body content.
func extractLeadingItalic(body string) (desc, stripped string, ok bool) {
	trimmed := strings.TrimLeft(body, "\n")
	nl := strings.IndexByte(trimmed, '\n')
	var first string
	if nl < 0 {
		first = trimmed
	} else {
		first = trimmed[:nl]
	}
	first = strings.TrimSpace(first)
	if len(first) < 3 || !strings.HasPrefix(first, "_") || !strings.HasSuffix(first, "_") {
		return "", body, false
	}
	if strings.Count(first, "_") != 2 {
		return "", body, false
	}
	desc = first[1 : len(first)-1]
	rest := ""
	if nl >= 0 {
		rest = strings.TrimLeft(trimmed[nl+1:], "\n")
	}
	return desc, rest, true
}

// sliceMainFileByH2 splits <root>/<srcName> on `## ` headings into one
// rule per section in dstDir. Without headings it writes a single rule
// named after the project directory. No-op when the source file is
// absent or empty. Reused by every importer whose target keeps rules
// in a single concatenated markdown file (CONVENTIONS.md, AGENTS.md,
// GEMINI.md, .opencode/AGENTS.md, .rules, etc.).
func sliceMainFileByH2(root, srcName, dstDir string) (int, error) {
	src := filepath.Join(root, srcName)
	data, err := os.ReadFile(src)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", src, err)
	}

	preamble, sections := splitH2Sections(string(data))
	if len(sections) == 0 {
		body := strings.TrimSpace(string(data))
		if body == "" {
			return 0, nil
		}
		name := projectSlug(root)
		path := filepath.Join(dstDir, name+".md")
		if err := writeRule(path, name, body); err != nil {
			return 0, err
		}
		return 1, nil
	}

	used := map[string]int{}
	for _, s := range sections {
		used[s.slug] = 1
	}
	count := 0
	if preamble != "" {
		slug := preambleSlug(preamble, used)
		path := filepath.Join(dstDir, slug+".md")
		if err := writeRule(path, slug, preamble); err != nil {
			return 0, err
		}
		count++
	}
	for _, s := range sections {
		path := filepath.Join(dstDir, s.slug+".md")
		if err := writeRule(path, s.slug, s.body); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// importJSONMCPMap reads a JSON file at srcPath, extracts the
// flat-or-dotted key, and writes one yaml per server into dstDir.
// Common helper for amp / opencode / vscode-style MCP shapes where
// servers are a map keyed by name.
func importJSONMCPMap(srcPath, mapKey, dstDir string) (int, error) {
	servers, err := readJSONMapAt(srcPath, mapKey)
	if err != nil || len(servers) == 0 {
		return 0, err
	}
	return writeMCPYAMLs(servers, dstDir)
}

// readJSONMapAt loads srcPath as JSON and returns the map at mapKey.
// Supports a dotted key like "amp.mcpServers" so callers can target a
// nested key without writing custom decoders. Missing file or missing
// key returns (nil, nil); only IO and parse failures bubble up.
func readJSONMapAt(srcPath, mapKey string) (map[string]any, error) {
	data, err := os.ReadFile(srcPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", srcPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	v, ok := doc[mapKey]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	out := map[string]any{}
	for k, val := range m {
		sub, ok := val.(map[string]any)
		if !ok {
			continue
		}
		out[k] = sub
	}
	return out, nil
}

// writeMCPYAMLs writes one yaml file per server into dstDir. Each
// destination doc has `name: <key>` prepended; server fields pass
// through verbatim so transport-specific keys (command/args/env or
// url/headers) survive a round-trip.
func writeMCPYAMLs(servers map[string]any, dstDir string) (int, error) {
	names := make([]string, 0, len(servers))
	for k := range servers {
		names = append(names, k)
	}
	sort.Strings(names)
	count := 0
	for _, name := range names {
		entry, _ := servers[name].(map[string]any)
		doc := map[string]any{"name": name}
		for k, v := range entry {
			doc[k] = v
		}
		raw, err := yaml.Marshal(doc)
		if err != nil {
			return count, fmt.Errorf("marshal mcp %s: %w", name, err)
		}
		path := filepath.Join(dstDir, name+".yaml")
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			return count, fmt.Errorf("write %s: %w", path, err)
		}
		count++
	}
	return count, nil
}

// writeAgentMD writes an agent spec to path with a name + optional
// description and tags frontmatter, followed by body.
func writeAgentMD(path, name, description string, tags []string, body string) error {
	var sb strings.Builder
	sb.WriteString("---\nname: " + name + "\n")
	if description != "" {
		sb.WriteString("description: " + description + "\n")
	}
	if len(tags) > 0 {
		sb.WriteString("tags: [" + strings.Join(tags, ", ") + "]\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(strings.TrimRight(body, "\n"))
	sb.WriteString("\n")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// copyDirTree walks srcDir recursively and writes every regular file
// byte-for-byte into the matching location under dstDir, recreating
// the directory layout as it goes. File mode bits are preserved so an
// executable script remains executable on the destination. Symlinks
// are not followed; if they appear inside a skill folder they are
// silently skipped (skills are documented to be plain files +
// directories — symlinks would not survive a tar/zip release anyway).
func copyDirTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Strip the agnostic-ai provenance header from SKILL.md so a
		// roundtrip (claude / codex emit -> import) does not bake the
		// header into the source spec. Sibling assets pass through
		// byte-for-byte because they are user-authored.
		if filepath.Base(path) == "SKILL.md" {
			data = []byte(header.Strip(string(data)))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
		}
		if err := os.WriteFile(target, data, info.Mode().Perm()); err != nil {
			return fmt.Errorf("write %s: %w", target, err)
		}
		return nil
	})
}

// printImportNextSteps emits the post-import guidance block. It always
// suggests the sync workflow first, then surfaces up to three other
// detected CLIs as `import <name>` hints so users discover the rest of
// the migration path without rereading the docs.
//
// justImported is the source name the caller passed to runImport; it
// is filtered out of the suggested list so users do not see their own
// CLI echoed back.
func printImportNextSteps(root, justImported string) {
	summaryf("\n")
	summaryf("next steps:\n")
	summaryf("  agnostic-ai sync --check   # preview what changes\n")
	summaryf("  agnostic-ai sync           # write to configured targets\n")

	detected := detectExistingTargets(root)
	hints := make([]string, 0, len(detected))
	for _, d := range detected {
		if d == justImported {
			continue
		}
		hints = append(hints, d)
	}
	if len(hints) == 0 {
		return
	}
	shown := hints
	extra := 0
	if len(hints) > 3 {
		shown = hints[:3]
		extra = len(hints) - 3
	}
	for _, h := range shown {
		summaryf("  also detected %s/ - run 'agnostic-ai import %s' to import it\n", h, h)
	}
	if extra > 0 {
		summaryf("  (and %d more detected targets)\n", extra)
	}
}

// stringSliceFromAny coerces a YAML-unmarshaled `any` value into a
// []string, dropping non-string elements. Returns nil for missing or
// non-list values.
func stringSliceFromAny(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
