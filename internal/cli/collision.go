package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// detectCollisions checks whether two or more targets emit to the same
// output path and applies cfg.Sync.CollisionPolicy to decide what to do:
//
//   - prompt (default): error with a resolution hint. On non-interactive
//     stdin an extra CI hint is appended.
//   - prefer-spec: skip the collision check; let the last adapter win.
//   - fail: hard error with no resolution hint.
//
// AGENTS.md is the canonical example: Codex, Amp, Warp, OpenCode, and Zed
// all default to that file. Enabling more than one causes silent
// last-writer-wins and perpetual drift in `sync --check`.
func detectCollisions(cfg *config.Config, b spec.Bundle, targets []string) error {
	policy := cfg.Sync.CollisionPolicy
	if policy == "" {
		policy = "prompt"
	}
	if policy == "prefer-spec" {
		return nil
	}

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

	if policy == "fail" {
		return errs.Coded(errs.CodeOutputCollision,
			"output collision: targets emit to the same path\n%s",
			strings.Join(lines, "\n"))
	}

	// prompt: error with resolution hint
	msg := "output collision: targets emit to the same path\n%s\n" +
		"resolve by dropping one from `targets:` in agnostic-ai.yaml, " +
		"or override the collider via `outputs.<target>.file`"
	if !term.IsTerminal(os.Stdin.Fd()) {
		msg += "\nfor CI use: set `sync.collision-policy: prefer-spec` in agnostic-ai.yaml"
	}
	return errs.Coded(errs.CodeOutputCollision, msg, strings.Join(lines, "\n"))
}
