package adapters

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type SkillRenderer interface {
	SkillMarkdown(skill spec.Entry) string
}

// SkillSidecarRenderer is implemented by adapters that write files
// beside SKILL.md, such as Codex's agents/openai.yaml.
type SkillSidecarRenderer interface {
	SkillSidecars(skill spec.Entry) map[string]string
}

// ManualOnlySkillReader is implemented by adapters whose manual-only
// marker lives outside SKILL.md frontmatter. An explicit value of that
// marker, either way, is the author's choice and silences the note, as
// an explicit x-<target> field does on crush and factory.
// SkillManualOnlyField names the marker for the coverage note.
type ManualOnlySkillReader interface {
	SkillInvocationPolicySet(skill spec.Entry) bool
	SkillManualOnlyField() string
}

// RenderSkillMarkdown keeps shared user directories free of target-specific metadata.
func RenderSkillMarkdown(target string, skill spec.Entry, shared bool) (string, error) {
	content := ""
	if shared {
		content = emit.SkillMarkdown(skill, "")
	} else {
		adapter, err := Resolve(target)
		if err != nil {
			return "", err
		}
		skill.Body = skill.BodyFor(target)
		if renderer, ok := adapter.(SkillRenderer); ok {
			content = renderer.SkillMarkdown(skill)
		} else {
			content = emit.SkillMarkdown(skill, target)
		}
	}
	return strings.TrimRight(emit.WithHeader(content, emit.FormatMarkdown), "\n") + "\n", nil
}

func NoteDroppedSkillFields(target string, skills []spec.Entry) {
	emit.NoteDroppedSkillFields(target, (spec.Bundle{Skills: skills}).For(target).Skills)
}

// RenderSkillSidecars returns target's extra per-skill files, keyed by
// path relative to the skill folder, or nil when it writes none.
func RenderSkillSidecars(target string, skill spec.Entry) (map[string]string, error) {
	adapter, err := Resolve(target)
	if err != nil {
		return nil, err
	}
	if renderer, ok := adapter.(SkillSidecarRenderer); ok {
		return renderer.SkillSidecars(skill), nil
	}
	return nil, nil
}

// NoteManualOnlySkillDrops reports skills marked
// `disable-model-invocation: true` whose global copy for target stays
// model-invocable. A manual-only skill is a safety boundary, so losing
// the marker must never be silent (#1156).
func NoteManualOnlySkillDrops(target string, skills []spec.Entry, shared bool) error {
	adapter, err := Resolve(target)
	if err != nil {
		return err
	}
	reader, hasReader := adapter.(ManualOnlySkillReader)
	dropped := 0
	for _, skill := range (spec.Bundle{Skills: skills}).For(target).Skills {
		if manual, _ := emit.ResolveMeta(skill.Meta, target)["disable-model-invocation"].(bool); !manual {
			continue
		}
		if hasReader && reader.SkillInvocationPolicySet(skill) {
			continue
		}
		rendered, err := RenderSkillMarkdown(target, skill, shared)
		if err != nil {
			return err
		}
		if !renderedManualOnly(rendered) {
			dropped++
		}
	}
	reason := "its global copy stays model-invocable"
	if hasReader {
		reason = "set " + reader.SkillManualOnlyField() + " instead"
	}
	emit.NoteFieldNoOp(target, spec.KindSkill, "disable-model-invocation", dropped, reason)
	return nil
}

func renderedManualOnly(rendered string) bool {
	entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(rendered))
	if err != nil {
		return false
	}
	manual, _ := entry.Meta["disable-model-invocation"].(bool)
	return manual
}
