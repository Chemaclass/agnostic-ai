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
			return walkErr
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
