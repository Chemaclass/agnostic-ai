package cli

import (
	"fmt"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// lintDuplicateKeys flags a spec that writes one top-level key more than
// once (LINT046, warning). The loader keeps the last value, so sync and
// validate pass while other YAML parsers reject the file and an earlier
// value is silently lost. It is a warning because 0.83.0 imports wrote
// repeated `workspaces` keys into projects that still sync.
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
		subject, lines := "key "+keys[0]+" is", "the extra line"
		if len(keys) > 1 {
			subject, lines = "keys "+strings.Join(keys, ", ")+" are", "the extra lines"
		}
		out = append(out, lintFinding{
			Code:     "LINT046",
			Severity: lintWarn,
			Path:     e.Path,
			Message:  fmt.Sprintf("%s written more than once; sync uses the last value and other YAML parsers reject the file. Remove %s to fix it", subject, lines),
		})
	}
	return out
}
