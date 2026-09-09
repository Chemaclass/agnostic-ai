package emit

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// scopeDocument names the native instruction file for directory-context hosts.
// Other verified hosts use their existing rule renderer after normalization.
func scopeDocument(target string) string {
	switch target {
	case "codex", "amp", "warp", "opencode", "goose", "augment", "factory", "kilo":
		return "AGENTS.md"
	case "gemini":
		return "GEMINI.md"
	default:
		return ""
	}
}

func hasScopeFilters(target string) bool {
	switch target {
	case "claude", "cursor", "copilot", "cline", "windsurf", "continue", "kiro", "trae", "qoder", "openhands":
		return true
	default:
		return false
	}
}

// PrepareScopedRules separates directory documents from adapter-native rules.
// The input has already been filtered and expanded for this target. It never
// mutates source metadata, so concurrent target emissions remain independent.
func PrepareScopedRules(b spec.Bundle, cfg *config.Config, target string) (spec.Bundle, []CapturedFile, error) {
	out := b
	out.Rules = nil
	grouped := map[string][]spec.Entry{}
	for _, r := range b.Rules {
		original := r
		r.Meta, r.MetaKeys = ResolveMetaOrdered(r.Meta, r.MetaKeys, target)
		scope, err := spec.RuleScope(r)
		if err != nil {
			return out, nil, err
		}
		if scope == "" {
			out.Rules = append(out.Rules, original)
			continue
		}
		r.Scope = scope
		if r.Meta == nil {
			r.Meta = make(map[string]any)
		}
		if o := cfg.Outputs[target]; o.ProvenanceHeader != nil && !*o.ProvenanceHeader {
			return out, nil, fmt.Errorf("%s: %s: scoped rules require provenance-header for safe ownership", r.Path, target)
		}
		if err := CheckScopePath(scope); err != nil {
			return out, nil, fmt.Errorf("%s: %w", r.Path, err)
		}
		doc := scopeDocument(target)
		if doc == "" && !hasScopeFilters(target) {
			if err := unsupportedScope(cfg, target, r, "no verified native directory or file-scoped instructions"); err != nil {
				return out, nil, err
			}
			continue
		}
		patterns, err := scopePatterns(r, scope)
		if err != nil {
			if err = unsupportedScope(cfg, target, r, err.Error()); err != nil {
				return out, nil, err
			}
			continue
		}
		if OutputRulesFile(cfg, target, "") != "" {
			if target == "goose" && filepath.Base(OutputRulesFile(cfg, target, "")) == ".goosehints" {
				doc = ".goosehints"
			} else {
				return out, nil, fmt.Errorf("%s: %s: scoped rules cannot use outputs.%s.rules-file; remove the legacy override", r.Path, target, target)
			}
		}
		// Cursor reads shared nested AGENTS.md natively. Prefer that shared copy
		// when its configured peers already require one for this same directory.
		if target == "cursor" && len(patterns) == 1 && patterns[0] == scope+"/**" {
			for _, peer := range cfg.Targets {
				if scopeDocument(peer) == "AGENTS.md" && r.EmitsTo(peer) {
					doc = "AGENTS.md"
					break
				}
			}
		}
		if doc != "" {
			if len(patterns) != 1 || patterns[0] != scope+"/**" {
				if err := unsupportedScope(cfg, target, r, "native directory instructions cannot preserve narrower file filters"); err != nil {
					return out, nil, err
				}
				continue
			}
			if o := cfg.Outputs[target]; o.File != "" || o.RulesDir != "" {
				return out, nil, fmt.Errorf("%s: %s: remove file/rules-dir overrides to use native scoped %s", r.Path, target, doc)
			}
			path := filepath.Join(scope, doc)
			grouped[path] = append(grouped[path], r)
			continue
		}
		if len(patterns) > 1 && target != "claude" && target != "cline" && target != "qoder" && target != "openhands" {
			if err := unsupportedScope(cfg, target, r, "multiple intersected patterns are not supported by this renderer; use one rule per pattern"); err != nil {
				return out, nil, err
			}
			continue
		}
		custom, _ := CustomTargetMeta(original.Meta, target, "scope", "paths", "globs", "alwaysApply", "applyTo", "fileMatchPattern", "inclusion", "trigger", "glob", "regex")
		if len(custom) > 0 {
			r.Meta["x-"+target] = custom
		}
		r.Meta["paths"] = patterns
		r.Meta["globs"] = strings.Join(patterns, ",")
		r.Meta["alwaysApply"] = false
		// Native activation keys must not override the portable scope contract.
		for _, k := range []string{"applyTo", "fileMatchPattern", "inclusion", "trigger", "glob", "regex"} {
			delete(r.Meta, k)
		}
		if target == "continue" {
			delete(r.Meta, "alwaysApply")
		}
		for _, k := range []string{"paths", "globs", "alwaysApply"} {
			if _, ok := r.Meta[k]; ok && !containsKey(r.MetaKeys, k) {
				r.MetaKeys = append(r.MetaKeys, k)
			}
		}
		out.Rules = append(out.Rules, r)
	}
	paths := make([]string, 0, len(grouped))
	for p := range grouped {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	files := make([]CapturedFile, 0, len(paths))
	for _, p := range paths {
		rules := grouped[p]
		sort.SliceStable(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
		files = append(files, CapturedFile{Path: p, Content: scopedDocumentBody(rules)})
	}
	return out, files, nil
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}

func unsupportedScope(cfg *config.Config, target string, r spec.Entry, reason string) error {
	message := fmt.Sprintf("%s: %s: scoped rule %q skipped (%s); use a supported target or simplify its selectors", r.Path, target, r.Name, reason)
	switch cfg.OnUnsupported {
	case OnUnsupportedError:
		return fmt.Errorf("%s", message)
	case OnUnsupportedSilent:
		return nil
	default:
		NoteCoverageGap(target, spec.KindRule, 1, message)
		return nil
	}
}

// scopePatterns handles portable conjunctions without guessing at glob algebra.
// A literal subtree prefix proves a narrower pattern remains inside the scope.
func scopePatterns(r spec.Entry, scope string) ([]string, error) {
	bound := scope + "/**"
	result := []string{bound}
	for _, key := range []string{"regex", "applyTo", "fileMatchPattern", "glob"} {
		if _, exists := r.Meta[key]; exists {
			return nil, fmt.Errorf("use portable paths or globs instead of %s with scope", key)
		}
	}
	for _, key := range []string{"paths", "globs"} {
		raw, exists := r.Meta[key]
		if !exists {
			continue
		}
		var patterns []string
		switch v := raw.(type) {
		case string:
			if v != "" {
				patterns = strings.Split(v, ",")
			}
		case []string:
			patterns = v
		case []any:
			for _, x := range v {
				p, ok := x.(string)
				if !ok {
					return nil, fmt.Errorf("%s must contain strings", key)
				}
				patterns = append(patterns, p)
			}
		default:
			return nil, fmt.Errorf("%s must be a string or string list", key)
		}
		if len(patterns) == 0 {
			continue
		}
		constrained := []string{}
		for _, p := range patterns {
			p = strings.TrimSpace(p)
			if p == "**" || p == "**/*" {
				constrained = []string{bound}
				break
			}
			if p == bound || p == scope+"/**/*" {
				constrained = []string{bound}
				break
			}
			if strings.HasPrefix(p, scope+"/") && !strings.Contains(p, "..") && !strings.ContainsAny(p, "{}!,\\") {
				constrained = append(constrained, p)
				continue
			}
			// An explicit ancestor subtree is another safe intersection.
			if strings.HasSuffix(p, "/**") && strings.HasPrefix(scope+"/", strings.TrimSuffix(p, "**")) {
				constrained = []string{bound}
				break
			}
			return nil, fmt.Errorf("cannot intersect %s %q with scope %q", key, p, scope)
		}
		sort.Strings(constrained)
		if len(result) == 1 && result[0] == bound {
			result = constrained
			continue
		}
		if len(constrained) == 1 && constrained[0] == bound {
			continue
		}
		if strings.Join(result, "\x00") != strings.Join(constrained, "\x00") {
			return nil, fmt.Errorf("paths and globs have different intersections; use one selector")
		}
	}
	return result, nil
}

func scopedDocumentBody(rules []spec.Entry) string {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	var body strings.Builder
	for _, r := range rules {
		WriteSection(&body, r.Name, r)
	}
	return WithHeader(wrapRulesBlock(body.String()), FormatMarkdown)
}

// EntryPointRules uses the same scope and target resolution as native emission.
func EntryPointRules(b spec.Bundle, target string) spec.Bundle {
	b = b.For(target)
	rules := make([]spec.Entry, 0, len(b.Rules))
	for _, r := range b.Rules {
		resolved := r
		resolved.Meta = ResolveMeta(r.Meta, target)
		scope, err := spec.RuleScope(resolved)
		if err == nil && scope == "" {
			rules = append(rules, r)
		}
	}
	b.Rules = rules
	return b
}

// CheckScopeReaders validates known cross-tool instruction discovery collisions.
// It considers configured readers even during a partial sync.
func CheckScopeReaders(bundles map[string]spec.Bundle, files map[string]CapturedFile, targets []string) error {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		file := files[path]
		if filepath.Base(path) == "AGENTS.md" {
			if _, exists := files[filepath.Join(filepath.Dir(path), ".goosehints")]; exists {
				return fmt.Errorf("%s: shared AGENTS.md and .goosehints would duplicate scoped context; remove the legacy Goose rules-file override", path)
			}
			scope := filepath.ToSlash(filepath.Dir(path))
			for _, target := range targets {
				if target == "kiro" {
					return fmt.Errorf("%s: scoped AGENTS.md conflicts with kiro, which includes nested AGENTS.md globally; use separate worktrees or remove an incompatible target", path)
				}
				if scopeDocument(target) != "AGENTS.md" && target != "cursor" && target != "copilot" && target != "windsurf" {
					continue
				}
				var expected []spec.Entry
				for _, r := range bundles[target].Rules {
					r.Meta = ResolveMeta(r.Meta, target)
					s, err := spec.RuleScope(r)
					if err != nil {
						return err
					}
					if s != scope {
						continue
					}
					patterns, err := scopePatterns(r, s)
					if err != nil || len(patterns) != 1 || patterns[0] != s+"/**" {
						return fmt.Errorf("%s: shared instructions cannot preserve %s's file filters; use compatible selectors or separate worktrees", path, target)
					}
					expected = append(expected, r)
				}
				if len(expected) == 0 || scopedDocumentBody(expected) != file.Content {
					return fmt.Errorf("%s: shared instructions differ for %s; use the same target conditions and bodies or separate worktrees", path, target)
				}
			}
		}
	}
	return nil
}
