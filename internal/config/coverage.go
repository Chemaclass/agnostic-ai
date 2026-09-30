package config

import (
	"fmt"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// CoverageConfig holds project-wide decisions about sync coverage notes.
type CoverageConfig struct {
	// FailOnNotes fails sync, in every mode, on each coverage note that
	// names a target and is not accepted.
	FailOnNotes bool `yaml:"fail-on-notes,omitempty" json:"fail-on-notes,omitempty"`
	// Accept lists the coverage notes the project already decided on.
	// sync stops printing an accepted note and FailOnNotes ignores it.
	Accept []CoverageAccept `yaml:"accept,omitempty" json:"accept,omitempty"`
}

// CoverageAccept marks coverage notes as known and accepted. Exactly one
// of Field, Via, and Surface names the note: Field a "has no effect"
// note for that field, whatever its reason; Via a "reaches <target> only
// via" note with that hint; Surface a "but not <surface>" note.
type CoverageAccept struct {
	Target  CoverageTargets `yaml:"target"            json:"target"`
	Kind    string          `yaml:"kind"              json:"kind" jsonschema:"enum=agents,enum=skills,enum=rules,enum=hooks,enum=mcps,enum=commands,enum=settings,enum=reviews,enum=environments,enum=ignores"`
	Field   string          `yaml:"field,omitempty"   json:"field,omitempty"`
	Via     string          `yaml:"via,omitempty"     json:"via,omitempty"`
	Surface string          `yaml:"surface,omitempty" json:"surface,omitempty"`
	Reason  string          `yaml:"reason"            json:"reason"`
}

// CoverageTargets is one target name or a list of them.
type CoverageTargets []string

// UnmarshalYAML reads a single name as a one-item list.
func (t *CoverageTargets) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		*t = CoverageTargets{node.Value}
		return nil
	}
	var names []string
	if err := node.Decode(&names); err != nil {
		return fmt.Errorf("target: want a name or a list of names: %w", err)
	}
	*t = names
	return nil
}

// coverageKinds maps each coverage.accept kind to its spec kind.
var coverageKinds = map[string]string{
	"agents": "agent", "skills": "skill", "rules": "rule", "hooks": "hook",
	"mcps": "mcp", "commands": "command", "settings": "settings",
	"reviews": "review", "environments": "environment", "ignores": "ignore",
}

// SpecKind returns the spec kind the entry's kind names.
func (a CoverageAccept) SpecKind() string { return coverageKinds[a.Kind] }

// String names the entry the way lint reports it: targets, kind, and the
// note it selects.
func (a CoverageAccept) String() string {
	s := strings.Join(a.Target, ", ") + " " + a.Kind
	switch {
	case a.Field != "":
		s += " `" + a.Field + "`"
	case a.Via != "":
		s += " via `" + a.Via + "`"
	case a.Surface != "":
		s += " on `" + a.Surface + "`"
	}
	return s
}

// Validate rejects an accept entry without a target or reason, with a
// kind no spec has, without exactly one of field, via, and surface, or
// that repeats an earlier entry for one of its targets, naming source.
// Target names are checked against the adapters by the caller.
func (c CoverageConfig) Validate(source string) error {
	seen := map[string]int{}
	for i, a := range c.Accept {
		if len(a.Target) == 0 || slices.ContainsFunc(a.Target, func(t string) bool { return strings.TrimSpace(t) == "" }) {
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
		selectors := 0
		for _, s := range []string{a.Field, a.Via, a.Surface} {
			if s != "" {
				selectors++
			}
		}
		if selectors != 1 {
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: set one of field, via, or surface to name the note", source, i)
		}
		if strings.TrimSpace(a.Reason) == "" {
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: reason is required, so the decision stays on record", source, i)
		}
		for _, t := range a.Target {
			key := strings.Join([]string{t, a.Kind, a.Field, a.Via, a.Surface}, "\x00")
			if prior, dup := seen[key]; dup {
				return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: duplicates coverage.accept[%d] for %s", source, i, prior, t)
			}
			seen[key] = i
		}
	}
	return nil
}
