package spec

import (
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// formatYAMLError wraps a yaml.v3 parse error in a `path:line:col: msg`
// envelope so editor and CI output can navigate straight to the
// offending byte. lineOffset is the 1-based line number on which the
// yaml block starts inside the source file (0 for pure YAML files; 1
// for markdown frontmatter, where the `---` opener occupies line 1 and
// yaml content begins on line 2).
func formatYAMLError(path string, err error, lineOffset int) error {
	if err == nil {
		return nil
	}
	msgs := yamlErrorMessages(err)
	if len(msgs) == 0 {
		return errs.Coded(errs.CodeSpecParse, "%s: %s", path, err.Error())
	}
	line, col, body := extractPosition(msgs[0])
	if line > 0 {
		line += lineOffset
	} else {
		line = 1
	}
	if col <= 0 {
		col = 1
	}
	return errs.Coded(errs.CodeSpecParse, "%s:%d:%d: %s", path, line, col, body)
}

// yamlErrorMessages flattens a yaml.v3 error chain into individual
// human-readable strings. yaml.TypeError carries multiple lines in its
// Errors slice; everything else is a single-string error.
func yamlErrorMessages(err error) []string {
	if te, ok := err.(*yaml.TypeError); ok && len(te.Errors) > 0 {
		return te.Errors
	}
	return []string{err.Error()}
}

var (
	reLineCol = regexp.MustCompile(`^yaml: line (\d+): column (\d+): (.+)$`)
	reLine    = regexp.MustCompile(`^yaml: line (\d+): (.+)$`)
)

// extractPosition pulls (line, col, message) out of a yaml.v3 error
// string. Returns (0, 0, raw) when the format is not recognized so
// callers fall back to a default position rather than emit "0:0".
func extractPosition(msg string) (line, col int, body string) {
	msg = strings.TrimSpace(msg)
	if m := reLineCol.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
		col, _ = strconv.Atoi(m[2])
		return line, col, m[3]
	}
	if m := reLine.FindStringSubmatch(msg); m != nil {
		line, _ = strconv.Atoi(m[1])
		return line, 0, m[2]
	}
	return 0, 0, msg
}
