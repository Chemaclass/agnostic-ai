package codebuddy

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type HookCommand struct {
	Command string
	Timeout int
}

func HookCommands(cfg *config.Config, h spec.Entry) []HookCommand {
	m := emit.ResolveMeta(h.Meta, target)
	scripts := emit.OutputSubDir(cfg, target, "hooks", ".codebuddy/hooks")
	var out []HookCommand
	for _, command := range emit.HookCommands(m["command"]) {
		command = emit.ShellHookCommand(command, target, m)
		command = emit.WrapPortableHook(h, target, command)
		command = strings.ReplaceAll(command, emit.HookScriptsDir(target)+"/", filepath.ToSlash(scripts)+"/")
		out = append(out, HookCommand{Command: command, Timeout: emit.HookIntMeta(m, "timeout")})
	}
	return out
}

// PromptHookEvents are the events CodeBuddy runs prompt hooks on.
func PromptHookEvents() []string { return []string{"Stop", "UserPromptSubmit", "PreToolUse"} }

// PromptHandler returns the native prompt handler for h, or nil when h is not a prompt hook.
func PromptHandler(h spec.Entry) map[string]any {
	m := emit.ResolveMeta(h.Meta, target)
	if m["type"] != "prompt" {
		return nil
	}
	handler := map[string]any{"type": "prompt", "prompt": m["prompt"]}
	if timeout := emit.HookIntMeta(m, "timeout"); timeout > 0 {
		handler["timeout"] = timeout
	}
	if emit.HookBoolMeta(m, "continueOnBlock") {
		handler["continueOnBlock"] = true
	}
	return handler
}

// ValidateHooks checks hooks for project settings, which take command and prompt handlers.
func ValidateHooks(hooks []spec.Entry) error {
	if err := validateHooks(hooks, true); err != nil {
		return err
	}
	count := 0
	for _, h := range hooks {
		if m := emit.ResolveMeta(h.Meta, target); m["type"] == "prompt" && m["model"] != nil {
			count++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "model", count, "CodeBuddy prompt hooks have no model field")
	return nil
}

// ValidateUserHooks checks hooks for user settings, where sync writes command handlers only.
func ValidateUserHooks(hooks []spec.Entry) error {
	return validateHooks(hooks, false)
}

func validateHooks(hooks []spec.Entry, prompts bool) error {
	for _, h := range hooks {
		m := emit.ResolveMeta(h.Meta, target)
		if m["disabled"] == true {
			return fmt.Errorf("%s: codebuddy: disabled hooks have no native mapping; exclude this target", h.Path)
		}
		event, _ := m["event"].(string)
		if event != "" && !slices.Contains(HookEvents(), event) {
			return fmt.Errorf("%s: codebuddy: unsupported hook event %q", h.Path, event)
		}
		typ, _ := m["type"].(string)
		switch {
		case typ == "" || typ == "command":
		case !prompts:
			return fmt.Errorf("%s: codebuddy: user settings take only command hooks", h.Path)
		case typ != "prompt" || !slices.Contains(PromptHookEvents(), event):
			return fmt.Errorf("%s: codebuddy: only command hooks, or prompt hooks on Stop, UserPromptSubmit, and PreToolUse, are supported", h.Path)
		default:
			if prompt, _ := m["prompt"].(string); prompt == "" {
				return fmt.Errorf("%s: codebuddy: prompt hooks need a prompt", h.Path)
			}
			if _, ok := m["command"]; ok {
				return fmt.Errorf("%s: codebuddy: prompt hooks take no command; split it into a command hook", h.Path)
			}
		}
	}
	return nil
}
