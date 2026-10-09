package emit

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// skillLicenseField is the Agent Skills standard's license key.
const skillLicenseField = "license"

// SkillMarkdown renders the standard Agent Skills SKILL.md body: `name`,
// `description`, and `license` frontmatter (description resolved through the
// per-target meta, falling back to the skill name), arbitrary
// `x-<target>` keys passed through, then the trimmed body. Extra
// exclude keys suppress custom-meta passthrough for fields the adapter
// routes elsewhere (e.g. codex's agents/openai.yaml keys).
//
// Every adapter of the shared SKILL.md format must render through this
// function: targets that emit into one tree (codex, amp, zed at
// `.agents/skills/`) rely on byte-identical output to dedupe, and
// `sync.shared-skills` links folders across trees only when the
// rendered bytes match.
func SkillMarkdown(s spec.Entry, target string, exclude ...string) string {
	exclude = append(append([]string(nil), exclude...), "model", "effort")
	resolved := ResolveMeta(s.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = s.Name
	}
	meta := map[string]any{
		"name":        s.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if v, ok := resolved[skillLicenseField]; ok {
		meta[skillLicenseField] = v
		keys = append(keys, skillLicenseField)
	}
	MergeCustomTargetMeta(meta, &keys, s.Meta, target, append(append([]string(nil), keys...), exclude...)...)
	front := FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// WriteSkillFolder writes the standard Agent Skills folder layout for
// one skill: `<skillsDir>/<name>/SKILL.md` rendered via SkillMarkdown
// plus every sibling asset propagated byte-for-byte. Adapters with
// extra per-skill artifacts (codex) or a wider frontmatter allowlist
// (cursor, claude) keep their own emit path.
func (s *Session) WriteSkillFolder(sk spec.Entry, target, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, sk.Name)
	if err := s.WriteFile(filepath.Join(folder, "SKILL.md"), WithHeader(SkillMarkdown(sk, target), FormatMarkdown), dryRun); err != nil {
		return err
	}
	return s.PropagateSkillAssets(sk, folder, SkipSKILLMd, dryRun)
}

// WriteSkillFolders writes the standard Agent Skills folder layout for
// every skill into skillsDir. It is the multi-skill form of
// WriteSkillFolder, shared by the adapters that emit skills through the
// native folder layout with no per-target customization.
func (s *Session) WriteSkillFolders(skills []spec.Entry, target, skillsDir string, dryRun bool) error {
	for _, sk := range skills {
		if err := s.WriteSkillFolder(sk, target, skillsDir, dryRun); err != nil {
			return err
		}
	}
	return nil
}

// ScopedSkillsDir places a target's native skills directory below the
// canonical source scope. A skill at `skills/backend/review/SKILL.md`
// therefore reaches `backend/<native-skills-dir>/review/SKILL.md`.
func ScopedSkillsDir(scope, skillsDir string) (string, error) {
	if scope == "" {
		return skillsDir, nil
	}
	dir := filepath.Join(filepath.FromSlash(scope), skillsDir)
	if err := CheckScopePath(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// WriteScopedSkillFolders writes every skill into the native skills tree
// rooted at its canonical scope.
func (s *Session) WriteScopedSkillFolders(skills []spec.Entry, target, skillsDir string, dryRun bool) error {
	for _, skill := range skills {
		dir, err := ScopedSkillsDir(skill.Scope, skillsDir)
		if err != nil {
			return err
		}
		if err := s.WriteSkillFolder(skill, target, dir, dryRun); err != nil {
			return err
		}
	}
	return nil
}

type SkillFieldCoverage struct {
	Markdown         func(spec.Entry) string
	AdditionalFields func(spec.Entry) map[string]any
	Handled          []string
}

func NoteDroppedSkillFields(target string, skills []spec.Entry, coverage ...SkillFieldCoverage) {
	if notesSkipped.Load() {
		return
	}
	var fields SkillFieldCoverage
	if len(coverage) > 0 {
		fields = coverage[0]
	}
	render := fields.Markdown
	if render == nil {
		render = func(skill spec.Entry) string { return SkillMarkdown(skill, target) }
	}
	dropped := map[string]int{}
	for _, skill := range skills {
		emitted, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(render(skill)))
		if err != nil {
			continue
		}
		var additional map[string]any
		if fields.AdditionalFields != nil {
			additional = fields.AdditionalFields(skill)
		}
		for field, value := range ResolveMeta(skill.Meta, target) {
			if field == "name" || field == "description" || slices.Contains(fields.Handled, field) || value == nil {
				continue
			}
			if text, ok := value.(string); ok && text == "" {
				continue
			}
			if _, kept := emitted.Meta[field]; kept {
				continue
			}
			if _, kept := additional[field]; kept {
				continue
			}
			if target == "codex" {
				custom, _ := skill.Meta[XPrefix+target].(map[string]any)
				if _, sidecar := custom[field]; sidecar && slices.Contains(OpenAIYAMLKeys, field) {
					continue
				}
				if field == "disable-model-invocation" && SkillOpenAIYAMLPolicySet(skill) {
					continue
				}
			}
			dropped[field]++
		}
	}
	keys := make([]string, 0, len(dropped))
	for field := range dropped {
		keys = append(keys, field)
	}
	sort.Strings(keys)
	for _, field := range keys {
		NoteFieldNoOp(target, spec.KindSkill, field, dropped[field], "the skill file has no "+field+" field")
	}
}
