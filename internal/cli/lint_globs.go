package cli

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// malformedGlobs returns the keys of r that hold a `globs` value that
// is neither a string nor a list of strings: `globs`, its `paths`
// alias, and each `x-<target>.globs`, sorted. Adapters read such a
// value as no globs, so the rule would load in every session.
func malformedGlobs(r spec.Entry) []string {
	var keys []string
	for _, key := range []string{"globs", "paths"} {
		if g, ok := r.Meta[key]; ok && !spec.ValidGlobs(g) {
			keys = append(keys, key)
		}
	}
	for k, v := range r.Meta {
		x, ok := v.(map[string]any)
		if !ok || !strings.HasPrefix(k, "x-") {
			continue
		}
		if g, ok := x["globs"]; ok && !spec.ValidGlobs(g) {
			keys = append(keys, k+".globs")
		}
	}
	sort.Strings(keys)
	return keys
}

func malformedGlobsMessage(key string) string {
	return fmt.Sprintf(`%s must be a string or a list of strings, such as globs: "*.go,*.mod" or globs: ["*.go", "*.mod"]`, key)
}

const pathsConflictGlobsMessage = "paths and globs name different patterns; keep one of them, since paths is another name for globs"

// globsIssues maps each rule selector problem in r to its message.
func globsIssues(r spec.Entry) map[string]string {
	issues := map[string]string{}
	for _, key := range malformedGlobs(r) {
		issues[key] = malformedGlobsMessage(key)
	}
	if r.PathsConflictGlobs() {
		issues["paths"] = pathsConflictGlobsMessage
	}
	return issues
}

// lintMalformedGlobs flags each malformed globs value, and paths that
// disagree with globs (LINT013, error).
func lintMalformedGlobs(rules []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, r := range rules {
		issues := globsIssues(r)
		for _, key := range slices.Sorted(maps.Keys(issues)) {
			out = append(out, lintFinding{Code: "LINT013", Severity: lintError, Path: r.Path, Message: issues[key]})
		}
	}
	return out
}
