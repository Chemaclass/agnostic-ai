package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookHandlers returns the command handlers sync writes for h on target,
// for the targets hookrun builds payloads for.
func HookHandlers(target string, h spec.Entry) []hookrun.Handler {
	var out []hookrun.Handler
	switch target {
	case "claude":
		for _, c := range claude.CommandHandlers(h) {
			out = append(out, hookrun.Handler{Command: c.Command, Args: c.Args, Shell: c.Shell})
		}
	case "codex":
		for _, c := range codex.HookCommands(h) {
			out = append(out, hookrun.Handler{Command: c.Command, CommandWindows: c.CommandWindows})
		}
	case "gemini":
		for _, c := range gemini.HookCommands(h) {
			out = append(out, hookrun.Handler{Command: c.Command, Env: c.Env, Timeout: c.Timeout})
		}
	}
	return out
}

// HookFile is the native file sync writes target's hooks to, for the
// targets hookrun builds payloads for.
func HookFile(cfg *config.Config, target string) string {
	switch target {
	case "claude":
		return claude.SettingsFilePath(cfg)
	case "codex":
		return codex.HooksFilePath(cfg)
	case "gemini":
		return gemini.SettingsFilePath(cfg)
	}
	return ""
}
