package cli

import (
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// portableHookIssues reports a hook spec in the portable form that is
// invalid, or that a target in targets it reaches cannot express.
func portableHookIssues(e spec.Entry, targets []string) []validationIssue {
	if e.Kind != spec.KindHook || !spec.IsPortableHook(e.Meta) {
		return nil
	}
	if problem := spec.PortableHookProblem(e.Meta); problem != "" {
		return []validationIssue{{Path: e.Path, Field: "on", Message: problem}}
	}
	var out []validationIssue
	for _, t := range targets {
		if !spec.TranslatesPortableHooks(t) || !e.EmitsTo(t) {
			continue
		}
		if _, reason := e.NativeHook(t); reason != "" {
			out = append(out, validationIssue{Path: e.Path, Field: "match", Message: reason + "; add target-exclude: " + t + " or use event:"})
		}
	}
	return out
}

// lintPortableHooks reports the portable hook issues validate reports
// (LINT032, error).
func lintPortableHooks(hooks []spec.Entry, targets []string) []lintFinding {
	var out []lintFinding
	for _, h := range hooks {
		for _, issue := range portableHookIssues(h, targets) {
			out = append(out, lintFinding{Code: "LINT032", Severity: lintError, Path: issue.Path, Message: issue.Message})
		}
	}
	return out
}
