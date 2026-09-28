package adapters

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// EntryPointRules is the root-context projection of the shared scope contract.
func EntryPointRules(b spec.Bundle, target string) spec.Bundle {
	return emit.EntryPointRules(b, target)
}

// ScopedDocuments lists the files target writes inside a scope directory,
// relative to it.
func ScopedDocuments(cfg *config.Config, target string) []string {
	return emit.ScopedDocuments(cfg, target)
}

// ValidateScopedRules preflights scope output against all configured readers.
// Unlike byte collision policy, semantic scope conflicts cannot use last-wins.
func ValidateScopedRules(cfg *config.Config, b spec.Bundle, requested []string) error {
	hasScope := false
	for _, r := range b.Rules {
		if _, exists := r.Meta["scope"]; r.Scope != "" || exists {
			hasScope = true
			break
		}
		for k := range r.Meta {
			if strings.HasPrefix(k, "x-") {
				hasScope = true
			}
		}
	}
	if !hasScope {
		return nil
	}
	set := map[string]bool{}
	for _, t := range cfg.Targets {
		set[t] = true
	}
	for _, t := range requested {
		set[t] = true
	}
	targets := make([]string, 0, len(set))
	for t := range set {
		targets = append(targets, t)
	}
	sort.Strings(targets)
	shared := map[string]emit.CapturedFile{}
	resolved := make(map[string]spec.Bundle, len(targets))
	prepared := make(map[string]spec.Bundle, len(targets))
	planned := make(map[string][]emit.CapturedFile, len(targets))
	for _, target := range targets {
		resolved[target] = expandBundleVars(b.For(target), cfg, target)
		p, files, err := emit.PrepareScopedRules(resolved[target], cfg, target)
		if err != nil {
			return err
		}
		prepared[target], planned[target] = p, files
	}
	replaced := map[string]bool{}
	if set["claude"] {
		for _, p := range companionPaths(cfg, prepared["claude"].Rules) {
			replaced[p] = true
		}
	}
	for _, target := range targets {
		for _, f := range planned[target] {
			// sync never writes a user-owned path, so neither content
			// conflicts nor the reader checks on shared files apply to it.
			if cfg.IsUnmanaged(f.Path) {
				continue
			}
			if prior, ok := shared[f.Path]; ok && prior.Content != f.Content {
				return fmt.Errorf("%s: incompatible target-specific scoped instructions; use identical content or separate worktrees", f.Path)
			}
			shared[f.Path] = f
			if err := emit.CheckScopedDestination(f.Path, f.Content, replaced); err != nil {
				return err
			}
		}
		var scoped []spec.Entry
		for _, r := range prepared[target].Rules {
			if r.EffectiveScope() != "" {
				scoped = append(scoped, r)
			}
		}
		if len(scoped) == 0 {
			continue
		}
		if target == "cursor" && set["crush"] {
			return fmt.Errorf("scoped Cursor rules conflict with crush, which reads .cursor/rules unconditionally; use separate worktrees or remove an incompatible target")
		}
		a, ok := Get(target)
		if !ok {
			continue
		}
		sess := NewSession()
		// User-owned destinations drop out of the capture, so sync never
		// writes them and the ownership check below never sees them.
		sess.SetUnmanaged(cfg.Sync.Unmanaged)
		sess.StartCapture()
		if err := a.Emit(sess, spec.Bundle{Rules: scoped}, cfg, false); err != nil {
			sess.StopCapture()
			return err
		}
		for _, f := range sess.StopCapture() {
			// Every scoped artifact must remain inside the project. Existing native
			// rule files also need ownership checks before replacing hand-written text.
			if err := emit.CheckScopePath(f.Path); err != nil {
				return err
			}
			ext := filepath.Ext(f.Path)
			if ext == ".md" || ext == ".mdc" {
				if err := emit.CheckScopedDestination(f.Path, f.Content, replaced); err != nil {
					return err
				}
			}
		}
	}
	return emit.CheckScopeReaders(resolved, shared, targets)
}

// AgentsCompanionPaths returns the nested CLAUDE.md companions sync
// removes when targets includes claude (see emit.AgentsCompanionDirs).
func AgentsCompanionPaths(cfg *config.Config, b spec.Bundle, targets []string) ([]string, error) {
	if !slices.Contains(targets, "claude") {
		return nil, nil
	}
	prepared, _, err := emit.PrepareScopedRules(expandBundleVars(b.For("claude"), cfg, "claude"), cfg, "claude")
	if err != nil {
		return nil, err
	}
	return companionPaths(cfg, prepared.Rules), nil
}

func companionPaths(cfg *config.Config, rules []spec.Entry) []string {
	dirs := emit.AgentsCompanionDirs(cfg, rules)
	paths := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		paths = append(paths, path.Join(dir, emit.AgentsCompanionFile))
	}
	return paths
}
