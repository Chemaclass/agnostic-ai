package emit

import (
	"reflect"
	"testing"
)

// ResolveMetaOrdered must emit a deterministic key order: unhinted
// top-level keys and keys introduced by an x-<target> block both append
// alphabetically, not in Go map-iteration order. A non-deterministic
// order would vary the emitted frontmatter bytes per run and break
// sync --check.
func TestResolveMetaOrdered_DeterministicKeyOrder(t *testing.T) {
	t.Parallel()
	meta := map[string]any{
		"name":        "a",
		"zeta":        "z", // unhinted top-level
		"alpha":       "a", // unhinted top-level
		"mike":        "m", // unhinted top-level
		"x-claude":    map[string]any{"tango": 1, "bravo": 2, "yankee": 3},
		"description": "d",
	}
	// Only `name` is hinted; everything else must order deterministically.
	hint := []string{"name"}

	_, first := ResolveMetaOrdered(cloneMeta(meta), hint, "claude")
	for i := 0; i < 50; i++ {
		_, got := ResolveMetaOrdered(cloneMeta(meta), hint, "claude")
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("non-deterministic key order:\n run0 = %v\n run%d = %v", first, i+1, got)
		}
	}
	want := []string{"name", "alpha", "description", "mike", "zeta", "bravo", "tango", "yankee"}
	if !reflect.DeepEqual(first, want) {
		t.Errorf("key order = %v, want %v", first, want)
	}
}

func cloneMeta(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestResolveMeta_DropsOtherTargets(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":     "r1",
		"x-claude": map[string]any{"allowed-tools": []any{"Read"}},
		"x-cursor": map[string]any{"globs": "src/**"},
	}
	got := ResolveMeta(in, "claude")
	want := map[string]any{
		"name":          "r1",
		"allowed-tools": []any{"Read"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestCustomTargetMeta_ReturnsSortedNonExcludedKeys(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"description": "top-level wins",
		"x-codex": map[string]any{
			"name":        "ignored",
			"description": "ignored",
			"interface":   map[string]any{"display_name": "X"},
			"zebra":       "z",
			"alpha":       "a",
		},
	}
	got, keys := CustomTargetMeta(in, "codex", "name", "description", "interface")
	wantKeys := []string{"alpha", "zebra"}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys = %#v, want %#v", keys, wantKeys)
	}
	want := map[string]any{"alpha": "a", "zebra": "z"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestCustomTargetMeta_AbsentOrEmptyReturnsNil(t *testing.T) {
	t.Parallel()
	for _, in := range []map[string]any{
		{"name": "r1"},
		{"x-codex": map[string]any{}},
		{"x-codex": map[string]any{"only": nil}}, // nil is a non-value
	} {
		got, keys := CustomTargetMeta(in, "codex", "name")
		if got != nil || keys != nil {
			t.Errorf("CustomTargetMeta(%#v) = %#v, %#v; want nil, nil", in, got, keys)
		}
	}
}

func TestResolveMeta_TargetOverridesTopLevel(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":     "r1",
		"model":    "haiku",
		"x-claude": map[string]any{"model": "opus"},
	}
	got := ResolveMeta(in, "claude")
	if got["model"] != "opus" {
		t.Errorf("expected x-claude to override model, got %v", got["model"])
	}
}

func TestResolveMeta_NoMatchingNamespaceLeaves(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":     "r1",
		"x-cursor": map[string]any{"globs": "src/**"},
	}
	got := ResolveMeta(in, "gemini")
	want := map[string]any{"name": "r1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestResolveMeta_NilMap(t *testing.T) {
	t.Parallel()
	if got := ResolveMeta(nil, "claude"); got != nil {
		t.Errorf("expected nil pass-through, got %#v", got)
	}
}

func TestResolveMeta_StripsRoutingKeys(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":            "r1",
		"description":     "desc",
		"target":          "claude",
		"targets":         []any{"claude", "codex"},
		"target-exclude":  "gemini",
		"targets-exclude": []any{"cursor"},
	}
	got := ResolveMeta(in, "claude")
	want := map[string]any{"name": "r1", "description": "desc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}

func TestResolveMetaOrdered_StripsRoutingKeysFromOrder(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":        "r1",
		"target":      "claude",
		"description": "desc",
	}
	keys := []string{"name", "target", "description"}
	got, gotKeys := ResolveMetaOrdered(in, keys, "claude")
	wantKeys := []string{"name", "description"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("ordered keys: got %v, want %v", gotKeys, wantKeys)
	}
	if _, present := got["target"]; present {
		t.Errorf("target should be stripped, got %#v", got)
	}
}

// `x-<target>.<key>: nil` is a deletion marker — the per-target emit
// must drop the key entirely instead of keeping the top-level value
// (#304). Without this, a codex agent that never had `model:` inherited
// claude's `model: haiku` after a round-trip import.
func TestResolveMeta_NilUnderXTargetDeletesKey(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":  "agent",
		"model": "haiku",
		"x-codex": map[string]any{
			"model": nil,
		},
	}
	got := ResolveMeta(in, "codex")
	if _, present := got["model"]; present {
		t.Errorf("x-codex.model: nil should drop model, got %#v", got)
	}
	// Claude side must still see the top-level value.
	gotClaude := ResolveMeta(in, "claude")
	if gotClaude["model"] != "haiku" {
		t.Errorf("claude side lost top-level model: got %#v", gotClaude)
	}
}

// `x-<target>.<key>` with a value overrides the top-level value for
// that target while leaving every other target's view alone (#304).
func TestResolveMeta_XTargetOverridesValue(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":        "agent",
		"description": "claude-side description",
		"x-codex": map[string]any{
			"description": "codex-side description",
		},
	}
	if got := ResolveMeta(in, "claude")["description"]; got != "claude-side description" {
		t.Errorf("claude desc: got %v", got)
	}
	if got := ResolveMeta(in, "codex")["description"]; got != "codex-side description" {
		t.Errorf("codex desc: got %v", got)
	}
}

// Deletion under x-<target> must also drop the key from the ordered key
// slice so renderers that iterate `ordered` don't emit a stray entry.
func TestResolveMetaOrdered_NilUnderXTargetDropsOrder(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":  "agent",
		"model": "haiku",
		"x-codex": map[string]any{
			"model": nil,
		},
	}
	keys := []string{"name", "model"}
	_, gotKeys := ResolveMetaOrdered(in, keys, "codex")
	wantKeys := []string{"name"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("got %v, want %v", gotKeys, wantKeys)
	}
}

func TestResolveMeta_StringModelPassesThrough(t *testing.T) {
	t.Parallel()
	in := map[string]any{"name": "a", "model": "gpt-4o"}
	got := ResolveMeta(in, "codex")
	if got["model"] != "gpt-4o" {
		t.Errorf("string model should pass through, got %v", got["model"])
	}
}

func TestResolveMeta_PerTargetModelPicksTarget(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name": "a",
		"model": map[string]any{
			"claude":  "claude-opus-4-8",
			"codex":   "gpt-5.5",
			"default": "gpt-4o",
		},
	}
	if got := ResolveMeta(in, "claude")["model"]; got != "claude-opus-4-8" {
		t.Errorf("claude got %v", got)
	}
	if got := ResolveMeta(in, "codex")["model"]; got != "gpt-5.5" {
		t.Errorf("codex got %v", got)
	}
}

func TestResolveMeta_PerTargetModelFallsBackToDefault(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name": "a",
		"model": map[string]any{
			"claude":  "claude-opus-4-8",
			"default": "gpt-4o",
		},
	}
	if got := ResolveMeta(in, "cursor")["model"]; got != "gpt-4o" {
		t.Errorf("unmatched target should use default, got %v", got)
	}
}

func TestResolveMeta_PerTargetModelNoMatchNoDefaultDrops(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":  "a",
		"model": map[string]any{"claude": "claude-opus-4-8"},
	}
	got := ResolveMeta(in, "codex")
	if _, ok := got["model"]; ok {
		t.Errorf("no target match and no default should drop model, got %v", got["model"])
	}
}

func TestResolveMetaOrdered_PerTargetModelDropKeepsOrderClean(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":  "a",
		"model": map[string]any{"claude": "opus"},
	}
	keys := []string{"name", "model"}
	_, gotKeys := ResolveMetaOrdered(in, keys, "codex")
	wantKeys := []string{"name"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("got %v, want %v", gotKeys, wantKeys)
	}
}

func TestResolveMeta_XTargetOverridesPerTargetMap(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":     "a",
		"model":    map[string]any{"claude": "opus", "default": "gpt-4o"},
		"x-codex":  map[string]any{"model": "gpt-5.5-codex"},
		"x-cursor": map[string]any{"model": nil},
	}
	if got := ResolveMeta(in, "codex")["model"]; got != "gpt-5.5-codex" {
		t.Errorf("x-codex.model should win over map, got %v", got)
	}
	if got, ok := ResolveMeta(in, "cursor")["model"]; ok {
		t.Errorf("x-cursor nil delete should drop model, got %v", got)
	}
}

// A plain `effort:` scalar reaches every target untouched, in both the
// string and the bare-integer spelling. Qoder documents an integer
// budget, so the int must stay an int rather than being stringified or
// mistaken for a per-target map.
func TestResolveMeta_ScalarEffortPassesThrough(t *testing.T) {
	t.Parallel()
	if got := ResolveMeta(map[string]any{"name": "a", "effort": "high"}, "claude")["effort"]; got != "high" {
		t.Errorf("string effort should pass through, got %v", got)
	}
	got := ResolveMeta(map[string]any{"name": "a", "effort": 8000}, "qoder")["effort"]
	if n, ok := got.(int); !ok || n != 8000 {
		t.Errorf("integer effort should pass through as int, got %T %v", got, got)
	}
}

func TestResolveMeta_PerTargetEffortPicksTarget(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":   "a",
		"effort": map[string]any{"claude": "xhigh", "qoder": 8000},
	}
	if got := ResolveMeta(in, "claude")["effort"]; got != "xhigh" {
		t.Errorf("claude got %v", got)
	}
	// The pick must not narrow to strings: Qoder's budget is a number.
	got := ResolveMeta(in, "qoder")["effort"]
	if n, ok := got.(int); !ok || n != 8000 {
		t.Errorf("qoder should get the int 8000, got %T %v", got, got)
	}
}

func TestResolveMeta_PerTargetEffortFallsBackToDefault(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":   "a",
		"effort": map[string]any{"claude": "xhigh", "default": "high"},
	}
	if got := ResolveMeta(in, "qoder")["effort"]; got != "high" {
		t.Errorf("unmatched target should use default, got %v", got)
	}
}

func TestResolveMeta_PerTargetEffortNoMatchNoDefaultDrops(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":   "a",
		"effort": map[string]any{"qoder": 8000},
	}
	if got, ok := ResolveMeta(in, "claude")["effort"]; ok {
		t.Errorf("no target match and no default should drop effort, got %v", got)
	}
}

func TestResolveMetaOrdered_PerTargetEffortDropKeepsOrderClean(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":   "a",
		"effort": map[string]any{"qoder": "high"},
	}
	_, gotKeys := ResolveMetaOrdered(in, []string{"name", "effort"}, "claude")
	wantKeys := []string{"name"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("got %v, want %v", gotKeys, wantKeys)
	}
}

func TestResolveMeta_XTargetOverridesPerTargetEffort(t *testing.T) {
	t.Parallel()
	in := map[string]any{
		"name":     "a",
		"effort":   map[string]any{"claude": "xhigh", "default": "high"},
		"x-qoder":  map[string]any{"effort": "medium"},
		"x-cursor": map[string]any{"effort": nil},
	}
	if got := ResolveMeta(in, "qoder")["effort"]; got != "medium" {
		t.Errorf("x-qoder.effort should win over the map, got %v", got)
	}
	if got, ok := ResolveMeta(in, "cursor")["effort"]; ok {
		t.Errorf("x-cursor nil delete should drop effort, got %v", got)
	}
	if got := ResolveMeta(in, "claude")["effort"]; got != "xhigh" {
		t.Errorf("an untouched target keeps its map pick, got %v", got)
	}
}

// A nested map, a list, or a null under the target's own key is not a
// value any adapter can write. It falls through to `default` the same
// way an absent key does, rather than reaching the frontmatter as a
// broken mapping. The rule holds for every collapsed key.
func TestResolveMeta_NonScalarUnderTargetFallsThrough(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"model", "effort"} {
		in := map[string]any{
			"name": "a",
			key:    map[string]any{"claude": map[string]any{"a": "b"}, "default": "high"},
		}
		if got := ResolveMeta(in, "claude")[key]; got != "high" {
			t.Errorf("%s: nested map under the target should fall through to default, got %v", key, got)
		}
		bare := map[string]any{
			"name": "a",
			key:    map[string]any{"claude": []any{"a"}},
		}
		if got, ok := ResolveMeta(bare, "claude")[key]; ok {
			t.Errorf("%s: a list under the target with no default should drop, got %v", key, got)
		}
	}
}
