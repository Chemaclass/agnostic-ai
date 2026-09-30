package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ExpandsClaudeSyntax reports whether target expands syntax in a spec of
// kind the way Claude Code does (re-exported from the emit layer).
func ExpandsClaudeSyntax(target string, kind spec.Kind, syntax spec.ClaudeSyntax) bool {
	return emit.ExpandsClaudeSyntax(target, kind, syntax)
}
