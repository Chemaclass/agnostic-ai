package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// detectCollisions returns an error when two or more targets emit to the
// same output path. AGENTS.md is a community-shared file consumed by
// Codex, Amp, Warp, OpenCode and Zed; enabling more than one adapter
// that owns the same default path causes silent last-writer-wins, and
// `sync --check` then reports perpetual drift no matter the write order.
// Force the user to pick: drop a target, or override the colliding path
// via outputs.<target>.file in agnostic-ai.yaml.
func detectCollisions(cfg *config.Config, b spec.Bundle, targets []string) error {
	owners := map[string][]string{}
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		adapters.StartCapture()
		if err := adapter.Emit(b, cfg, false); err != nil {
			adapters.StopCapture()
			return fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range adapters.StopCapture() {
			owners[f.Path] = append(owners[f.Path], t)
		}
	}
	var lines []string
	for path, ts := range owners {
		if len(ts) < 2 {
			continue
		}
		sort.Strings(ts)
		lines = append(lines, fmt.Sprintf("  %s ← %s", path, strings.Join(ts, ", ")))
	}
	if len(lines) == 0 {
		return nil
	}
	sort.Strings(lines)
	return fmt.Errorf("output collision: targets emit to the same path.\n%s\n"+
		"Resolve by dropping one from `targets:` in agnostic-ai.yaml, "+
		"or override the collider via `outputs.<target>.file`.",
		strings.Join(lines, "\n"))
}
