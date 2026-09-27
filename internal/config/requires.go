package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
)

// Requirement is the oldest agnostic-ai release a config's `requires`
// key accepts, written as `>=X.Y.Z`.
type Requirement struct {
	min release
}

type release [3]int

var (
	requirementRE = regexp.MustCompile(`^>=\s*v?(\d+)\.(\d+)\.(\d+)$`)
	releaseRE     = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)
)

// ParseRequirement reads a `requires` value such as ">=0.69.0".
func ParseRequirement(s string) (Requirement, error) {
	m := requirementRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Requirement{}, fmt.Errorf("%q is not a minimum version; write it as \">=X.Y.Z\", such as \">=0.69.0\"", s)
	}
	return Requirement{min: parseRelease(m[1:])}, nil
}

// Allows reports whether version is at or above the minimum. release is
// false for a build that is not an X.Y.Z release, such as a dev or
// pre-release build, whose place in the order is unknown; allowed is
// then false too.
func (r Requirement) Allows(version string) (allowed, release bool) {
	m := releaseRE.FindStringSubmatch(strings.TrimSpace(version))
	if m == nil {
		return false, false
	}
	v := parseRelease(m[1:])
	for i := range v {
		if v[i] != r.min[i] {
			return v[i] > r.min[i], true
		}
	}
	return true, true
}

func (r Requirement) String() string {
	return fmt.Sprintf(">=%d.%d.%d", r.min[0], r.min[1], r.min[2])
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
