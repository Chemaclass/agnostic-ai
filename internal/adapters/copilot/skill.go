package copilot

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// skillFields are the portable SKILL.md fields Copilot reads beside name
// and description: `argument-hint` shows in the chat input, and
// `disable-model-invocation: true` leaves the skill to the `/` menu
// (code.visualstudio.com/docs/copilot/customization/agent-skills).
var skillFields = []string{"license", "argument-hint", "disable-model-invocation"}

// SkillMarkdown renders s with the fields Copilot reads.
func (Adapter) SkillMarkdown(s spec.Entry) string {
	resolved := emit.ResolveMeta(s.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = s.Name
	}
	meta := map[string]any{"name": s.Name, "description": desc}
	keys := []string{"name", "description"}
	for _, k := range skillFields {
		if v, ok := resolved[k]; ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, s.Meta, target, append(append([]string(nil), keys...), "model", "effort")...)
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

func writeSkillFolders(sess *emit.Session, skills []spec.Entry, dir string, dryRun bool) error {
	for _, s := range skills {
		folder := filepath.Join(dir, s.Name)
		if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader((Adapter{}).SkillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		if err := sess.PropagateSkillAssets(s, folder, emit.SkipSKILLMd, dryRun); err != nil {
			return err
		}
	}
	return nil
}
