package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// config.Output is one struct shared by every target, so an adapter that
// never reads a field still accepts it in `agnostic-ai.yaml` and writes
// to the default path. That silence is how `outputs.claude.agents-dir`
// went unread while `{{agents_dir}}` resolved from it. This test holds
// each target page's documented keys against what sync actually writes.

var outputKeyProbes = map[string]struct {
	kind string
	set  func(*config.Output, string)
}{
	"agents-dir":       {"agent", func(o *config.Output, v string) { o.AgentsDir = v }},
	"skills-dir":       {"skill", func(o *config.Output, v string) { o.SkillsDir = v }},
	"commands-dir":     {"command", func(o *config.Output, v string) { o.CommandsDir = v }},
	"rules-dir":        {"rule", func(o *config.Output, v string) { o.RulesDir = v }},
	"instructions-dir": {"rule", func(o *config.Output, v string) { o.InstructionsDir = v }},
	"chatmodes-dir":    {"agent", func(o *config.Output, v string) { o.ChatmodesDir = v }},
	"workflows-dir":    {"agent", func(o *config.Output, v string) { o.WorkflowsDir = v }},
	"assistants-dir":   {"agent", func(o *config.Output, v string) { o.AssistantsDir = v }},
	"hooks-dir":        {"hook", func(o *config.Output, v string) { o.HooksDir = v }},
	"mcp-dir":          {"mcp", func(o *config.Output, v string) { o.MCPDir = v }},
	"mcp-file":         {"mcp", func(o *config.Output, v string) { o.MCPFile = v }},
	"cli-mcp-file":     {"mcp", func(o *config.Output, v string) { o.CLIMCPFile = v }},
	"root-mcp-file":    {"mcp", func(o *config.Output, v string) { o.RootMCPFile = v }},
	"hooks-file":       {"hook", func(o *config.Output, v string) { o.HooksFile = v }},
	"tasks-file":       {"hook", func(o *config.Output, v string) { o.TasksFile = v }},
	"ignore-file":      {"ignore", func(o *config.Output, v string) { o.IgnoreFile = v }},
	"rules-file":       {"rule", func(o *config.Output, v string) { o.RulesFile = v }},
	"review-file":      {"review", func(o *config.Output, v string) { o.ReviewFile = v }},
	"setup-file":       {"environment", func(o *config.Output, v string) { o.SetupFile = v }},
	"environment-file": {"environment", func(o *config.Output, v string) { o.EnvironmentFile = v }},
	"conf-file":        {"rule", func(o *config.Output, v string) { o.ConfFile = v }},
}

// valueOutputKeys are documented keys that carry a value rather than a
// path, so there is no emitted path to follow.
var valueOutputKeys = map[string]bool{
	"dir": true, "model": true, "weak-model": true, "rules-mode": true,
	"settings": true, "config": true, "exec-policies": true,
	"exec-policies-file": true, "shared-subagents": true,
	"emit-agents-as-commands": true, "emit-skills-as-commands": true,
	"collision-policy": true, "provenance-header": true,
}

// documentedNoOpKeys are keys a target page documents as no longer
// moving anything. Each entry needs the page to say so, because the
// point of the key is then to explain a path a reader may still have in
// their config.
var documentedNoOpKeys = map[string]string{
	"amp.commands-dir":       "Amp removed its file-based command surface, so nothing is emitted to point at",
	"junie.rules-dir":        "rules inline into .junie/AGENTS.md; the key only redirects the legacy-tree sweep",
	"windsurf.workflows-dir": "Devin Desktop removed Cascade, the only agent that read a Workflow file",
}

// probeKinds overrides the spec kind a key's probe uses where one
// config field serves a different surface on a different target.
// `conf-file` carries Aider's model settings out of a rule sync, but
// Devin's project permission policy out of a settings spec.
var probeKinds = map[string]string{
	"windsurf.conf-file": "settings",
	"factory.conf-file":  "settings",
}

func probeKind(target, key, fallback string) string {
	if kind, ok := probeKinds[target+"."+key]; ok {
		return kind
	}
	return fallback
}

// overrideValues holds the value to probe with where a target
// validates the shape of the path it accepts.
var overrideValues = map[string]string{
	"goose.hooks-file": "custom/hooks/hooks.json",
}

func overrideValue(target, key string) string {
	if v, ok := overrideValues[target+"."+key]; ok {
		return v
	}
	return "custom/moved"
}

func outputKeyProbeEntry(kind string) spec.Entry {
	switch kind {
	case "agent":
		return spec.Entry{Kind: spec.KindAgent, Name: "probe", Path: "agents/probe.md", Body: "b"}
	case "skill":
		return spec.Entry{Kind: spec.KindSkill, Name: "probe", Path: "skills/probe/SKILL.md", Body: "b"}
	case "command":
		return spec.Entry{Kind: spec.KindCommand, Name: "probe", Path: "commands/probe.md", Body: "b"}
	case "rule":
		return spec.Entry{Kind: spec.KindRule, Name: "probe", Path: "rules/probe.md", Body: "b"}
	case "mcp":
		return spec.Entry{Kind: spec.KindMCP, Name: "probe", Meta: map[string]any{"command": "x"}}
	case "hook":
		return spec.Entry{Kind: spec.KindHook, Name: "probe", Meta: map[string]any{"event": "PreToolUse", "command": "x"}}
	case "ignore":
		return spec.Entry{Kind: spec.KindIgnore, Name: "probe", Body: "node_modules/"}
	case "review":
		return spec.Entry{Kind: spec.KindReview, Name: "probe", Path: "reviews/probe.md", Body: "b"}
	case "settings":
		return spec.Entry{Kind: spec.KindSettings, Name: "probe", Path: "settings/probe.yaml", Meta: map[string]any{
			"model":       "probe-model",
			"permissions": map[string]any{"deny": []any{"Bash(rm:*)"}},
		}}
	case "environment":
		return spec.Entry{Kind: spec.KindEnvironment, Name: "probe", Meta: map[string]any{
			"install":   "echo hi",
			"terminals": []any{map[string]any{"name": "probe", "command": "echo hi"}},
		}}
	}
	return spec.Entry{}
}

func emittedPaths(t *testing.T, target string, cfg *config.Config, e spec.Entry) []string {
	t.Helper()
	adapter, err := adapters.Resolve(target)
	if err != nil {
		t.Fatalf("resolve %s: %v", target, err)
	}
	sess := adapters.NewSession()
	sess.StartCapture()
	if err := adapters.EmitWithProvenance(sess, adapter, spec.NewBundle([]spec.Entry{e}), cfg, true); err != nil {
		sess.StopCapture()
		t.Fatalf("%s: emit failed: %v", target, err)
	}
	var paths []string
	for _, f := range sess.StopCapture() {
		paths = append(paths, filepath.ToSlash(f.Path))
	}
	sort.Strings(paths)
	return paths
}

// documentedOutputKeys reads every `outputs.<target>.<key>` a target
// page mentions about itself.
func documentedOutputKeys(t *testing.T) map[string][]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "docs/site/content/docs/targets/*.md"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no target pages found: %v", err)
	}
	rx := regexp.MustCompile("`outputs\\.([a-z]+)\\.([a-z-]+)`")
	out := map[string][]string{}
	for _, f := range files {
		target := strings.TrimSuffix(filepath.Base(f), ".md")
		if target == "_index" {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, m := range rx.FindAllStringSubmatch(string(body), -1) {
			if m[1] != target || seen[m[2]] {
				continue
			}
			seen[m[2]] = true
			out[target] = append(out[target], m[2])
		}
		sort.Strings(out[target])
	}
	return out
}

func TestOutputKeys_DocumentedKeysMoveTheirOutput(t *testing.T) {
	for target, keys := range documentedOutputKeys(t) {
		for _, key := range keys {
			if valueOutputKeys[key] {
				continue
			}
			probe, ok := outputKeyProbes[key]
			if !ok {
				t.Errorf("%s documents outputs.%s.%s, which this test cannot probe; add it to outputKeyProbes or valueOutputKeys", target, target, key)
				continue
			}
			entry := outputKeyProbeEntry(probeKind(target, key, probe.kind))
			before := emittedPaths(t, target, &config.Config{}, entry)

			out := config.Output{}
			probe.set(&out, overrideValue(target, key))
			after := emittedPaths(t, target, &config.Config{Outputs: map[string]config.Output{target: out}}, entry)

			moved := strings.Join(before, ",") != strings.Join(after, ",")
			reason, documentedNoOp := documentedNoOpKeys[target+"."+key]
			switch {
			case moved && documentedNoOp:
				t.Errorf("outputs.%s.%s is listed as a documented no-op (%s) but now moves output: %v -> %v", target, key, reason, before, after)
			case !moved && !documentedNoOp:
				t.Errorf("%s documents outputs.%s.%s but setting it changes nothing; sync still writes %v", target, target, key, before)
			}
		}
	}
}
