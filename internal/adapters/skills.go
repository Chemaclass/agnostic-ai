package adapters

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type SkillRenderer interface {
	SkillMarkdown(skill spec.Entry) string
}

// SkillSidecarRenderer is implemented by adapters that write files
// beside SKILL.md, such as Codex's agents/openai.yaml.
type SkillSidecarRenderer interface {
	SkillSidecars(skill spec.Entry) (map[string]string, error)
}

// ManualOnlySkillReader is implemented by adapters whose manual-only
// marker lives outside SKILL.md frontmatter. A set marker, either way,
// silences the note, as an explicit x-<target> field does on crush and
// factory.
type ManualOnlySkillReader interface {
	SkillInvocationPolicySet(skill spec.Entry) bool
}

func PrepareSkillCapabilities(target string, skills []spec.Entry, mode string) ([]spec.Entry, error) {
	normalized := spec.Bundle{}
	for _, skill := range skills {
		if !skill.EmitsTo(target) {
			continue
		}
		native, problem := skill.NativeAllowedTools()
		if problem != "" {
			return nil, fmt.Errorf("%s: %s", skill.Path, problem)
		}
		normalized.Skills = append(normalized.Skills, native)
	}
	filtered, err := emit.WithoutUnsupportedDelete(normalized, target, mode)
	if err != nil {
		return nil, err
	}
	return filtered.Skills, nil
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

func NoteDroppedSkillFields(target string, skills []spec.Entry, shared bool) {
	coverage := emit.SkillFieldCoverage{Handled: []string{"disable-model-invocation"}, Shared: shared}
	if shared {
		coverage.Markdown = func(skill spec.Entry) string { return emit.SkillMarkdown(skill, "") }
	} else if adapter, ok := Get(target); ok {
		if renderer, ok := adapter.(SkillRenderer); ok {
			coverage.Markdown = renderer.SkillMarkdown
		}
	}
	emit.NoteDroppedSkillFields(target, (spec.Bundle{Skills: skills}).For(target).Skills, coverage)
}

// ReportClaudeSkillSyntax raises the Claude Code body syntax notes for
// the skills that reach target. mode is the on-unsupported policy.
func ReportClaudeSkillSyntax(target string, skills []spec.Entry, mode string) error {
	return emit.ReportClaudeBodySyntax(target, spec.KindSkill, (spec.Bundle{Skills: skills}).For(target).Skills, mode)
}

// RenderSkillSidecars returns target's extra per-skill files, keyed by
// path relative to the skill folder, or nil when it writes none.
func RenderSkillSidecars(target string, skill spec.Entry) (map[string]string, error) {
	adapter, err := Resolve(target)
	if err != nil {
		return nil, err
	}
	if renderer, ok := adapter.(SkillSidecarRenderer); ok {
		return renderer.SkillSidecars(skill)
	}
	return nil, nil
}

// Losing a manual-only flag makes a global skill model-invocable.
func NoteManualOnlySkillDrops(target string, skills []spec.Entry, shared bool) error {
	adapter, err := Resolve(target)
	if err != nil {
		return err
	}
	reader, hasReader := adapter.(ManualOnlySkillReader)
	dropped, omitted := 0, 0
	for _, skill := range (spec.Bundle{Skills: skills}).For(target).Skills {
		manual, set := emit.ResolveMeta(skill.Meta, target)["disable-model-invocation"].(bool)
		if !set {
			continue
		}
		if hasReader && reader.SkillInvocationPolicySet(skill) {
			continue
		}
		rendered, err := RenderSkillMarkdown(target, skill, shared)
		if err != nil {
			return err
		}
		if manual {
			if !renderedManualOnly(rendered) {
				dropped++
			}
		} else {
			entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(rendered))
			if err != nil {
				return fmt.Errorf("parse rendered skill %s: %w", skill.Path, err)
			}
			if _, kept := entry.Meta["disable-model-invocation"]; !kept {
				omitted++
			}
		}
	}
	emit.NoteSkillFieldNoOp(target, "disable-model-invocation", dropped, "its global copy stays model-invocable", shared)
	emit.NoteSkillFieldNoOp(target, "disable-model-invocation", omitted, "the skill file has no disable-model-invocation field", shared)
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

var skillFieldKeepers = struct {
	once    sync.Once
	byField map[string][]string
}{}

// SkillFieldKeepers lists the built-in targets whose own skill file keeps
// field, found by rendering a probe skill, so a fix that names target
// overrides cannot drift from what each adapter writes.
func SkillFieldKeepers(field string) []string {
	skillFieldKeepers.once.Do(func() {
		probe := spec.Entry{Kind: spec.KindSkill, Name: "probe", Meta: map[string]any{
			"name": "probe", "description": "Probe.", "argument-hint": "[value]", "metadata": map[string]any{"owner": "team"},
		}}
		skillFieldKeepers.byField = map[string][]string{}
		for _, name := range Names() {
			if !slices.Contains(registry[name].Capabilities(), spec.KindSkill) {
				continue
			}
			rendered, err := RenderSkillMarkdown(name, probe, false)
			if err != nil {
				continue
			}
			entry, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(rendered))
			if err != nil {
				continue
			}
			for _, f := range []string{"argument-hint", "metadata"} {
				if _, kept := entry.Meta[f]; kept {
					skillFieldKeepers.byField[f] = append(skillFieldKeepers.byField[f], name)
				}
			}
		}
	})
	return skillFieldKeepers.byField[field]
}

func init() { emit.SkillFieldKeepers = SkillFieldKeepers }
