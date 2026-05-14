package cli

import (
	"fmt"
	"io/fs"
	"os"
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
// Hidden directories, common vendor trees, and the project's own
// agnostic source dirs are skipped so unrelated copies (vendored
// projects, scaffolds) do not slip in. Used by codex / gemini and
// any other importer with subtree-scoped main files.
func findHierarchicalMainFiles(root, filename string, src config.Sources) ([]hierarchicalFile, error) {
	var out []hierarchicalFile
	skipDirs := map[string]bool{"node_modules": true, "vendor": true}
	for _, p := range []string{src.Agents, src.Skills, src.Rules, src.Hooks, src.MCPs} {
		if p != "" {
			skipDirs[firstSegment(p)] = true
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || skipDirs[name]) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != filename {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
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
		fm.WriteString("globs: " + globs + "\n")
	}
	fm.WriteString("---\n\n")
	fm.WriteString(strings.TrimRight(body, "\n"))
	fm.WriteString("\n")
	path := filepath.Join(dstDir, name+".md")
	if err := os.WriteFile(path, []byte(fm.String()), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
