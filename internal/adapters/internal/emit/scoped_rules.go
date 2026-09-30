package emit

import (
	"fmt"
	"path"
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

// ScopedDocuments lists the files target writes inside a scope directory,
// relative to it: a directory-context instruction file, and for Cursor the
// Bugbot review file. Every other scoped output lives in a tool directory.
func ScopedDocuments(cfg *config.Config, target string) []string {
	var docs []string
	if doc := scopeDocument(target); doc != "" {
		docs = append(docs, doc)
	}
	if target == "goose" && filepath.Base(OutputRulesFile(cfg, target, "")) == ".goosehints" {
		docs = append(docs, ".goosehints")
	}
	if target == "cursor" {
		docs = append(docs, ".cursor/"+OutputReviewFile(cfg, target, "BUGBOT.md"))
	}
	return docs
}

func hasScopeFilters(target string) bool {
	switch target {
	case "claude", "cursor", "copilot", "cline", "windsurf", "continue", "kiro", "trae", "qoder", "openhands", "antigravity":
		return true
	default:
		return false
	}
}

// PrepareScopedRules separates directory documents from adapter-native rules.
// The input has already been filtered and expanded for this target. It never
// mutates source metadata, so concurrent target emissions remain independent.
func PrepareScopedRules(b spec.Bundle, cfg *config.Config, target string) (spec.Bundle, []CapturedFile, error) {
	return PrepareScopedDocuments(b, cfg, target, nil)
}

// PrepareScopedDocuments is PrepareScopedRules that also writes each
// scope's review section, from ReviewSections, into that scope's
// AGENTS.md, creating the file when no rule lands there.
func PrepareScopedDocuments(b spec.Bundle, cfg *config.Config, target string, reviews map[string]string) (spec.Bundle, []CapturedFile, error) {
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
		// Cline's empty paths list disables activation and cannot widen scope.
		if target == "cline" && emptySelectorList(r.Meta["paths"]) {
			out.Rules = append(out.Rules, original)
			continue
		}
		if target == "continue" && emptySelectorList(r.Meta["globs"]) {
			if err := unsupportedScope(cfg, target, r, "empty native globs cannot safely express a directory scope"); err != nil {
				return out, nil, err
			}
			continue
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
		if _, err := scopeDirectories(patterns); target == "cursor" && err == nil {
			for _, peer := range cfg.Targets {
				if scopeDocument(peer) == "AGENTS.md" && r.EmitsTo(peer) {
					doc = "AGENTS.md"
					break
				}
			}
		}
		if doc != "" {
			directories, err := scopeDirectories(patterns)
			if err != nil {
				if err := unsupportedScope(cfg, target, r, err.Error()); err != nil {
					return out, nil, err
				}
				continue
			}
			if o := cfg.Outputs[target]; o.File != "" || o.RulesDir != "" {
				return out, nil, fmt.Errorf("%s: %s: remove file/rules-dir overrides to use native scoped %s", r.Path, target, doc)
			}
			for _, directory := range directories {
				if err := CheckScopePath(directory); err != nil {
					return out, nil, fmt.Errorf("%s: %w", r.Path, err)
				}
				path := filepath.Join(directory, doc)
				grouped[path] = append(grouped[path], r)
			}
			continue
		}
		// "trigger" stays out of this exclusion: unlike the file-matching
		// keys below, it is an activation-mode override
		// (antigravity's x-antigravity.trigger, honored first in
		// ruleTrigger before any glob/alwaysApply check runs) rather
		// than a competing selector, so keeping it does not reopen the
		// portable scope contract those keys guard. Qoder's own
		// ruleMarkdown strips a scoped entry's "trigger" independently
		// at render time, so this is not qoder's only guard (#1114
		// review: a scoped antigravity `manual` rule otherwise lost
		// its override here and re-derived as `glob` from the scope's
		// own forced globs on the very next sync).
		custom, _ := CustomTargetMeta(original.Meta, target, "scope", "paths", "globs", "alwaysApply", "applyTo", "fileMatchPattern", "inclusion", "glob", "regex")
		if len(custom) > 0 {
			r.Meta["x-"+target] = custom
		}
		r.Meta["paths"] = patterns
		nativeGlobs := r.Meta["globs"]
		r.Meta["globs"] = strings.Join(patterns, ",")
		r.Meta["alwaysApply"] = false
		// Native activation keys must not override the portable scope contract.
		for _, k := range []string{"applyTo", "fileMatchPattern", "inclusion", "trigger", "glob", "regex"} {
			delete(r.Meta, k)
		}
		if target == "continue" {
			delete(r.Meta, "alwaysApply")
			switch nativeGlobs.(type) {
			case []string, []any:
				r.Meta["globs"] = patterns
			default:
				if len(patterns) > 1 {
					r.Meta["globs"] = patterns
				}
			}
		}
		for _, k := range []string{"paths", "globs", "alwaysApply"} {
			if _, ok := r.Meta[k]; ok && !containsKey(r.MetaKeys, k) {
				r.MetaKeys = append(r.MetaKeys, k)
			}
		}
		out.Rules = append(out.Rules, r)
	}
	sections, err := scopedReviewSections(cfg, target, reviews, grouped)
	if err != nil {
		return out, nil, err
	}
	paths := make([]string, 0, len(grouped)+len(sections))
	for p := range grouped {
		paths = append(paths, p)
	}
	for p := range sections {
		if _, ok := grouped[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	files := make([]CapturedFile, 0, len(paths))
	for _, p := range paths {
		files = append(files, CapturedFile{Path: p, Content: scopedDocument(grouped[p], sections[p])})
	}
	return out, files, nil
}

// scopedReviewSections keys the scoped review sections by the AGENTS.md
// path target writes them to. A target writes a review-only AGENTS.md
// when that file is its own scope document; a target that writes a
// shared AGENTS.md only for its rules, such as cursor, adds the section
// to those files alone.
func scopedReviewSections(cfg *config.Config, target string, reviews map[string]string, grouped map[string][]spec.Entry) (map[string]string, error) {
	owns := scopeDocument(target) == "AGENTS.md" && OutputRulesFile(cfg, target, "") == ""
	out := map[string]string{}
	for scope, section := range reviews {
		if scope == "" {
			continue
		}
		path := filepath.Join(scope, "AGENTS.md")
		if _, ok := grouped[path]; !ok && !owns {
			continue
		}
		if o := cfg.Outputs[target]; o.ProvenanceHeader != nil && !*o.ProvenanceHeader {
			return nil, fmt.Errorf("%s: %s: scoped review sections require provenance-header for safe ownership", path, target)
		}
		if err := CheckScopePath(scope); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out[path] = section
	}
	return out, nil
}

func emptySelectorList(value any) bool {
	switch list := value.(type) {
	case []string:
		return len(list) == 0
	case []any:
		return len(list) == 0
	default:
		return false
	}
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

// A scope adds its whole directory to the selector union.
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
			patterns = spec.GlobList(v)
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
		for _, p := range patterns {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if strings.HasPrefix(p, "/") || strings.HasPrefix(p, "!") || strings.ContainsAny(p, "\\:\r\n\x00") {
				return nil, fmt.Errorf("cannot combine %s %q with scope: use positive project-relative patterns", key, p)
			}
			for _, part := range strings.Split(p, "/") {
				if part == ".." {
					return nil, fmt.Errorf("cannot combine %s %q with scope: parent traversal is not allowed", key, p)
				}
			}
			p = path.Clean(p)
			if p == "**/*" {
				p = "**"
			}
			if strings.HasSuffix(p, "/**/*") {
				p = strings.TrimSuffix(p, "/*")
			}
			result = append(result, p)
		}
	}
	sort.Strings(result)
	unique := make([]string, 0, len(result))
	for _, p := range result {
		if len(unique) > 0 && unique[len(unique)-1] == p {
			continue
		}
		covered := false
		for _, other := range result {
			if other == p {
				continue
			}
			if selectorCovers(other, p) {
				covered = true
				break
			}
		}
		if !covered {
			unique = append(unique, p)
		}
	}
	return unique, nil
}

func selectorCovers(pattern, other string) bool {
	if pattern == "**" {
		return true
	}
	if !strings.HasSuffix(pattern, "/**") {
		return false
	}
	directory := strings.TrimSuffix(pattern, "/**")
	return !strings.ContainsAny(directory, "*?[]{}!,()") && strings.HasPrefix(other, directory+"/")
}

func scopeDirectories(patterns []string) ([]string, error) {
	directories := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		if !strings.HasSuffix(pattern, "/**") || strings.ContainsAny(strings.TrimSuffix(pattern, "/**"), "*?[]{}!,()") {
			return nil, fmt.Errorf("native directory instructions cannot preserve selector %q; use a complete directory pattern such as tests/a/**", pattern)
		}
		directory, err := spec.NormalizeScope(strings.TrimSuffix(pattern, "/**"))
		if err != nil || directory == "" {
			return nil, fmt.Errorf("native directory instructions cannot preserve selector %q: use a project subdirectory", pattern)
		}
		directories = append(directories, directory)
	}
	return directories, nil
}

// scopedDocument renders one scoped AGENTS.md: the rules block, then the
// review section. Either may be empty. A directory with one rule gets
// that rule's text as written, inside the sentinel markers: the file is
// the rule, so a "## Rules" and a "### <name>" heading would only add
// words to a hand-written file that import brought in whole.
func scopedDocument(rules []spec.Entry, reviewSection string) string {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Name < rules[j].Name })
	content := ""
	switch len(rules) {
	case 0:
	case 1:
		content = RulesStartMarker + "\n\n" + SourceComment(rules[0].Path)
		if d := rules[0].Description(); d != "" {
			content += "_" + d + "_\n\n"
		}
		content += rules[0].Body + "\n\n" + RulesEndMarker + "\n"
	default:
		var body strings.Builder
		for _, r := range rules {
			WriteSection(&body, r.Name, r)
		}
		content = wrapRulesBlock(body.String())
	}
	if reviewSection != "" {
		if content != "" {
			content += "\n"
		}
		content += reviewSection
	}
	return WithHeader(content, FormatMarkdown)
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
// reviews are the ReviewSections every AGENTS.md reader writes.
func CheckScopeReaders(bundles map[string]spec.Bundle, files map[string]CapturedFile, targets []string, reviews map[string]string) error {
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
					if s == "" {
						continue
					}
					patterns, err := scopePatterns(r, s)
					if err != nil {
						if s != scope {
							continue
						}
						return fmt.Errorf("%s: shared instructions cannot preserve %s's file filters: %w", path, target, err)
					}
					applies := s == scope
					for _, pattern := range patterns {
						applies = applies || pattern == "**" || strings.HasPrefix(pattern, scope+"/")
					}
					if !applies {
						continue
					}
					directories, err := scopeDirectories(patterns)
					if err != nil {
						return fmt.Errorf("%s: shared instructions cannot preserve %s's file filters; use compatible selectors or separate worktrees", path, target)
					}
					for _, directory := range directories {
						if directory == scope {
							expected = append(expected, r)
							break
						}
					}
				}
				section := reviews[scope]
				if len(expected) == 0 && section == "" || scopedDocument(expected, section) != file.Content {
					return fmt.Errorf("%s: shared instructions differ for %s; use the same target conditions and bodies or separate worktrees", path, target)
				}
			}
		}
	}
	return nil
}
