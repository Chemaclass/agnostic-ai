package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

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
			// `path == root` has to be checked before the SkipDir
			// branch below, not after: root is always a directory, so
			// checking d.IsDir() first meant this case never ran (#1124
			// second review). An unreadable project root is not a
			// stray directory the scan can shrug off -- it means the
			// scan saw nothing at all, silently, while the root-level
			// rules import (which does not go through this walker)
			// still succeeded, so import as a whole reported success
			// with every scoped rule missing.
			if path == root {
				return walkErr
			}
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
			// and skip past the one directory instead.
			summaryf("  ! skipping unreadable %s while scanning for scoped rules: %v\n", path, walkErr)
			if d != nil && d.IsDir() {
				return fs.SkipDir
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

// preflightRulesDirs confirms every rules directory an import is about
// to read -- rulesDir at root, plus `<scope>/rulesDir` for every
// scope scopedRulesDirs already discovered -- can actually be read,
// before any of them is imported. Shared by importFromWindsurf and
// importFromAntigravity (#1124 review): discovery only Stats a
// candidate directory to confirm it exists (dirExists), which
// succeeds even when the directory's own contents cannot actually be
// read, so a scope can be recorded whose rules dir is unreadable.
// Without this preflight, the root rules dir imports first,
// overwriting its destination, and only then does a later scoped
// import reach the unreadable directory and fail, leaving the
// already-overwritten destination with no way back to its previous
// content. A missing directory is not a failure here, the same as
// importRulesDirectoryWith's own `os.Stat` + `fs.ErrNotExist` check:
// the root's own rulesDir is not guaranteed to exist yet on a project
// with no native rules at all.
//
// This checks one level deep only, `os.ReadDir` plus opening every
// `.md` file it lists, matching the exact failure this guards against
// (the directory itself unreadable). It does not recurse into a
// deeper unreadable subdirectory nested inside a rules dir; the real
// import's own walk still surfaces that case as an ordinary import
// error, just not necessarily before another destination has already
// been written.
func preflightRulesDirs(root, rulesDir string, scopes []string) error {
	dirs := make([]string, 0, len(scopes)+1)
	dirs = append(dirs, rulesDir)
	for _, scope := range scopes {
		dirs = append(dirs, filepath.Join(scope, rulesDir))
	}
	for _, d := range dirs {
		full := filepath.Join(root, d)
		entries, err := os.ReadDir(full)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("preflight %s: %w", full, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			p := filepath.Join(full, entry.Name())
			f, err := os.Open(p)
			if err != nil {
				return fmt.Errorf("preflight %s: %w", p, err)
			}
			_ = f.Close()
		}
	}
	return nil
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
