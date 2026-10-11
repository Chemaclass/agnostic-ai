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

func TestEmit_SkillInvocationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		meta    map[string]any
		want    any
		present bool
	}{
		{"true", map[string]any{"disable-model-invocation": true}, true, true},
		{"false", map[string]any{"disable-model-invocation": false}, false, true},
		{"absent", map[string]any{}, nil, false},
		{"override", map[string]any{"disable-model-invocation": true, "x-opencode": map[string]any{"disable-model-invocation": false}}, false, true},
		{"deleted", map[string]any{"disable-model-invocation": true, "x-opencode": map[string]any{"disable-model-invocation": nil}}, nil, false},
		{"metadata-false", map[string]any{"disable-model-invocation": false, "x-opencode": map[string]any{"metadata": map[string]any{"opencode/autoinvoke": "false"}}}, false, true},
		{"metadata-alias", map[string]any{"disable-model-invocation": true, "x-opencode": map[string]any{"metadata": map[string]any{"opencode/autoinvoke": "true"}}}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			emit.DrainNotes()
			t.Cleanup(func() { emit.DrainNotes() })
			skill := spec.Entry{Kind: spec.KindSkill, Name: "release", Scope: "services/api", Body: "Release only when asked.", Meta: tc.meta}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{skill}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "services/api/.opencode/skills/release/SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			native, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
			if err != nil {
				t.Fatal(err)
			}
			got, present := native.Meta["disable-model-invocation"]
			if present != tc.present || got != tc.want {
				t.Errorf("policy = %#v, present %t, want %#v, present %t", got, present, tc.want, tc.present)
			}
			if tc.name == "metadata-alias" || tc.name == "metadata-false" {
				metadata, _ := native.Meta["metadata"].(map[string]any)
				wantAlias := "true"
				if tc.name == "metadata-false" {
					wantAlias = "false"
				}
				if metadata["opencode/autoinvoke"] != wantAlias {
					t.Errorf("native alias lost: %#v", metadata)
				}
			}
			for _, note := range emit.DrainNotes() {
				if note.Field == "disable-model-invocation" {
					t.Errorf("retained policy reported dropped: %#v", note)
				}
			}
		})
	}
}
