package adapters

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/aider"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cline"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/continueai"
	"github.com/chemaclass/agnostic-ai/internal/adapters/copilot"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
	"github.com/chemaclass/agnostic-ai/internal/adapters/gemini"
	"github.com/chemaclass/agnostic-ai/internal/adapters/windsurf"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type Adapter interface {
	Name() string
	Emit(entries []spec.Entry, cfg *config.Config, dryRun bool) error
}

var registry = map[string]Adapter{
	"claude":   claude.New(),
	"codex":    codex.New(),
	"gemini":   gemini.New(),
	"cursor":   cursor.New(),
	"copilot":  copilot.New(),
	"aider":    aider.New(),
	"cline":    cline.New(),
	"windsurf": windsurf.New(),
	"continue": continueai.New(),
}

func Get(name string) (Adapter, bool) {
	a, ok := registry[name]
	return a, ok
}

func Names() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}
