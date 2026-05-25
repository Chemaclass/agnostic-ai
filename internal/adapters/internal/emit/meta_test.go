package emit

import (
	"reflect"
	"testing"
)

func TestResolveMeta_DropsOtherTargets(t *testing.T) {
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

func TestResolveMeta_TargetOverridesTopLevel(t *testing.T) {
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
	if got := ResolveMeta(nil, "claude"); got != nil {
		t.Errorf("expected nil pass-through, got %#v", got)
	}
}

func TestResolveMeta_StripsRoutingKeys(t *testing.T) {
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
