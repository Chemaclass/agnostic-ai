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
//   - prefer-spec: the last target in `targets:` that writes the path wins.
//     The returned winners map each contested path to that content; sync
//     writes it and `sync --check` expects it (Session.SetCollisionWinners).
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
func detectCollisions(cfg *config.Config, b spec.Bundle, targets []string) (winners map[string]string, err error) {
	cfg = cfg.WithAdditionalTargets(targets...)
	if err := adapters.ValidateScopedRules(cfg, b, targets); err != nil {
		return nil, err
	}
	readers := append(append([]string(nil), cfg.Targets...), targets...)
	if _, err := renderEntryPointFiles(cfg, b, readers, ""); err != nil {
		return nil, err
	}
	policy := cfg.Sync.CollisionPolicy
	if policy == "" {
		policy = "prompt"
	}
	// The winner must not depend on which targets one run selects, so
	// prefer-spec renders every configured target.
	writers := targets
	if policy == "prefer-spec" {
		writers = cfg.Targets
	}

	owners := map[string][]string{}
	contents := map[string]map[string]bool{}
	last := map[string]string{}
	lastOwner := map[string]string{}
	written := map[string]map[string]string{}
	sess := adapters.NewSession()
	for _, t := range writers {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		sess.StartCapture()
		if err := adapters.EmitWithProvenance(sess, adapter, b, cfg, false); err != nil {
			sess.StopCapture()
			// The target's own emit reports the error, after the others write.
			if policy == "prefer-spec" {
				continue
			}
			return nil, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range sess.StopCapture() {
			owners[f.Path] = append(owners[f.Path], t)
			if contents[f.Path] == nil {
				contents[f.Path] = map[string]bool{}
			}
			contents[f.Path][f.Content] = true
			last[f.Path] = f.Content
			lastOwner[f.Path] = t
			if written[f.Path] == nil {
				written[f.Path] = map[string]string{}
			}
			written[f.Path][t] = f.Content
		}
	}
	if policy == "prefer-spec" {
		for path, variants := range contents {
			if len(variants) > 1 {
				if winners == nil {
					winners = map[string]string{}
				}
				winners[filepath.Clean(path)] = last[path]
			}
		}
		noteCollisionWinners(writers, contents, lastOwner, written)
		return winners, nil
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
		return nil, nil
	}
	sort.Strings(lines)

	if policy == "fail" {
		return nil, errs.Coded(errs.CodeOutputCollision,
			"output collision: targets emit to the same path\n%s",
			strings.Join(lines, "\n"))
	}

	// prompt: error with resolution hint
	msg := "output collision: targets emit to the same path\n%s\n" +
		"resolve by dropping one from `targets:` in agnostic-ai.yaml, " +
		"or override the matching `outputs.<target>` path setting for that output"
	if !term.IsTerminal(os.Stdin.Fd()) {
		msg += "\nfor CI use: set `sync.collision-policy: prefer-spec` in agnostic-ai.yaml"
	}
	return nil, errs.Coded(errs.CodeOutputCollision, msg, strings.Join(lines, "\n"))
}

// noteCollisionWinners names, per winning target, the contested paths it won
// and the targets whose different version it overrode, so a reorder of
// `targets:` is the visible way to change the outcome.
func noteCollisionWinners(order []string, contents map[string]map[string]bool, lastOwner map[string]string, written map[string]map[string]string) {
	type won struct {
		paths      []string
		overridden map[string]bool
	}
	byWinner := map[string]*won{}
	for path, variants := range contents {
		if len(variants) < 2 {
			continue
		}
		winner := lastOwner[path]
		w := byWinner[winner]
		if w == nil {
			w = &won{overridden: map[string]bool{}}
			byWinner[winner] = w
		}
		w.paths = append(w.paths, filepath.ToSlash(filepath.Clean(path)))
		for t, content := range written[path] {
			if content != written[path][winner] {
				w.overridden[t] = true
			}
		}
	}
	for _, winner := range order {
		w := byWinner[winner]
		if w == nil {
			continue
		}
		delete(byWinner, winner)
		sort.Strings(w.paths)
		examples := w.paths[:min(len(w.paths), maxCollisionExamples)]
		list := strings.Join(examples, ", ")
		if len(w.paths) > len(examples) {
			list += ", ..."
		}
		var losers []string
		for _, t := range order {
			if w.overridden[t] {
				losers = append(losers, t)
			}
		}
		noun := "shared file uses"
		if len(w.paths) > 1 {
			noun = "shared files use"
		}
		adapters.NoteProject(fmt.Sprintf("prefer-spec: %d %s %s's version over %s (%s); reorder targets to change it",
			len(w.paths), noun, winner, strings.Join(losers, ", "), list))
	}
}

// maxCollisionExamples caps the paths a prefer-spec note lists per winner.
const maxCollisionExamples = 3

// sharedAgentsTree is the project subagent root several vendors read.
const sharedAgentsTree = ".agents/agents/"

// Devin discovers shared profiles even when target filtering omits its native copy.
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
	adapters.NoteProject(fmt.Sprintf("%d agent file%s in %s written by %s %s also read by Devin; shared profiles may lack allowed-tools restrictions (check discovered profiles for duplicate names)",
		files, plural(files), sharedAgentsTree, strings.Join(names, ", "), verb))
}
