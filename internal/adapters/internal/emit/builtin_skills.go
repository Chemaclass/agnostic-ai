package emit

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	builtinSkillsStartMarker = "<!-- agnostic-ai:builtin-skills:start -->"
	builtinSkillsEndMarker   = "<!-- agnostic-ai:builtin-skills:end -->"
)

func renderBuiltinSkills(skills []spec.Entry) string {
	var sb strings.Builder
	for _, sk := range skills {
		if sk.Layer != "builtin" {
			continue
		}
		WriteSection(&sb, sk.Name, sk)
	}
	if sb.Len() == 0 {
		return ""
	}
	return builtinSkillsStartMarker + "\n\n## Built-in skills\n\nUse these skills when requested.\n\n" + sb.String() + builtinSkillsEndMarker + "\n"
}

func StripBuiltinSkills(body string) string {
	return stripMarkedBlock(body, builtinSkillsStartMarker, builtinSkillsEndMarker)
}
