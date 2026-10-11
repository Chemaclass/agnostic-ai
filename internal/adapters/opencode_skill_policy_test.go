package adapters

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestOpenCodeSkillRendererKeepsInvocationPolicyWithoutChangingSibling(t *testing.T) {
	skill := spec.Entry{Kind: spec.KindSkill, Name: "release", Body: "Release only when asked.", Meta: map[string]any{"disable-model-invocation": true}}
	for _, target := range []string{"opencode", "amp"} {
		rendered, err := RenderSkillMarkdown(target, skill, false)
		if err != nil {
			t.Fatal(err)
		}
		native, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(rendered))
		if err != nil {
			t.Fatal(err)
		}
		policy, present := native.Meta["disable-model-invocation"]
		if target == "opencode" && (!present || policy != true) {
			t.Errorf("OpenCode global policy lost: %#v", native.Meta)
		}
		if target == "amp" && present {
			t.Errorf("Amp gained an unsupported field: %#v", native.Meta)
		}
	}
}
