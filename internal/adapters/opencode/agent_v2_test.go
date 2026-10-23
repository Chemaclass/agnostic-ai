package opencode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_Agent_V2TriggersPreserveRestrictions(t *testing.T) {
	for _, trigger := range []struct {
		key   string
		value any
	}{
		{"disabled", false}, {"system", "Native prompt"}, {"system", ""}, {"request", map[string]any{}}, {"permissions", []any{}},
	} {
		t.Run(trigger.key, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			if err := os.WriteFile("opencode.json", []byte(`{"permission":{"bash":"ask"},"permissions":[{"action":"read","resource":"secrets/*","effect":"deny"},{"action":"read","resource":"secrets/public/*","effect":"ask"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read", "Bash", "Task"}, "x-opencode": map[string]any{trigger.key: trigger.value}}, Body: "Review."}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			rules := readNativeAgentRules(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			for _, probe := range []simRule{{"read", "src/main.go", "allow"}, {"read", "secrets/token", "deny"}, {"read", "secrets/public/info", "ask"}, {"shell", "git status", "ask"}, {"subagent", "review", "allow"}, {"edit", "src/main.go", "deny"}} {
				if got := simEvaluate(rules, probe.key, probe.pattern, false); got != probe.action {
					t.Errorf("%s %s = %s, want %s; rules %v", probe.key, probe.pattern, got, probe.action, rules)
				}
			}
		})
	}
}

func TestEmit_Agent_V2KeepsNativePermissionOrder(t *testing.T) {
	dir := testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}, "x-opencode": map[string]any{"disabled": false, "permission": map[string]any{"read": map[string]any{"secrets/public/*": "allow", "secrets/*": "deny"}}, "permissions": []any{
		map[string]any{"action": "read", "resource": "secrets/*", "effect": "ask"}, map[string]any{"action": "read", "resource": "secrets/public/*", "effect": "deny"},
	}}}, NestedKeys: map[string][]string{"x-opencode\x00permission": {"read"}, "x-opencode\x00permission\x00read": {"secrets/public/*", "secrets/*"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	rules := readNativeAgentRules(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
	want := []simRule{{"read", "secrets/public/*", "allow"}, {"read", "secrets/*", "deny"}, {"read", "secrets/*", "ask"}, {"read", "secrets/public/*", "deny"}}
	if len(rules) < len(want) {
		t.Fatalf("rules %v", rules)
	}
	tail := rules[len(rules)-len(want):]
	for i := range want {
		if tail[i] != want[i] {
			t.Errorf("rule %d = %v, want %v", i, tail[i], want[i])
		}
	}
}

func readNativeAgentRules(t *testing.T, path string) []simRule {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := spec.ParseMarkdownBytes(spec.KindAgent, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := entry.Meta["permission"]; ok {
		t.Fatalf("V2 file retains ignored legacy permission:\n%s", data)
	}
	raw, ok := entry.Meta["permissions"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatalf("V2 permissions missing:\n%s", data)
	}
	var rules []simRule
	for _, r := range raw {
		m := r.(map[string]any)
		rules = append(rules, simRule{m["action"].(string), m["resource"].(string), m["effect"].(string)})
	}
	return rules
}

func TestEmit_Agent_V2NativeOverridesKeepScalarAndAliases(t *testing.T) {
	for _, tc := range []struct {
		name       string
		permission any
		want       []simRule
	}{
		{"scalar", "ask", []simRule{{"*", "*", "ask"}}},
		{"patch alias", map[string]any{"apply_patch": "deny"}, []simRule{{"edit", "*", "deny"}}},
		{"ordered map", map[string]any{"bash": "ask", "task": "deny"}, []simRule{{"shell", "*", "ask"}, {"subagent", "*", "deny"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}, "x-opencode": map[string]any{"disabled": false, "permission": tc.permission}}}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			rules := readNativeAgentRules(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			for _, want := range tc.want {
				if got := simEvaluate(rules, want.key, want.pattern, false); got != want.action {
					t.Errorf("%v: got %s in %v", want, got, rules)
				}
			}
		})
	}
}
