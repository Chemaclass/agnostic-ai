package qwen

import (
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSkillMarkdown_WritesInvocationFields(t *testing.T) {
	out := New().SkillMarkdown(spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{
			"description":              "Ship a release",
			"disable-model-invocation": true,
			"user-invocable":           false,
			"paths":                    []any{"deploy/**"},
			"allowed-tools":            []any{"Bash"},
		},
		Body: "Run the release script.",
	})

	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(out))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if entry.Meta["disable-model-invocation"] != true {
		t.Errorf("disable-model-invocation missing:\n%s", out)
	}
	if entry.Meta["user-invocable"] != false {
		t.Errorf("user-invocable missing:\n%s", out)
	}
	if !reflect.DeepEqual(entry.Meta["paths"], []any{"deploy/**"}) {
		t.Errorf("paths missing:\n%s", out)
	}
	if _, set := entry.Meta["allowed-tools"]; set {
		t.Errorf("allowed-tools must stay out, Qwen reads allowedTools as a grant:\n%s", out)
	}
}

func TestSkillMarkdown_KeepsNativeInvocationOverride(t *testing.T) {
	out := New().SkillMarkdown(spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{
			"description": "Ship a release",
			"x-qwen":      map[string]any{"disable-model-invocation": true},
		},
	})
	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(out))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if entry.Meta["disable-model-invocation"] != true {
		t.Errorf("x-qwen.disable-model-invocation lost:\n%s", out)
	}
}

func TestSkillMarkdown_WritesStringPathsAsList(t *testing.T) {
	out := New().SkillMarkdown(spec.Entry{
		Kind: spec.KindSkill, Name: "deploy",
		Meta: map[string]any{"description": "d", "paths": "deploy/**, src/*.go"},
	})
	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(out))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, out)
	}
	if want := []any{"deploy/**", "src/*.go"}; !reflect.DeepEqual(entry.Meta["paths"], want) {
		t.Errorf("paths = %#v, want %#v\n%s", entry.Meta["paths"], want, out)
	}
}
