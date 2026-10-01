package gemini

import (
	"fmt"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// HookCommand is one command handler sync writes for a hook spec, with
// the env and timeout Gemini CLI gives that handler.
type HookCommand struct {
	Command string
	Env     map[string]string
	Timeout time.Duration
}

// HookCommands returns the command handlers sync writes for h.
func HookCommands(h spec.Entry) []HookCommand {
	var out []HookCommand
	for _, handler := range hookHandlers(h) {
		command, _ := handler["command"].(string)
		env, _ := handler["env"].(map[string]any)
		c := HookCommand{Command: command, Env: map[string]string{}}
		for k, v := range emit.WithHookTarget(env, any(target)) {
			c.Env[k] = fmt.Sprint(v)
		}
		switch ms := handler["timeout"].(type) {
		case int:
			c.Timeout = time.Duration(ms) * time.Millisecond
		case int64:
			c.Timeout = time.Duration(ms) * time.Millisecond
		case float64:
			c.Timeout = time.Duration(ms * float64(time.Millisecond))
		}
		out = append(out, c)
	}
	return out
}

// SettingsFilePath is the settings file sync writes hooks to.
func SettingsFilePath(cfg *config.Config) string {
	return emit.OutputMCPFile(cfg, target, defaultSettingsFile)
}
