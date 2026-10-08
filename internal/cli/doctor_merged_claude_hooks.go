package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mergedClaudeHookSpecs returns the paths of Claude hook specs with two or
// more commands that all sit in one .claude/settings.json group of the same
// event and matcher, where that group gives them different settings.
// `import claude` wrote such specs before it kept each handler's settings,
// and they add a second group that runs every command twice.
func mergedClaudeHookSpecs(cfg *config.Config, b spec.Bundle) []string {
	if !slices.Contains(cfg.Targets, "claude") {
		return nil
	}
	data, err := os.ReadFile(claude.SettingsFilePath(cfg))
	if err != nil {
		return nil
	}
	var s claudehooks.Settings
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	var out []string
	for _, h := range b.HooksFor("claude") {
		event, _ := h.Meta["event"].(string)
		matcher, _ := h.Meta["matcher"].(string)
		h = adapters.TargetHook("claude", h)
		handlers := claude.CommandHandlers(claude.GuardCursorCopies(cfg, []spec.Entry{h})[0])
		if len(handlers) < 2 {
			continue
		}
		for _, g := range s.Hooks[event] {
			if g.Matcher == matcher && holdsWithOtherSettings(g.Hooks, handlers) {
				out = append(out, h.Path)
				break
			}
		}
	}
	return out
}

// holdsWithOtherSettings reports whether group has a command handler for
// every one in want, and at least one of them differs from want.
func holdsWithOtherSettings(group, want []claudehooks.CommandEntry) bool {
	byCommand := map[string]claudehooks.CommandEntry{}
	for _, h := range group {
		if h.Type == "" || h.Type == "command" {
			h.Type = "command"
			h.Command = adapters.StripCursorGuard(h.Command)
			byCommand[h.Command] = h
		}
	}
	differs := false
	for _, w := range want {
		w.Command = adapters.StripCursorGuard(w.Command)
		got, ok := byCommand[w.Command]
		if !ok {
			return false
		}
		differs = differs || !reflect.DeepEqual(got, w)
	}
	return differs
}

func reportMergedClaudeHookSpecs(cmd *cobra.Command, cfg *config.Config, paths []string) {
	if len(paths) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Claude hook specs:")
	for _, p := range paths {
		cmd.Printf("  ! %s merges commands that %s runs with different settings, so each runs twice\n", p, claude.SettingsFilePath(cfg))
	}
}
