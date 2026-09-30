package spec

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Protect decisions for a settings spec's `protected` block.
const (
	ProtectAsk  = "ask"
	ProtectDeny = "deny"
)

// ProtectGroup is the `protected` block of one settings spec: paths an
// agent must not edit without asking (ask) or at all (deny).
//
// Paths are globs anchored at the project root: `*` and `?` stay inside
// one path segment and `**` crosses segments. A path also covers every
// file under it, so `vendor` protects `vendor/a.go`. The grammar is the
// part of gitignore that Claude Code permission rules and the generated
// Codex hook both read the same way, which is why character classes,
// braces, and negation are rejected rather than translated.
type ProtectGroup struct {
	Source   string
	Paths    []string
	Decision string
	Reason   string
}

var protectKeys = []string{"paths", "decision", "reason"}

// ProtectedPaths reads the `protected` block of each settings spec, in
// source order. Specs without one are skipped.
func ProtectedPaths(settings []Entry) ([]ProtectGroup, error) {
	var groups []ProtectGroup
	for _, entry := range settings {
		raw, ok := entry.Meta["protected"]
		if !ok {
			continue
		}
		group, err := parseProtectGroup(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: protected: %w", entry.Path, err)
		}
		group.Source = entry.Path
		groups = append(groups, group)
	}
	return groups, nil
}

func parseProtectGroup(raw any) (ProtectGroup, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return ProtectGroup{}, fmt.Errorf("want a mapping with paths, decision, and reason")
	}
	for key := range fields {
		if !slices.Contains(protectKeys, key) {
			return ProtectGroup{}, fmt.Errorf("unknown key %q; it takes %s", key, strings.Join(protectKeys, ", "))
		}
	}
	group := ProtectGroup{Decision: ProtectAsk}
	list, _ := fields["paths"].([]any)
	if len(list) == 0 {
		return ProtectGroup{}, fmt.Errorf("paths must list at least one path")
	}
	for _, item := range list {
		path, ok := item.(string)
		if !ok {
			return ProtectGroup{}, fmt.Errorf("path %v is not a string", item)
		}
		normalized, err := normalizeProtectPath(path)
		if err != nil {
			return ProtectGroup{}, err
		}
		if !slices.Contains(group.Paths, normalized) {
			group.Paths = append(group.Paths, normalized)
		}
	}
	if raw, set := fields["decision"]; set {
		decision, _ := raw.(string)
		if decision != ProtectAsk && decision != ProtectDeny {
			return ProtectGroup{}, fmt.Errorf("decision %v is not ask or deny", raw)
		}
		group.Decision = decision
	}
	if raw, set := fields["reason"]; set {
		reason, ok := raw.(string)
		if !ok {
			return ProtectGroup{}, fmt.Errorf("reason %v is not a string", raw)
		}
		group.Reason = strings.Join(strings.Fields(reason), " ")
	}
	return group, nil
}

func normalizeProtectPath(path string) (string, error) {
	p := strings.TrimPrefix(path, "./")
	switch {
	case strings.TrimSpace(p) == "":
		return "", fmt.Errorf("path %q is empty", path)
	case strings.HasPrefix(p, "//"), strings.HasPrefix(p, "~"):
		return "", fmt.Errorf("path %q leaves the project; paths are relative to the project root", path)
	case strings.HasPrefix(p, "!"):
		return "", fmt.Errorf("path %q is a negation, which protected paths do not take", path)
	}
	if i := strings.IndexFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f || strings.ContainsRune(`\[]{}()`, r) }); i >= 0 {
		return "", fmt.Errorf("path %q uses %q; protected paths take *, **, and ? only", path, p[i])
	}
	p = strings.TrimPrefix(p, "/")
	if strings.HasSuffix(p, "/") {
		p += "**"
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return "", fmt.Errorf("path %q must be a plain path inside the project", path)
		}
	}
	return p, nil
}

// ProtectEditRule is the permission rule for a normalized protected
// path. The leading `/` anchors it at the project root, as Claude Code
// reads a rule in project settings. It lives here because import must
// recognize the rules sync wrote.
func ProtectEditRule(path string) string {
	return "Edit(/" + path + ")"
}

// ProtectPatternRegexp translates a normalized protected path into an
// anchored extended regular expression. Go's regexp and POSIX awk read
// the result the same way, so the lint check and the generated Codex
// hook agree on what a path covers.
func ProtectPatternRegexp(pattern string) string {
	var out strings.Builder
	out.WriteString("^")
	segments := strings.Split(pattern, "/")
	for i, segment := range segments {
		last := i == len(segments)-1
		if segment == "**" {
			if last {
				out.WriteString(".+")
			} else {
				out.WriteString("(.*/)?")
			}
			continue
		}
		for _, r := range segment {
			switch r {
			case '*':
				out.WriteString("[^/]*")
			case '?':
				out.WriteString("[^/]")
			case '.', '+', '|', '^', '$':
				out.WriteByte('\\')
				out.WriteRune(r)
			default:
				out.WriteRune(r)
			}
		}
		if !last {
			out.WriteString("/")
		}
	}
	out.WriteString("$")
	return out.String()
}

// Match reports the first path of g that covers the project-relative
// file path, either directly or through one of its parent directories.
// Case is ignored, as the Codex hook ignores it: a checkout on a
// case-insensitive file system opens `.GITHUB/ci.yml` as `.github/ci.yml`.
func (g ProtectGroup) Match(file string) (string, bool) {
	file = strings.ToLower(file)
	candidates := []string{file}
	for dir := file; strings.Contains(dir, "/"); {
		dir = dir[:strings.LastIndex(dir, "/")]
		candidates = append(candidates, dir)
	}
	for _, pattern := range g.Paths {
		re := regexp.MustCompile(strings.ToLower(ProtectPatternRegexp(pattern)))
		if slices.ContainsFunc(candidates, re.MatchString) {
			return pattern, true
		}
	}
	return "", false
}
