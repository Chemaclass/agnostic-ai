package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// malformedGlobs returns the keys of r that hold a `globs` value that
// is neither a string nor a list of strings: `globs` and each
// `x-<target>.globs`, sorted. Adapters read such a value as no globs,
// so the rule would load in every session.
func malformedGlobs(r spec.Entry) []string {
	var keys []string
	if g, ok := r.Meta["globs"]; ok && !spec.ValidGlobs(g) {
		keys = append(keys, "globs")
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

// lintMalformedGlobs flags each malformed globs value (LINT013, error).
func lintMalformedGlobs(rules []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, r := range rules {
		for _, key := range malformedGlobs(r) {
			out = append(out, lintFinding{Code: "LINT013", Severity: lintError, Path: r.Path, Message: malformedGlobsMessage(key)})
		}
	}
	return out
}
