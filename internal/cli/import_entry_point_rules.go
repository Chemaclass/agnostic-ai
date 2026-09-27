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
	_, sections := splitH2Sections(reduceToGeneratedRules(string(data)))
	dstDir := filepath.Join(root, cfg.Sources.Rules)
	count := 0
	for _, s := range sections {
		if !mergedDocWrapperHeadings[s.slug] {
			continue
		}
		children, _ := unwrapMergedH3Children(s.body, map[string]int{})
		for _, c := range children {
			path := filepath.Join(dstDir, c.slug+".md")
			if fileExists(path) {
				continue
			}
			if err := writeRule(path, c.slug, c.body); err != nil {
				return err
			}
			count++
		}
	}
	if count > 0 {
		summaryf("imported %d rules from %s, where %s reads them\n", count, name, source)
	}
	return nil
}
