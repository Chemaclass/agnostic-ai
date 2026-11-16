package conversations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClaudeAppCacheJSON_RejectsRepeatedReferencesOverTheBudget(t *testing.T) {
	var repeated any = []any{}
	for range 20 {
		repeated = []any{repeated, repeated}
	}
	cache := map[string]any{"clientState": map[string]any{"queries": []any{}}, "extra": repeated}
	if _, err := claudeAppCacheJSON(cache); err == nil {
		t.Error("accepted a cache whose repeated references exceed the expansion budget")
	}
}

func TestClaudeAppCacheJSON_KeepsSharedValuesWithinTheBudget(t *testing.T) {
	shared := map[string]any{"title": "A chat"}
	value := []any{shared, shared}
	got, err := claudeAppCacheJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestClaudeAppCacheJSON_RejectsCycles(t *testing.T) {
	value := map[string]any{}
	value["self"] = value
	if _, err := claudeAppCacheJSON(value); err == nil {
		t.Error("accepted a cyclic cache")
	}
}

func TestClaudeAppCacheJSON_RejectsSharedStringsOverTheByteBudget(t *testing.T) {
	var repeated any = strings.Repeat("x", 4096)
	for range 14 {
		repeated = []any{repeated, repeated}
	}
	if _, err := claudeAppCacheJSON(repeated); err == nil {
		t.Error("accepted shared strings whose expansion exceeds the byte budget")
	}
}
