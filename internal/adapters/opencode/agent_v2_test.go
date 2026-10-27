package opencode

import (
	"os"
	"path/filepath"
	"reflect"
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

func readAgentMeta(t *testing.T, path string) (map[string]any, string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := spec.ParseMarkdownBytes(spec.KindAgent, data)
	if err != nil {
		t.Fatal(err)
	}
	return entry.Meta, string(data)
}

func TestEmit_Agent_V2MovesTemperatureIntoRequestBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		meta map[string]any
		want map[string]any
	}{
		{"portable temperature with permissions", map[string]any{"temperature": 0.2, "x-opencode": map[string]any{"permissions": []any{}}}, map[string]any{"body": map[string]any{"temperature": 0.2}}},
		{"x-opencode temperature with disabled", map[string]any{"x-opencode": map[string]any{"temperature": 0.3, "disabled": false}}, map[string]any{"body": map[string]any{"temperature": 0.3}}},
		{"merged with request", map[string]any{"temperature": 0.4, "x-opencode": map[string]any{"request": map[string]any{"headers": map[string]any{"x-agent": "reviewer"}, "body": map[string]any{"top_p": 0.9}}}}, map[string]any{"headers": map[string]any{"x-agent": "reviewer"}, "body": map[string]any{"top_p": 0.9, "temperature": 0.4}}},
		{"request body temperature wins", map[string]any{"temperature": 0.4, "x-opencode": map[string]any{"request": map[string]any{"body": map[string]any{"temperature": 0.1}}}}, map[string]any{"body": map[string]any{"temperature": 0.1}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: tc.meta, Body: "Review."}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			meta, data := readAgentMeta(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
			if _, present := meta["temperature"]; present {
				t.Errorf("V2 file keeps top-level temperature, which OpenCode ignores:\n%s", data)
			}
			if got := meta["request"]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("request = %#v, want %#v\n%s", got, tc.want, data)
			}
		})
	}
}

func TestEmit_Agent_V2RejectsNonObjectRequestWithTemperature(t *testing.T) {
	testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"temperature": 0.2, "x-opencode": map[string]any{"request": "fast"}}, Body: "Review."}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err == nil {
		t.Error("want an error for a request that is not an object")
	}
}

func TestEmit_Agent_V1KeepsTopLevelTemperature(t *testing.T) {
	dir := testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"temperature": 0.4, "tools": []any{"Read"}}, Body: "Review."}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	meta, data := readAgentMeta(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
	if meta["temperature"] != 0.4 {
		t.Errorf("temperature = %v, want 0.4\n%s", meta["temperature"], data)
	}
	if _, present := meta["request"]; present {
		t.Errorf("V1 file gains request, which would switch it to V2:\n%s", data)
	}
}
