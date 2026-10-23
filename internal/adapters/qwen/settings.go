package qwen

import (
	"fmt"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func emitSettings(sess *emit.Session, b spec.Bundle, cfg *config.Config, dryRun bool) error {
	path := emit.OutputConfFile(cfg, target, defaultSettingsFile)
	mcpPath := emit.OutputMCPFile(cfg, target, path)
	hooksPath := emit.OutputHooksFile(cfg, target, path)
	files := map[string]map[string]any{path: {}}
	if model := emit.SettingsModel(b.Settings, target); model != "" {
		files[path]["model"] = map[string]any{"name": model}
	}
	if permissions := emit.SettingsPermissions(b.Settings); permissions != nil {
		files[path]["permissions"] = permissions
	}
	emit.MergeSettingsCustomKeys(files[path], b.Settings, target, "mcpServers", "hooks")
	if files[mcpPath] == nil {
		files[mcpPath] = map[string]any{}
	}
	if servers := buildMCPServers(b.MCPs); len(servers) > 0 {
		files[mcpPath]["mcpServers"] = servers
	}
	emit.MergeSettingsCustomRecordMap(files[mcpPath], b.Settings, target, "mcpServers")
	hooks := b.HooksFor(target)
	if files[hooksPath] == nil {
		files[hooksPath] = map[string]any{}
	}
	if block := buildHooks(hooks); len(block) > 0 {
		files[hooksPath]["hooks"] = block
	}
	if native, ok := emit.SettingsCustomKeys(b.Settings, target)["hooks"]; ok {
		files[hooksPath]["hooks"] = emit.MergeSettingsCustomValue(target, "hooks", files[hooksPath]["hooks"], native)
	}
	for out, keys := range files {
		if out == hooksPath {
			value, ok, err := sess.OwnedEventLists(out, "hooks", keys["hooks"], dryRun, "enabled", "disabled", "notifications")
			if err != nil {
				return fmt.Errorf("qwen hooks: %w", err)
			}
			if ok {
				keys["hooks"] = value
			} else {
				delete(keys, "hooks")
			}
		}
		if len(keys) == 0 {
			continue
		}
		emit.MergeEntriesOf(keys, "mcpServers")
		if err := sess.MergeJSONFileNested(out, keys, []string{"model", "permissions", "hooks"}, dryRun); err != nil {
			return err
		}
	}
	if err := sess.MaterializeNeutralHookScripts(hooks, target, emit.HookScriptsDir(target), dryRun); err != nil {
		return err
	}
	for _, h := range hooks {
		for _, command := range hookSourceCommands(h) {
			source, _ := emit.SourceToolFromHookCommand(command)
			if err := sess.MaterializeHookScript(emit.RewriteHookPath(command, target, h.Meta), target, source, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}
