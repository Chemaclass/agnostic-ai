package config

import (
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// CoverageConfig holds project-wide decisions about sync coverage notes.
type CoverageConfig struct {
	// Accept lists the coverage notes the project already decided on.
	// sync stops printing an accepted note and on-unsupported: error
	// ignores it.
	Accept []CoverageAccept `yaml:"accept,omitempty" json:"accept,omitempty"`
}

// CoverageAccept marks one coverage note as known and accepted. Field
// names the attribute of a "has no effect" note; left out, the entry
// matches the target's notes about the whole kind instead.
type CoverageAccept struct {
	Target string `yaml:"target"          json:"target"`
	Kind   string `yaml:"kind"            json:"kind" jsonschema:"enum=agents,enum=skills,enum=rules,enum=hooks,enum=mcps,enum=commands,enum=settings,enum=reviews,enum=environments,enum=ignores"`
	Field  string `yaml:"field,omitempty" json:"field,omitempty"`
	Reason string `yaml:"reason"          json:"reason"`
}

// coverageKinds maps each coverage.accept kind to its spec kind.
var coverageKinds = map[string]string{
	"agents": "agent", "skills": "skill", "rules": "rule", "hooks": "hook",
	"mcps": "mcp", "commands": "command", "settings": "settings",
	"reviews": "review", "environments": "environment", "ignores": "ignore",
}

// SpecKind returns the spec kind the entry's kind names.
func (a CoverageAccept) SpecKind() string { return coverageKinds[a.Kind] }

// String names the entry the way lint reports it: target, kind, and
// field when set.
func (a CoverageAccept) String() string {
	s := a.Target + " " + a.Kind
	if a.Field != "" {
		s += " `" + a.Field + "`"
	}
	return s
}

// Validate rejects an accept entry without a target or reason, or with
// a kind no spec has, naming source.
func (c CoverageConfig) Validate(source string) error {
	for i, a := range c.Accept {
		if strings.TrimSpace(a.Target) == "" {
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: target is required", source, i)
		}
		if _, ok := coverageKinds[a.Kind]; !ok {
			kinds := make([]string, 0, len(coverageKinds))
			for k := range coverageKinds {
				kinds = append(kinds, k)
			}
			slices.Sort(kinds)
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: unknown kind %q (want one of %s)", source, i, a.Kind, strings.Join(kinds, ", "))
		}
		if strings.TrimSpace(a.Reason) == "" {
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: reason is required, so the decision stays on record", source, i)
		}
	}
	return nil
}
