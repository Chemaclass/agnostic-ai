package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importInlinedEntryPointRules recovers the rules source loads from the
// shared entry point in place of its own rule files (#1224): each rule
// in that file's rules block that has no spec yet. Rules the source's
// own files carried, with their activation, were imported first and win.
//
// The block records no `targets`, so a rule with no spec left cannot be
// told apart from one scoped to another reader of the file. It imports
// untargeted: a rule reaching more targets than before beats a rule
// dropped from every target by the next sync.
func importInlinedEntryPointRules(root, source string, cfg *config.Config) error {
	if adapters.EntryPointRuleInliner(cfg, source) == "" {
		return nil
	}
	name := adapters.EntryPointPath(cfg, source)
	data, err := readEntryFile(root, filepath.Join(root, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if !strings.Contains(string(data), adapters.RulesStartMarker) {
		return nil
	}
	specs := map[string]bool{}
	if _, b, err := loadProject(root); err == nil {
		for _, r := range b.Rules {
			specs[r.Name] = true
		}
	}
	dstDir := filepath.Join(root, cfg.Sources.Rules)
	count := 0
	for _, c := range generatedBlockRules(reduceToGeneratedRules(string(data))) {
		path := filepath.Join(dstDir, c.slug+".md")
		if specs[c.slug] || fileExists(path) {
			continue
		}
		if err := writeRule(path, c.slug, c.body); err != nil {
			return err
		}
		count++
	}
	if count > 0 {
		summaryf("imported %d rules from %s, where %s reads them\n", count, name, source)
	}
	return nil
}

// generatedBlockRules returns one rule per `###` child of the wrapper
// headings in block, the inner text of the rules block sync appends to
// an entry point.
func generatedBlockRules(block string) []mergedH3Child {
	_, sections := splitH2Sections(block)
	var out []mergedH3Child
	for _, s := range sections {
		if !mergedDocWrapperHeadings[s.slug] {
			continue
		}
		children, _ := unwrapMergedH3Children(s.body, map[string]int{})
		out = append(out, children...)
	}
	return out
}
