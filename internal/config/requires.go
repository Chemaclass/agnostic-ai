package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// Requirement is the set of agnostic-ai releases a config's `requires`
// key accepts. It holds one or more space-separated terms that must all
// hold: `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, and `=X.Y.Z` or a bare `X.Y.Z`
// for one exact release.
type Requirement struct {
	terms []term
}

type release [3]int

type comparison string

const (
	atLeast comparison = ">="
	below   comparison = "<"
	atMost  comparison = "<="
	exactly comparison = "="
)

type term struct {
	op comparison
	at release
}

var (
	termRE    = regexp.MustCompile(`^(>=|<=|<|=)?\s*v?(\d+)\.(\d+)\.(\d+)`)
	releaseRE = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
)

// ParseRequirement reads a `requires` value such as ">=0.69.0", "0.73.0",
// or ">=0.73.0 <0.74.0".
func ParseRequirement(s string) (Requirement, error) {
	rest := strings.TrimSpace(s)
	var r Requirement
	for rest != "" {
		m := termRE.FindStringSubmatch(rest)
		if m == nil {
			return Requirement{}, badRequirement(s)
		}
		op := comparison(m[1])
		if op == "" {
			op = exactly
		}
		r.terms = append(r.terms, term{op: op, at: parseRelease(m[2:])})
		rest = rest[len(m[0]):]
		if rest != "" && !unicode.IsSpace(rune(rest[0])) {
			return Requirement{}, badRequirement(s)
		}
		rest = strings.TrimSpace(rest)
	}
	if len(r.terms) == 0 {
		return Requirement{}, badRequirement(s)
	}
	return r, nil
}

func badRequirement(s string) error {
	return fmt.Errorf("%q is not a version requirement; write \">=X.Y.Z\", an exact \"X.Y.Z\", or a range such as \">=X.Y.Z <X.Y.Z\", separated by spaces", s)
}

// Allows reports whether version meets every term. release is false for
// a build that is not an X.Y.Z release, such as a dev or pre-release
// build, whose place in the order is unknown; allowed is then false too.
func (r Requirement) Allows(version string) (allowed, release bool) {
	m := releaseRE.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return false, false
	}
	v := parseRelease(m[1:])
	for _, t := range r.terms {
		if !t.holds(v) {
			return false, true
		}
	}
	return true, true
}

// Above reports whether version exceeds a requirement that accepts a stable release.
func (r Requirement) Above(version string) bool {
	m := releaseRE.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return false
	}
	v := parseRelease(m[1:])
	var floor release
	for _, t := range r.terms {
		if (t.op == atLeast || t.op == exactly) && compareReleases(t.at, floor) > 0 {
			floor = t.at
		}
	}
	for _, t := range r.terms {
		if !t.holds(floor) {
			return false
		}
	}
	if compareReleases(v, floor) < 0 {
		return false
	}
	for _, t := range r.terms {
		if !t.holds(v) {
			return true
		}
	}
	return false
}

func (t term) holds(v release) bool {
	c := compareReleases(v, t.at)
	switch t.op {
	case atLeast:
		return c >= 0
	case below:
		return c < 0
	case atMost:
		return c <= 0
	default:
		return c == 0
	}
}

// InstallTarget names the release to install for a binary outside the
// requirement. latest is true when a minimum alone is unmet, so the
// newest release fits. Otherwise version is the lowest release the terms
// allow, and empty when they set only an upper bound.
func (r Requirement) InstallTarget() (version string, latest bool) {
	var floor *release
	onlyMinimums := true
	for _, t := range r.terms {
		if t.op != atLeast {
			onlyMinimums = false
		}
		if t.op == atLeast || t.op == exactly {
			if floor == nil || compareReleases(t.at, *floor) > 0 {
				at := t.at
				floor = &at
			}
		}
	}
	if onlyMinimums {
		return "", true
	}
	if floor == nil {
		return "", false
	}
	return formatRelease(*floor), false
}

// String writes the requirement in its canonical form: a lone exact
// release bare, every other term with its operator.
func (r Requirement) String() string {
	if len(r.terms) == 1 && r.terms[0].op == exactly {
		return formatRelease(r.terms[0].at)
	}
	parts := make([]string, 0, len(r.terms))
	for _, t := range r.terms {
		parts = append(parts, string(t.op)+formatRelease(t.at))
	}
	return strings.Join(parts, " ")
}

func compareReleases(a, b release) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func formatRelease(v release) string {
	return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
}

// parseRelease converts three digit-only parts the regexps matched.
func parseRelease(parts []string) release {
	var v release
	for i, p := range parts {
		v[i], _ = strconv.Atoi(p)
	}
	return v
}

// validateRequires rejects a `requires` value ParseRequirement cannot
// read. source names the config file or files it came from.
func validateRequires(requires, source string) error {
	if requires == "" {
		return nil
	}
	if _, err := ParseRequirement(requires); err != nil {
		return errs.Coded(errs.CodeConfigDecode, "%s: requires: %w", source, err)
	}
	return nil
}
