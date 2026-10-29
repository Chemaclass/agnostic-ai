package preview

import (
	"reflect"
	"strings"
	"testing"
)

func TestRedactTree_HidesKeysAndValuesAndReportsTheirPaths(t *testing.T) {
	tree := map[string]any{
		"keep":     "plain",
		"list":     []any{"ok", "SECRET-a", map[string]any{"SECRET-k": "allow"}},
		"rows":     []map[string]any{{"cmd": "SECRET-b"}},
		"SECRET-x": "deny",
		"SECRET-y": map[string]any{"inner": "SECRET-c"},
	}
	secret := func(s string) bool { return strings.HasPrefix(s, "SECRET-") }
	got, paths := RedactTree(tree, secret, func(_ []any, v any) bool {
		text, ok := v.(string)
		return ok && secret(text)
	})
	want := map[string]any{
		"keep":          "plain",
		"list":          []any{"ok", Redacted, map[string]any{Redacted: "allow"}},
		"rows":          []any{map[string]any{"cmd": Redacted}},
		Redacted + "#1": "deny",
		Redacted + "#2": map[string]any{"inner": Redacted},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
	wantPaths := [][]any{
		{"SECRET-x"},
		{"SECRET-y"},
		{"SECRET-y", "inner"},
		{"list", 1},
		{"list", 2, "SECRET-k"},
		{"rows", 0, "cmd"},
	}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("paths = %v, want %v", paths, wantPaths)
	}
	if tree["list"].([]any)[1] != "SECRET-a" {
		t.Error("RedactTree changed the tree it was given")
	}
}

func TestRedactTree_DoesNotWalkAReportedPart(t *testing.T) {
	tree := map[string]any{"notify": []any{"SECRET-a"}}
	var seen [][]any
	got, paths := RedactTree(tree, nil, func(path []any, v any) bool {
		seen = append(seen, path)
		_, list := v.([]any)
		return list
	})
	if want := map[string]any{"notify": Redacted}; !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %#v, want %#v", got, want)
	}
	if want := [][]any{{"notify"}}; !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
	if want := [][]any{nil, {"notify"}}; !reflect.DeepEqual(seen, want) {
		t.Errorf("visited %v, want %v", seen, want)
	}
}
