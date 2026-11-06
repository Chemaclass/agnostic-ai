package overview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// gapKinds are the spec kinds a project can add to reach more of what
// its tools support, in the order Context lists them. Hooks are listed
// by stage instead.
var gapKinds = []struct {
	kind              spec.Kind
	noun, one, plural string
	use               string
}{
	{spec.KindRule, "rule", "a rule", "rules", "a convention the agent should follow, such as how to test or name things"},
	{spec.KindAgent, "agent", "an agent", "agents", "a subagent with its own role, such as a reviewer"},
	{spec.KindSkill, "skill", "a skill", "skills", "a workflow the agent runs on request, such as cutting a release"},
	{spec.KindCommand, "command", "a command", "commands", "a prompt the user starts by name"},
	{spec.KindReview, "review instructions", "review instructions", "review instructions", "what a code review checks in this project"},
	{spec.KindIgnore, "ignore file", "an ignore file", "ignore files", "files the agent must not read, such as secrets or build output"},
}

// recommendedBuiltins are the built-ins init turns on for a new
// project, each with the spec kind it ships as.
var recommendedBuiltins = []struct {
	name string
	kind spec.Kind
}{{"handoff", spec.KindSkill}}

// buildGaps lists what the configured tools support and the project does
// not use yet. Gaps are suggestions, never findings. The user
// configuration has none: `agnostic-ai new` writes project specs only.
func buildGaps(in Inputs, targets []string, caps map[string][]spec.Kind) []Gap {
	gaps := []Gap{}
	if in.Global {
		return gaps
	}
	for _, k := range gapKinds {
		supported := kindSupport(k.kind, "", targets, caps).Supported
		if len(supported) == 0 || slices.ContainsFunc(entriesOf(in.Bundle, k.kind), ownSpec) {
			continue
		}
		label := kindLabels[k.kind]
		gaps = append(gaps, Gap{
			ID: "kind:" + string(k.kind), Section: SectionContext, Label: label, Supported: supported,
			Missing: "no " + k.noun + " yet",
			Fix: fmt.Sprintf("This agnostic-ai project has no %s, and %s support them. Read the project and decide whether %s would help the agent here: %s. If one would, create it with `agnostic-ai new %s <name>`, fill it in, and run `agnostic-ai sync`. If none would, say so and change nothing.",
				k.plural, strings.Join(supported, ", "), k.one, k.use, k.kind),
		})
	}
	for _, st := range lifecycleStages {
		var supported []string
		for _, t := range targets {
			if _, ok := spec.PortableHookEvent(t, st.id); ok {
				supported = append(supported, t)
			}
		}
		if len(supported) == 0 || slices.ContainsFunc(in.Bundle.Hooks, func(e spec.Entry) bool { return ownSpec(e) && hookEvent(e) == st.id }) {
			continue
		}
		gaps = append(gaps, Gap{
			ID: "stage:" + st.id, Section: SectionLifecycle, Label: st.label, Supported: supported,
			Missing: "no hook yet",
			Fix: fmt.Sprintf("In this agnostic-ai project, %s can run a hook at the %q stage and no hook runs there. %s Read the project and decide whether a hook at this point would help. If one would, create it with `agnostic-ai new hook <name>`, set `on: %s`, write its command, and run `agnostic-ai sync`. If none would, say so and change nothing.",
				strings.Join(supported, ", "), st.label, st.description, st.id),
		})
	}
	var enabled []string
	if in.Cfg != nil {
		enabled = in.Cfg.Builtins
	}
	for _, b := range recommendedBuiltins {
		supported := kindSupport(b.kind, "", targets, caps).Supported
		if len(supported) == 0 || slices.Contains(enabled, b.name) {
			continue
		}
		name := b.name
		gaps = append(gaps, Gap{
			ID: "builtin:" + name, Section: SectionSettings, Label: name, Supported: supported,
			Missing: "built-in is off",
			Fix: fmt.Sprintf("The %s built-in of this agnostic-ai project is off. %s To turn it on, add `%s` to the `builtins` list in agnostic-ai.yaml and run `agnostic-ai sync`.",
				name, builtinDescriptions[name], name),
		})
	}
	return gaps
}

// ownSpec reports whether the project wrote e. A built-in ships a fixed
// spec, so it fills no gap.
func ownSpec(e spec.Entry) bool {
	return e.Layer != builtinLayer
}
