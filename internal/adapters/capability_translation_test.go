package adapters_test

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestCapabilityTranslation_NeutralNamesAndAliases(t *testing.T) {
	for _, tc := range []struct{ target, rule, want string }{
		{"gemini", "read", "read_file"}, {"gemini", "Read", "read_file"},
		{"kiro", "delete", "write"}, {"kiro", "mcp:github", "@github"},
		{"kiro", "mcp:github/get_issue", "@github/get_issue"},
		{"factory", "shell", "Execute"},
	} {
		t.Run(tc.target+"/"+tc.rule, func(t *testing.T) {
			got := adapters.TranslateCapability(tc.target, tc.rule)
			if !got.Supported || strings.Join(got.Native, ",") != tc.want {
				t.Errorf("translation = %+v, want %q", got, tc.want)
			}
		})
	}
}

func TestCapabilityTranslation_QoderPermissions(t *testing.T) {
	got := adapters.TranslatePermissionCapability("qoder", "allow", "shell", nil)
	if !got.Supported || strings.Join(got.Native, ",") != "Bash" {
		t.Errorf("translation = %+v, want Bash", got)
	}
}

func TestCapabilityTranslation_ErrorModeRejectsKiroWidening(t *testing.T) {
	a, _ := adapters.Get("kiro")
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindAgent, Name: "editor", Path: "agents/editor.md", Meta: map[string]any{"tools": []any{"Edit"}}}})
	err := a.Emit(adapters.NewSession(), b, &config.Config{OnUnsupported: "error"}, true)
	if err == nil || !strings.Contains(err.Error(), "delete_file") {
		t.Errorf("error = %v, want widening error", err)
	}
}

func TestCapabilityTranslation_UnsupportedDeleteDoesNotLeak(t *testing.T) {
	a, _ := adapters.Get("claude")
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindAgent, Name: "deleter", Path: "agents/deleter.md", Meta: map[string]any{"can": []any{"delete"}}, Body: "Delete files."}})
	err := adapters.EmitWithProvenance(adapters.NewSession(), a, b, &config.Config{OnUnsupported: "error"}, true)
	if err == nil || !strings.Contains(err.Error(), "can capability delete") {
		t.Errorf("error = %v, want source delete failure", err)
	}
}
