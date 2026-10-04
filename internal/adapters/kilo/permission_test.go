package kilo

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSettingsPermission_EditDenyAlsoBlocksWrite(t *testing.T) {
	settings := []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Write"}, "deny": []any{"Edit(.env)"}}}}}
	got, dropped := settingsPermission(settings)
	if dropped != 0 {
		t.Errorf("dropped = %d", dropped)
	}
	writes, ok := got["write"].(*emit.OrderedJSON)
	if !ok {
		t.Fatalf("write permissions = %#v", got["write"])
	}
	allow, _ := writes.Get("*")
	deny, _ := writes.Get(".env")
	if string(allow) != `"allow"` || string(deny) != `"deny"` {
		t.Errorf("write permissions = %#v", got["write"])
	}
}

func TestAgentPermission_PathRestriction(t *testing.T) {
	got, dropped := agentPermission([]string{"Read(src/**)", "Edit(docs/**)"})
	if dropped {
		t.Error("path-scoped tools dropped")
	}
	reads, ok := got["read"].(map[string]any)
	if !ok || reads["*"] != "deny" || reads["src/**"] != "allow" {
		t.Errorf("read permission = %#v", got["read"])
	}
}

func TestTranslatePermission_EditRestrictionsCoverWrite(t *testing.T) {
	for _, tc := range []struct {
		list, rule string
		want       []string
	}{
		{"deny", "Edit", []string{"edit", "write"}},
		{"ask", "Edit", []string{"edit", "write"}},
		{"deny", "Edit(.env)", []string{"edit(.env)", "write(.env)"}},
		{"ask", "Edit(go.mod)", []string{"edit(go.mod)", "write(go.mod)"}},
		{"allow", "Edit(docs/**)", []string{"edit(docs/**)"}},
	} {
		t.Run(tc.list+"/"+tc.rule, func(t *testing.T) {
			got, ok := (Adapter{}).TranslatePermission(tc.list, tc.rule)
			if !ok || !slices.Equal(got, tc.want) {
				t.Errorf("translation = %v, %t, want %v", got, ok, tc.want)
			}
		})
	}
}
