package adapters

import (
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// protectedPathsCoverage is implemented by an adapter that enforces
// protected paths natively, returning the mechanism, or that has a more
// specific reason than the default for why it cannot.
type protectedPathsCoverage interface {
	ProtectedPaths() (enforcement, reason string)
}

// ClaudeOwnedPermissionsFiles name the files under `.claude/` that
// record the permission rules sync wrote, newest first, so import skips
// them.
var ClaudeOwnedPermissionsFiles = []string{claude.OwnedPermissionsFile, claude.LegacyProtectedRulesFile}

const advisoryProtectedPathsReason = "the target has no native edit guard sync writes, so protected paths are advisory; state them in a rule"

// ProtectedPathsEnforcement returns how target enforces protected paths:
// "permission", "hook", or "" when protection there is advisory.
func ProtectedPathsEnforcement(target string) string {
	enforcement, _ := protectedPathsFor(registry[target])
	return enforcement
}

func protectedPathsFor(a Adapter) (enforcement, reason string) {
	if c, ok := a.(protectedPathsCoverage); ok {
		enforcement, reason = c.ProtectedPaths()
	}
	if enforcement == "" && reason == "" {
		reason = advisoryProtectedPathsReason
	}
	return enforcement, reason
}

// checkProtectedPaths rejects an invalid protected block on every
// target, so a project whose targets all ignore it still hears about a
// typo. A target that takes settings but does not enforce protected
// paths gets a coverage note; one that takes no settings already
// reports the whole spec as unsupported.
func checkProtectedPaths(a Adapter, settings []spec.Entry) error {
	groups, err := spec.ProtectedPaths(settings)
	if err != nil || len(groups) == 0 {
		return err
	}
	if !slices.Contains(a.Capabilities(), spec.KindSettings) {
		return nil
	}
	enforcement, reason := protectedPathsFor(a)
	if enforcement == "" {
		emit.NoteFieldNoOp(a.Name(), spec.KindSettings, "protected", len(groups), reason)
	}
	return nil
}
