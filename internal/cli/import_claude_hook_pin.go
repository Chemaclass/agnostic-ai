package cli

import (
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// claudeHookPin decides whether an imported Claude hook keeps
// `target: claude`. A hook stays portable only when every other target
// the project syncs runs it as written; one target that cannot tell
// keeps the pin, since a wider hook is harder to spot than a missing one.
type claudeHookPin struct {
	// others are the configured targets besides claude.
	others []string
	// judged is set when one of others can tell whether it runs a hook,
	// so a kept pin has a reason worth printing.
	judged bool
}

func newClaudeHookPin(root string) claudeHookPin {
	cfg, err := config.Load(root)
	if err != nil {
		return claudeHookPin{}
	}
	var pin claudeHookPin
	for _, target := range cfg.Targets {
		if target == "claude" || slices.Contains(pin.others, target) {
			continue
		}
		pin.others = append(pin.others, target)
		pin.judged = pin.judged || adapters.JudgesHooks(target)
	}
	return pin
}

// apply sets doc's target, printing why a pin stays when another target
// could have shared the hook. path is where the spec lands under root.
func (p claudeHookPin) apply(doc map[string]any, root, path string) {
	if !p.judged {
		doc["target"] = "claude"
		return
	}
	for _, target := range p.others {
		if reason := adapters.AcceptsHook(target, doc); reason != "" {
			doc["target"] = "claude"
			if rel, err := filepath.Rel(root, path); err == nil {
				path = rel
			}
			summaryf("  → %s keeps target: claude (%s)\n", filepath.ToSlash(path), reason)
			return
		}
	}
}
