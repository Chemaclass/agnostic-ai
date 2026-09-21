package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// AGENTS.md is the canonical shared path: Codex, Amp, Warp, and OpenCode all
// default to it (Zed defaults to .rules, which it reads ahead of AGENTS.md,
// so it does not contend here). Their entry-point pointer body is
// byte-identical, so sync deduplicates the write and they do not collide. The same rule applies to
// adapter-emitted files: Codex and Amp both emit skills to
// `.agents/skills/<name>/SKILL.md`, and for a spec without divergent
// per-target overrides the bytes match, so the shared write is the dedup the
// tool exists for, not a conflict. A genuine collision is two adapters
// writing DIFFERENT content to one path (e.g. a conflicting
// outputs.<target>.rules-file: AGENTS.md, or a skill whose x-codex/x-amp
// overrides diverge), which would otherwise cause silent last-writer-wins
// and perpetual drift in `sync --check`.
func detectCollisions(cfg *config.Config, b spec.Bundle, targets []string) error {
	if err := adapters.ValidateScopedRules(cfg, b, targets); err != nil {
		return err
	}
	readers := append(append([]string(nil), cfg.Targets...), targets...)
	if _, err := renderEntryPointFiles(cfg, b, readers, ""); err != nil {
		return err
	}
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
	warnSharedAgentsTreeReaders(owners, targets)

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

// sharedAgentsTree is the project subagent root several vendors read.
const sharedAgentsTree = ".agents/agents/"

// warnSharedAgentsTreeReaders reports the one overlap the collision
// check above cannot see. Devin reads `.agents/agents/` as well as its
// own `.devin/agents/`, and the targets that own that tree write an
// agent at a different path inside it: antigravity nests
// `<name>/agent.md`, goose and openhands write a flat `<name>.md`. Two
// different paths never collide on content, so one spec quietly becomes
// two profiles claiming the same name, and Devin documents a conflict
// rule only for built-in names.
//
// It stays a warning rather than an error. The configuration is
// legitimate, and the two files cannot be merged into one: `model`
// means a model id to Devin and a closed `inherit`/`flash`/`pro` tier
// to Antigravity, so a single file would have to carry a value one of
// the two vendors never documented. Scope the spec with `target:` to
// silence it (#863). It folds into one line and buffers with the
// coverage notes, so an unchanged overlap is not re-printed every sync.
func warnSharedAgentsTreeReaders(owners map[string][]string, targets []string) {
	if !slices.Contains(targets, "windsurf") {
		return
	}
	files := 0
	writers := map[string]bool{}
	for path, ts := range owners {
		if !strings.HasPrefix(filepath.ToSlash(path), sharedAgentsTree) {
			continue
		}
		counted := false
		for _, t := range ts {
			if t == "windsurf" {
				continue
			}
			writers[t] = true
			if !counted {
				files++
				counted = true
			}
		}
	}
	if files == 0 {
		return
	}
	names := make([]string, 0, len(writers))
	for t := range writers {
		names = append(names, t)
	}
	sort.Strings(names)
	verb := "are"
	if files == 1 {
		verb = "is"
	}
	adapters.NoteProject(fmt.Sprintf("%d agent file%s in %s written by %s %s also read by Devin, which loads them beside its own .devin/agents/ copy; only the .devin/ copy carries allowed-tools (scope the spec with `target:` to write one)",
		files, plural(files), sharedAgentsTree, strings.Join(names, ", "), verb))
}
