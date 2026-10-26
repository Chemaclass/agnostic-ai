package qwen

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitPathRules(sess *emit.Session, rules []spec.Entry, cfg *config.Config, dryRun bool) error {
	dir := emit.OutputRulesDir(cfg, target, defaultRulesDir)
	for _, r := range rules {
		patterns := emit.PathRulePatterns(cfg, target, r)
		if patterns == nil {
			continue
		}
		meta := map[string]any{"paths": patterns}
		keys := []string{"paths"}
		if description := r.Description(); description != "" {
			meta["description"] = description
			keys = []string{"description", "paths"}
		}
		content := emit.WithHeader(emit.FrontmatterOrdered(meta, keys)+"\n"+r.Body, emit.FormatMarkdown)
		if err := sess.WriteFile(filepath.Join(dir, r.Name+".md"), content, dryRun); err != nil {
			return err
		}
	}
	return nil
}
