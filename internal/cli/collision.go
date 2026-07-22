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
// AGENTS.md is the canonical shared path: Codex, Amp, and Warp all default to
// it (OpenCode defaults to .opencode/AGENTS.md and Zed to .rules, so neither
// contends here). Their entry-point pointer body is byte-identical, so sync
// deduplicates the write and they do not collide. The same rule applies to
// adapter-emitted files: Codex and Amp both emit skills to
// `.agents/skills/<name>/SKILL.md`, and for a spec without divergent
// per-target overrides the bytes match, so the shared write is the dedup the
// tool exists for, not a conflict. A genuine collision is two adapters
// writing DIFFERENT content to one path (e.g. a conflicting
// outputs.<target>.rules-file: AGENTS.md, or a skill whose x-codex/x-amp
// overrides diverge), which would otherwise cause silent last-writer-wins
// and perpetual drift in `sync --check`.
func detectCollisions(cfg *config.Config, b spec.Bundle, targets []string) error {
	policy := cfg.Sync.CollisionPolicy
	if policy == "" {
		policy = "prompt"
	}
	if policy == "prefer-spec" {
		return nil
	}

	owners := map[string][]string{}
	contents := map[string]map[string]bool{}
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		sess.StartCapture()
		if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, false); err != nil {
			sess.StopCapture()
			return fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range sess.StopCapture() {
			owners[f.Path] = append(owners[f.Path], t)
			if contents[f.Path] == nil {
				contents[f.Path] = map[string]bool{}
			}
			contents[f.Path][f.Content] = true
		}
	}
	var lines []string
	for path, ts := range owners {
		// Byte-identical writes from several targets dedupe into one
		// file; only divergent content is a real conflict.
		if len(ts) < 2 || len(contents[path]) < 2 {
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
