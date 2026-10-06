package cli

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/config"
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

// lintPortableHookForms names the on: and match: for each native hook
// the hooks-portable-events migration rewrites (LINT034, warn). A plan
// that fails suggests nothing, since migrate then rewrites nothing, and
// so does a project command run in the global home, where migrate needs
// --global.
func lintPortableHookForms(root string, cfg *config.Config, b spec.Bundle) []lintFinding {
	if refuseGlobalHome(root, "") != nil {
		return nil
	}
	hs, err := projectHookMigrationSpecs(root, cfg, b)
	if err != nil {
		return nil
	}
	planned, _, err := planPortableHooks(migrationScope{root: root}, hs)
	if err != nil {
		return nil
	}
	var out []lintFinding
	for _, p := range planned {
		out = append(out, lintFinding{Code: "LINT034", Severity: lintWarn, Path: p.hook.Path, Message: p.form.suggestion(p.hook.Name)})
	}
	return out
}

// suggestion names the on: and match: that replace hook name's native
// event: and matcher:.
func (f portableHookForm) suggestion(name string) string {
	native, portable := yamlPair("event", f.event), yamlPair("on", f.on)
	if f.hasMatcher {
		native += " with " + yamlPair("matcher", f.matcher)
		portable += " with " + yamlPair("match", f.match)
	}
	return fmt.Sprintf("Hook %q: %s gives every target it reaches the same native hook as %s. Run `agnostic-ai migrate --only hooks` to rewrite it",
		name, portable, native)
}

func yamlPair(key, value string) string {
	if value == "" {
		value = `""`
	}
	return "`" + key + ": " + value + "`"
}
