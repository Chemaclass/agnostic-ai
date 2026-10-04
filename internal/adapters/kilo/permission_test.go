package kilo

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSettingsPermission_EditDenyAlsoBlocksWrite(t *testing.T) {
	settings := []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"Write"}, "deny": []any{"Edit(.env)"}}}}}
	got, dropped := settingsPermission(settings)
	if dropped != 0 {
		t.Errorf("dropped = %d", dropped)
	}
	writes, ok := got["write"].(map[string]any)
	if !ok || writes["*"] != "allow" || writes[".env"] != "deny" {
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
