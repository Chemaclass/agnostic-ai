package codebuddy

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func HookEvents() []string {
	return []string{"SessionStart", "SessionEnd", "UserPromptSubmit", "PreToolUse", "PostToolUse", "PostToolUseFailure", "Stop", "StopFailure", "SubagentStart", "SubagentStop", "PreCompact", "PostCompact", "Notification", "PermissionRequest", "PermissionDenied", "Elicitation", "ElicitationResult", "InstructionsLoaded", "ConfigChange", "TaskCreated", "TaskCompleted", "TeammateIdle", "FileChanged", "CwdChanged", "WorktreeCreate", "WorktreeRemove", "Setup"}
}
func writeSettings(sess *emit.Session, hooks, settings []spec.Entry, cfg *config.Config, dryRun bool) error {
	path := emit.OutputConfFile(cfg, target, emit.OutputSubDir(cfg, target, "settings.json", ".codebuddy/settings.json"))
	scripts := emit.OutputSubDir(cfg, target, "hooks", ".codebuddy/hooks")
	if err := ValidateHooks(hooks); err != nil {
		return err
	}
	keys := map[string]any{}
	if v := emit.SettingsModel(settings, target); v != "" {
		keys["model"] = v
	}
	if v := emit.SettingsEffortLevel(settings, target, (Adapter{}).SettingsEffortLevels()); v != "" {
		keys["reasoningEffort"] = v
	}
	permissions := emit.SettingsPermissions(settings)
	for _, key := range []string{"allow", "ask", "deny"} {
		for _, rule := range list(permissions[key]) {
			if _, ok := (Adapter{}).TranslatePermission(key, rule); !ok {
				return fmt.Errorf("codebuddy: permissions.%s rule %q has no verified native mapping; use x-codebuddy.permissions", key, rule)
			}
		}
	}
	if permissions == nil {
		permissions = map[string]any{}
	}
	for _, s := range settings {
		p, _ := s.Meta["permissions"].(map[string]any)
		if v, ok := p["default-mode"]; ok {
			permissions["defaultMode"] = v
		}
	}
	if len(permissions) > 0 {
		keys["permissions"] = permissions
	}
	emit.MergeSettingsCustomKeys(keys, settings, target)
	if _, ok := keys["autoMode"]; ok {
		return fmt.Errorf("codebuddy: project settings cannot provide autoMode; keep this policy in user settings")
	}
	if p, ok := keys["permissions"].(map[string]any); ok {
		if v, exists := p["defaultMode"]; exists && !slices.Contains([]string{"default", "acceptEdits", "auto", "dontAsk", "plan", "bypassPermissions"}, fmt.Sprint(v)) {
			return fmt.Errorf("codebuddy: unsupported permissions.defaultMode %v", v)
		}
	}
	if p, ok := keys["permissions"].(map[string]any); ok && p["defaultMode"] == "auto" {
		return fmt.Errorf("codebuddy: project settings cannot grant permissions.defaultMode auto; choose it in user settings")
	}
	events := map[string]any{}
	for _, h := range hooks {
		m := emit.ResolveMeta(h.Meta, target)
		event, _ := m["event"].(string)
		if event == "" {
			continue
		}
		var handlers []any
		if prompt := PromptHandler(h); prompt != nil {
			handlers = append(handlers, prompt)
		}
		for _, c := range HookCommands(cfg, h) {
			handler := map[string]any{"type": "command", "command": c.Command}
			if c.Timeout > 0 {
				handler["timeout"] = c.Timeout
			}
			handlers = append(handlers, handler)
		}
		if len(handlers) == 0 {
			continue
		}
		group := map[string]any{"hooks": handlers}
		if matcher, _ := m["matcher"].(string); matcher != "" {
			group["matcher"] = matcher
		}
		groups, _ := events[event].([]any)
		events[event] = append(groups, group)
	}
	if len(events) > 0 {
		keys["hooks"] = emit.MergeSettingsCustomValue(target, "hooks", keys["hooks"], events)
	}
	if raw, ok := keys["hooks"].(map[string]any); ok {
		nested := map[string]any{}
		for event, groups := range raw {
			nested[event] = mergeManagedList(sess, path, "hooks", event, hookList(groups), dryRun)
		}
		keys["hooks"] = nested
	}

	if p, ok := keys["permissions"].(map[string]any); ok {
		for _, key := range []string{"allow", "ask", "deny"} {
			if v, ok := p[key]; ok {
				items := []any{}
				for _, rule := range list(v) {
					items = append(items, rule)
				}
				p[key] = mergeManagedList(sess, path, "permissions", key, items, dryRun)
			}
		}
	}
	if len(keys) > 0 {
		if err := sess.MergeJSONFileNested(path, keys, []string{"permissions", "hooks"}, dryRun); err != nil {
			return err
		}
	}
	if err := sess.MaterializeNeutralHookScripts(hooks, target, scripts, dryRun); err != nil {
		return err
	}
	for _, h := range hooks {
		for _, command := range emit.HookCommands(emit.ResolveMeta(h.Meta, target)["command"]) {
			source, _ := emit.SourceToolFromHookCommand(command)
			if err := sess.MaterializeHookScript(emit.RewriteHookPath(command, target, h.Meta), target, source, dryRun); err != nil {
				return err
			}
		}
	}
	return nil
}
func hookList(v any) []any {
	if v, ok := v.([]any); ok {
		return v
	}
	return nil
}

func mergeManagedList(sess *emit.Session, path, parent, event string, groups []any, dryRun bool) any {
	existing := sess.ExistingJSONObject(path, parent, dryRun)
	prior := emit.PriorClaimedItems(path, []string{parent, event})
	kept := []any{}
	for _, g := range hookList(existing[event]) {
		raw, _ := json.Marshal(g)
		if !slices.Contains(prior, emit.ContentSum(string(raw))) {
			kept = append(kept, g)
		}
	}
	merged := emit.MergeSettingsCustomValue(target, "hooks", kept, groups)
	owned := []string{}
	for _, g := range groups {
		raw, _ := json.Marshal(g)
		owned = append(owned, string(raw))
	}
	return emit.ClaimedJSONItems(merged, owned)
}
