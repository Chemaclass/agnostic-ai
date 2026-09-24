package cli

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// scopedRulesDirs returns every project sub-directory under root
// holding its own copy of rulesDir, sorted, root excluded. Shared by
// antigravityScopedRulesDirs and windsurfScopedRulesDirs (#1114,
// #1123): both targets accept a rule scoped to any project
// sub-directory, so import has to scan the whole tree for one instead
// of trusting a fixed list of candidate paths.
//
// Only `.git` (never a legitimate scope, and large enough that walking
// it is wasted work), the target's own always-unscoped output subtrees
// (ownOutputSubtrees), and agnostic-ai's own configured source
// directories are pruned. Pruning matches the exact root-relative path,
// never a bare directory name at any depth: an earlier draft of both
// callers skipped every directory named after a source root's first
// segment (or, for windsurf, a hardcoded `node_modules`/`vendor` list
// plus any hidden directory), so a legitimate scope that happened to
// share one of those names, or that sat inside one, never imported
// back (#1114, #1123).
func scopedRulesDirs(root, rulesDir string, ownOutputSubtrees map[string]bool, src config.Sources) ([]string, error) {
	skipDirs := map[string]bool{".git": true}
	for k := range ownOutputSubtrees {
		skipDirs[k] = true
	}
	for _, p := range []string{src.Agents, src.Skills, src.Rules, src.Hooks, src.MCPs} {
		if p != "" {
			skipDirs[filepath.ToSlash(filepath.Clean(p))] = true
		}
	}
	var scopes []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// This walker now descends into directories an earlier
			// draft pruned outright (node_modules, vendor, every
			// hidden directory), since CheckScopePath accepts any of
			// them as a scope name (#1123). That means a directory
			// this scan has no reason to care about, and no
			// permission over, can still turn up mid-walk: a
			// restrictive mode, a broken symlink, a directory removed
			// between listing and reading. Aborting the whole scan on
			// that alone left every scope discovered after it
			// unimported, with the root-level rules already written
			// by the time this ran, so import completed with source
			// specs only partly reconstructed (#1124 review). Warn
			// and skip past the one directory instead. `path == root`
			// is the one case worth still failing on: it means the
			// project root itself could not be read, which is not a
			// stray unrelated directory but the scan having nothing
			// to scan at all.
			summaryf("  ! skipping unreadable %s while scanning for scoped rules: %v\n", path, walkErr)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			if path == root {
				return walkErr
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if skipDirs[rel] {
			return fs.SkipDir
		}
		if !dirExists(filepath.Join(path, rulesDir)) {
			return nil
		}
		scopes = append(scopes, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s for scoped rules dirs: %w", root, err)
	}
	sort.Strings(scopes)
	return scopes, nil
}

// rulesDirFromCfg returns the project-relative `outputs.<target>.rules-dir`
// path when configured, otherwise "". Shared by antigravityRulesDirFromCfg
// and windsurfRulesDirFromCfg: the target name was the only difference
// between the two (#1123 ref pass).
func rulesDirFromCfg(cfg *config.Config, target string) string {
	if cfg == nil {
		return ""
	}
	if o, ok := cfg.Outputs[target]; ok {
		return o.RulesDir
	}
	return ""
}
