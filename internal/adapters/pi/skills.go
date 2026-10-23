package pi

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func (Adapter) SkillMarkdown(entry spec.Entry) string {
	resolved := emit.ResolveMeta(entry.Meta, target)
	meta := map[string]any{"name": entry.Name, "description": entry.Name}
	keys := []string{"name", "description"}
	if description, ok := resolved["description"].(string); ok && description != "" {
		meta["description"] = description
	}
	for _, key := range []string{"license", "compatibility", "metadata", "allowed-tools", "disable-model-invocation"} {
		if value, ok := resolved[key]; ok {
			meta[key] = value
			keys = append(keys, key)
		}
	}
	return emit.FrontmatterOrdered(meta, keys) + "\n" + strings.TrimSpace(entry.Body) + "\n"
}

func writeSkills(sess *emit.Session, entries []spec.Entry, dir string, dryRun bool) error {
	for _, entry := range entries {
		folder := filepath.Join(dir, entry.Name)
		if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader((Adapter{}).SkillMarkdown(entry), emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		if err := sess.PropagateSkillAssets(entry, folder, emit.SkipSKILLMd, dryRun); err != nil {
			return err
		}
	}
	return nil
}
