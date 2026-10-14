package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintDuplicateKeys flags a spec that writes one top-level key more than
// once (LINT046, error). The loader keeps the last value, so sync and
// validate pass while every other YAML parser rejects the file and an
// earlier value is silently lost.
func lintDuplicateKeys(entries []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, e := range entries {
		if len(e.DuplicateKeys) == 0 {
			continue
		}
		keys := make([]string, len(e.DuplicateKeys))
		for i, key := range e.DuplicateKeys {
			keys[i] = "`" + key + "`"
		}
		out = append(out, lintFinding{
			Code:     "LINT046",
			Severity: lintError,
			Path:     e.Path,
			Message:  fmt.Sprintf("%s written more than once; the last value wins and other YAML parsers reject the file, so delete the repeats", strings.Join(keys, ", ")),
		})
	}
	return out
}
