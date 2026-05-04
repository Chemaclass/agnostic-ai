package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type stringBuilder struct{ b strings.Builder }

func (s *stringBuilder) Builder() *strings.Builder { return &s.b }
func (s *stringBuilder) String() string            { return s.b.String() }

func entryFixture() spec.Entry {
	return spec.Entry{
		Kind: spec.KindRule,
		Name: "x",
		Path: "rules/x.md",
		Meta: map[string]any{"description": "a description"},
		Body: "body content",
	}
}

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }
