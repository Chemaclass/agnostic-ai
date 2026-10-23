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
func ValidateHooks(hooks []spec.Entry) error {
	for _, h := range hooks {
		m := emit.ResolveMeta(h.Meta, target)
		if m["disabled"] == true {
			return fmt.Errorf("%s: codebuddy: disabled hooks have no native mapping; exclude this target", h.Path)
		}
		event, _ := m["event"].(string)
		if event != "" && !slices.Contains(HookEvents(), event) {
			return fmt.Errorf("%s: codebuddy: unsupported hook event %q", h.Path, event)
		}
		if typ, _ := m["type"].(string); typ != "" && typ != "command" {
			return fmt.Errorf("%s: codebuddy: only command hooks are supported", h.Path)
		}
	}
	return nil
}
