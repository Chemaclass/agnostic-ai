package cli

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
)

// syncedSharedBody reports whether data, the entry point at path, is one
// sync wrote from the current AGNOSTIC_AI.md and nothing else: it carries
// the provenance header, holds no inlined rules block, and its body is
// the view sync renders for that file. Importing it would only slice the
// shared body into rules and copy it back over its own source. fenced
// reports whether AGNOSTIC_AI.md holds `::target` fences.
func syncedSharedBody(root, path, data string) (synced, fenced bool) {
	if !header.Has(data) || strings.Contains(data, adapters.RulesStartMarker) {
		return false, false
	}
	raw, err := os.ReadFile(filepath.Join(root, agnosticMainFile))
	if err != nil {
		return false, false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false, false
	}
	source := header.Strip(string(raw))
	body := header.Strip(adapters.StripGeneratedAppendices(data))
	return matchesRenderedView(root, rel, source, body), strings.Contains(source, "::target")
}

// syncedRuleHasSource reports whether srcPath, a rule file under a
// native rules directory at rel, still holds the bytes the last sync
// wrote and its source rule exists under dstDir. Copying it back would
// only restate that source in the target's own frontmatter, and for a
// rule with `scope: <dir>` frontmatter, whose source sits at the top of
// dstDir while sync writes it to `<dir>/<name>.md`, add a second copy.
func syncedRuleHasSource(srcPath, rel, dstDir string) bool {
	data, err := os.ReadFile(srcPath)
	if err != nil || !header.Has(string(data)) || !unchangedSinceSync(srcPath, string(data)) {
		return false
	}
	if _, err := os.Stat(filepath.Join(dstDir, rel)); err == nil {
		return true
	}
	source, err := os.ReadFile(filepath.Join(dstDir, filepath.Base(rel)))
	if err != nil {
		return false
	}
	fm, _, ok := splitFrontmatter(source)
	if !ok {
		return false
	}
	var meta struct {
		Scope string `yaml:"scope"`
	}
	if yaml.Unmarshal(fm, &meta) != nil || meta.Scope == "" ||
		filepath.ToSlash(filepath.Dir(rel)) != filepath.ToSlash(filepath.Clean(meta.Scope)) {
		return false
	}
	return true
}

// unchangedSinceSync reports whether data, the content of path, is what
// the last sync recorded writing there. The ledger keys paths relative
// to the working directory, where sync and import both run.
func unchangedSinceSync(path, data string) bool {
	rel := path
	if filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return false
		}
		if rel, err = filepath.Rel(wd, path); err != nil {
			return false
		}
	}
	sums := readStateFile(".").OutputSums
	sum, ok := sums[rel]
	if !ok {
		sum = sums[filepath.ToSlash(rel)]
	}
	return sum != "" && adapters.ContentSum(data) == sum
}
