package qwen

import (
	"fmt"
	"maps"

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
		mergeMCPExclusions(sess, keys, out, dryRun)
		emit.MergeEntriesOf(keys, "mcpServers")
		if err := sess.MergeJSONFileNested(out, keys, []string{"model", "permissions", "hooks", "mcp"}, dryRun); err != nil {
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

func mergeMCPExclusions(sess *emit.Session, keys map[string]any, path string, dryRun bool) {
	mcp, ok := keys["mcp"].(map[string]any)
	if !ok {
		return
	}
	mcp = maps.Clone(mcp)
	keys["mcp"] = mcp
	raw, held := mcp["excluded"]
	if !held {
		return
	}
	var planned []any
	switch list := raw.(type) {
	case []any:
		planned = list
	case []string:
		for _, value := range list {
			planned = append(planned, value)
		}
	default:
		return
	}
	existing, _ := sess.ExistingJSONObject(path, "mcp", dryRun)["excluded"].([]any)
	merged, claims := emit.MergeOwnedLists(path, []string{"mcp"}, map[string][]any{"excluded": existing}, map[string][]any{"excluded": planned})
	switch {
	case len(merged["excluded"]) == 0:
		mcp["excluded"] = emit.RemoveJSONKey
	case len(claims["excluded"]) == 0:
		mcp["excluded"] = emit.CarriedJSONValue(merged["excluded"])
	default:
		mcp["excluded"] = emit.ClaimedJSONItems(merged["excluded"], claims["excluded"])
	}
}
