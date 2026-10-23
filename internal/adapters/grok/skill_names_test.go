package grok

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_ValidatesNativeSkillAndCommandIdentities(t *testing.T) {
	for _, kind := range []spec.Kind{spec.KindSkill, spec.KindCommand} {
		for _, tc := range []struct {
			label, name string
			valid       bool
		}{
			{"boundary", strings.Repeat("a", 64), true},
			{"oversized", strings.Repeat("a", 65), false},
			{"normalized", "My__Skill", true},
			{"long-valid-normalization", strings.Repeat("a", 63) + "__", true},
			{"empty-normalization", "___", false},
			{"unicode-normalization", "Åread", true},
		} {
			t.Run(string(kind)+"/"+tc.label, func(t *testing.T) {
				dir := testutil.TempCwd(t)
				entry := spec.Entry{Kind: kind, Name: tc.name, Body: "Read the supplied diff."}
				bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindRule, Name: "first", Body: "Keep changes small."}, entry})
				err := New().Emit(emit.NewSession(), bundle, visibleConfig(), false)
				if !tc.valid {
					if err == nil || !strings.Contains(err.Error(), "64") {
						t.Fatalf("invalid identity accepted: %v", err)
					}
					if files := testutil.WalkRel(t, dir); len(files) != 0 {
						t.Errorf("wrote before rejecting invalid identity: %v", files)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(".grok", "skills", tc.name, "SKILL.md")
				if kind == spec.KindCommand {
					path = filepath.Join(".grok", "commands", tc.name+".md")
				}
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), entry.Body) {
					t.Errorf("missing native content: %s (%v)", data, err)
				}
			})
		}
	}
}

func TestEmit_KeepsValidNativeSkillNameFallbacks(t *testing.T) {
	for _, tc := range []struct {
		label, name string
		native      any
		valid       bool
	}{
		{"directory-fallback", "review", "!!!", true},
		{"native-name-fallback", strings.Repeat("a", 65), "review", true},
		{"numeric-native-name", strings.Repeat("a", 65), 42, true},
		{"map-name-directory-fallback", "review", map[string]any{"invalid": true}, true},
		{"all-fallbacks-invalid", strings.Repeat("a", 65), "!!!", false},
		{"map-name-invalid-fallback", strings.Repeat("a", 65), map[string]any{"invalid": true}, false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			testutil.TempCwd(t)
			entry := spec.Entry{Kind: spec.KindSkill, Name: tc.name, Meta: map[string]any{"x-grok": map[string]any{"name": tc.native}}, Body: "Inspect the supplied change."}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), visibleConfig(), false)
			if !tc.valid {
				if err == nil || !strings.Contains(err.Error(), "64") {
					t.Fatalf("invalid fallback accepted: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(".grok", "skills", tc.name, "SKILL.md"))
			if err != nil || !strings.Contains(string(data), entry.Body) {
				t.Errorf("valid native fallback not emitted: %s (%v)", data, err)
			}
		})
	}
}
