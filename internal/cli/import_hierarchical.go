package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

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
	skipDirs := map[string]bool{"node_modules": true, "vendor": true}
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
