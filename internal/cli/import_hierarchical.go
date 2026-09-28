package cli

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// hierarchicalFile pairs a discovered main markdown file with its
// inferred scope. globs is "" for root, "<dir>/**" for nested files.
type hierarchicalFile struct {
	path  string
	globs string
}

// findHierarchicalMainFiles walks root for every file named filename.
// Hidden directories, common vendor trees, the project's own agnostic
// source dirs, and what importTree leaves out are skipped so unrelated
// copies (vendored projects, clones, worktrees) do not slip in. Used by
// codex / gemini and any other importer with subtree-scoped main files.
// Callers read each match through readEntryFile.
func findHierarchicalMainFiles(root, filename string, src config.Sources) ([]hierarchicalFile, error) {
	var out []hierarchicalFile
	skipDirs := map[string]bool{"vendor": true}
	for _, p := range []string{src.Agents, src.Skills, src.Rules, src.Hooks, src.MCPs} {
		if p != "" {
			skipDirs[firstSegment(p)] = true
		}
	}
	tree := importTreeFor(root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || skipDirs[name] || tree.skipsDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != filename {
			return nil
		}
		dir := filepath.ToSlash(filepath.Dir(rel))
		var globs string
		if dir != "." {
			globs = dir + "/**"
		}
		out = append(out, hierarchicalFile{path: path, globs: globs})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, nil
}

// hierarchicalRulesText returns the part of main file f that holds
// rules: the rules block sync appended, or the whole file when it has
// none. ok is false for a root file without that block: it is the
// shared instructions mirrorMainFile copies into AGNOSTIC_AI.md, and
// importing it as rules too would make sync write its text twice.
func hierarchicalRulesText(f hierarchicalFile, raw string) (text string, ok bool) {
	if f.globs == "" && !strings.Contains(raw, adapters.RulesStartMarker) {
		return "", false
	}
	return reduceToGeneratedRules(raw), true
}

// sectionsPreamble returns the text above the first ## of a main file,
// or "" when that is only a title and the intro line older syncs wrote.
// It becomes a rule of its own so a section split does not drop it.
func sectionsPreamble(text string) string {
	preamble, _ := splitH2Sections(text)
	rest := mergedDocTitleRE.ReplaceAllString(preamble, "")
	if strings.TrimSpace(mergedDocPreambleRE.ReplaceAllString(rest, "")) == "" {
		return ""
	}
	return preamble
}

// wholeFileRuleNames names the rule a whole main file becomes, keyed by
// its globs. A nested file takes its scope's last directory when no
// other scope ends the same way, the whole scope path otherwise (`api`,
// `services-api`); the root file takes the project's name.
func wholeFileRuleNames(root string, files []hierarchicalFile) map[string]string {
	last := map[string]int{}
	for _, f := range files {
		if f.globs != "" {
			last[slugify(path.Base(strings.TrimSuffix(f.globs, "/**")))]++
		}
	}
	names := map[string]string{"": projectSlug(root)}
	for _, f := range files {
		if f.globs == "" {
			continue
		}
		scope := strings.TrimSuffix(f.globs, "/**")
		name := slugify(path.Base(scope))
		if last[name] > 1 || name == "" {
			name = slugify(scope)
		}
		if name == "" {
			name = "scoped"
		}
		names[f.globs] = name
	}
	return names
}

// writeScopedRule writes a rule spec with optional `globs:` frontmatter
// into dstDir/name.md. Used by importers that infer scope from a
// hierarchical source layout.
func writeScopedRule(dstDir, name, globs, body string) error {
	var fm strings.Builder
	fm.WriteString("---\nname: " + name + "\n")
	if globs != "" {
		fm.WriteString(yamlFrontmatterLine("globs", globs))
		fm.WriteString(yamlFrontmatterLine("scope", strings.TrimSuffix(globs, "/**")))
	}
	fm.WriteString("---\n\n")
	fm.WriteString(strings.TrimRight(body, "\n"))
	fm.WriteString("\n")
	path := filepath.Join(dstDir, name+".md")
	if err := importWriteFile(path, []byte(fm.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
