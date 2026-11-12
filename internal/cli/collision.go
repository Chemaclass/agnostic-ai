package cli

import (
	"fmt"
	"maps"
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
	return detectRenderedCollisions(newTargetRenders(cfg, b), cfg, targets, ".", readStateFile(".").Backups)
}

// detectRenderedCollisions is detectCollisions over the run's renders,
// for the project at root whose ledger records backups.
func detectRenderedCollisions(renders *targetRenders, cfg *config.Config, targets []string, root string, backups map[string]string) (map[string]string, error) {
	found, err := findRenderedCollisions(renders, cfg, targets, root, backups)
	return found.winners, err
}

// renderedCollisions is what findRenderedCollisions saw: the winners, the
// shared rules-files, and the instructions files the run writes.
type renderedCollisions struct {
	winners  map[string]string
	shared   []sharedRulesFile
	runFiles []entryPointFile
}

func findRenderedCollisions(renders *targetRenders, cfg *config.Config, targets []string, root string, backups map[string]string) (found renderedCollisions, err error) {
	b := renders.b
	cfg = cfg.WithAdditionalTargets(targets...)
	if err := adapters.ValidateScopedRules(cfg, b, targets); err != nil {
		return found, err
	}
	readers := append(append([]string(nil), cfg.Targets...), targets...)
	entryFiles, err := renderEntryPointFiles(cfg, b, readers, "")
	if err != nil {
		return found, err
	}
	shared := sharedRulesFiles(cfg, b, entryFiles)
	found.shared = shared
	var conflicts []string
	for _, s := range shared {
		if !s.kept {
			conflicts = append(conflicts, s.conflict())
		}
	}
	if len(conflicts) > 0 {
		return found, errs.Coded(errs.CodeOutputCollision, "output collision: %s", strings.Join(conflicts, "\n"))
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
	for _, t := range writers {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		render := renders.capture(adapter, cfg)
		if err := render.err; err != nil {
			// The target's own emit reports the error, after the others write.
			if policy == "prefer-spec" {
				continue
			}
			return found, fmt.Errorf("%s: %w", t, err)
		}
		for _, f := range render.files {
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
	runFiles, err := renderEntryPointFiles(cfg, b, targets, "")
	if err != nil {
		return found, err
	}
	found.runFiles = runFiles
	if err := outputPathCollisions(root, cfg, owners, entryFiles, backups); err != nil {
		return found, err
	}
	overwritten := maps.Clone(owners)
	for _, s := range shared {
		for path, ts := range overwritten {
			if filepath.Clean(path) == filepath.Clean(s.path) && slices.Contains(ts, s.owner) {
				adapters.NoteProject(s.noEffect())
				overwritten[path] = slices.DeleteFunc(slices.Clone(ts), func(t string) bool { return t == s.owner })
			}
		}
	}
	if err := entryPointOverwrites(cfg, runFiles, overwritten); err != nil {
		return found, err
	}
	replaced, err := replacedRulesFileWinners(cfg, b, shared)
	if err != nil {
		return found, err
	}
	if policy == "prefer-spec" {
		for path, variants := range contents {
			if len(variants) > 1 {
				if found.winners == nil {
					found.winners = map[string]string{}
				}
				found.winners[filepath.Clean(path)] = last[path]
			}
		}
		noteCollisionWinners(writers, contents, lastOwner, written)
		found.winners = withWinners(found.winners, replaced)
		return found, nil
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
		found.winners = replaced
		return found, nil
	}
	sort.Strings(lines)

	if policy == "fail" {
		return found, errs.Coded(errs.CodeOutputCollision,
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
	return found, errs.Coded(errs.CodeOutputCollision, msg, strings.Join(lines, "\n"))
}

// replacedRulesFileWinners maps each kept shared rules-file to the
// instructions file its configured readers write there. The owner's
// adapter then writes those bytes in every run, so a run of the owner
// alone leaves the file as the full run wrote it, and a later sync finds
// no change there and no hand edit to back up. Only those readers are
// rendered, so another target's instructions file cannot stop the run.
func replacedRulesFileWinners(cfg *config.Config, b spec.Bundle, shared []sharedRulesFile) (map[string]string, error) {
	var out map[string]string
	var body *string
	for _, s := range shared {
		if !s.kept {
			continue
		}
		if body == nil {
			resolved, err := previewAgnosticBody()
			if err != nil {
				return nil, err
			}
			body = &resolved
		}
		files, err := renderEntryPointFiles(cfg, b, append(slices.Clone(s.readers), s.owner), *body)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			if samePath(f.Path, s.path) {
				if out == nil {
					out = map[string]string{}
				}
				out[filepath.Clean(f.Path)] = f.Content
			}
		}
	}
	return out, nil
}

func withWinners(winners, more map[string]string) map[string]string {
	if len(more) == 0 {
		return winners
	}
	if winners == nil {
		winners = map[string]string{}
	}
	maps.Copy(winners, more)
	return winners
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

// entryPointOverwrites stops a run in which a target's adapter writes a
// file that sync then writes again as another target's instructions file.
// The collision policy cannot settle it: sync writes instructions files
// after every adapter, so the adapter's version would never survive.
func entryPointOverwrites(cfg *config.Config, files []entryPointFile, owners map[string][]string) error {
	byPath := map[string][]string{}
	for path, ts := range owners {
		byPath[filepath.Clean(path)] = ts
	}
	var lines []string
	for _, f := range files {
		ts := byPath[filepath.Clean(f.Path)]
		if len(ts) == 0 || cfg.IsUnmanaged(f.Path) {
			continue
		}
		ts = slices.Clone(ts)
		sort.Strings(ts)
		lines = append(lines, fmt.Sprintf("  %s ← %s, and the instructions file of %s", f.Path, strings.Join(ts, ", "), strings.Join(f.Readers, ", ")))
	}
	if len(lines) == 0 {
		return nil
	}
	sort.Strings(lines)
	return errs.Coded(errs.CodeOutputCollision,
		"output collision: an adapter writes a file sync also writes as an instructions file\n%s\n"+
			"point that output elsewhere, or set outputs.<target>.file for the readers", strings.Join(lines, "\n"))
}

// outputPathCollisions refuses outputs that would destroy a hand edit:
// an output at another output's `<path>.bak`, where sync saves a hand
// edit to the first, and an output on a backup the ledger records, which
// holds a saved edit. Backup paths match without regard to case, since
// some filesystems store both spellings as one file.
func outputPathCollisions(root string, cfg *config.Config, owners map[string][]string, entryFiles []entryPointFile, backups map[string]string) error {
	names := map[string][]string{}
	add := func(path, name string) {
		key := filepath.Clean(filepath.FromSlash(path))
		if !slices.Contains(names[key], name) {
			names[key] = append(names[key], name)
		}
	}
	for key, path := range cfg.ConfiguredOutputPaths() {
		if !directorySetting(key) {
			add(path, key)
		}
	}
	for path, ts := range owners {
		for _, t := range ts {
			add(path, t)
		}
	}
	for _, f := range entryFiles {
		add(f.Path, "the instructions file of "+strings.Join(f.Readers, ", "))
	}
	spellings := map[string][]string{}
	for path := range names {
		key := strings.ToLower(path)
		spellings[key] = append(spellings[key], path)
	}
	label := func(path string) string {
		ns := slices.Sorted(slices.Values(names[path]))
		return fmt.Sprintf("%s (%s)", filepath.ToSlash(path), strings.Join(ns, ", "))
	}
	var lines []string
	for path := range names {
		for _, backup := range spellings[strings.ToLower(path+".bak")] {
			lines = append(lines, fmt.Sprintf("  %s is the backup sync saves for %s", label(backup), label(path)))
		}
	}
	if len(lines) > 0 {
		sort.Strings(lines)
		return errs.Coded(errs.CodeOutputCollision,
			"output collision: an output sits where sync saves another output's hand edit\n%s\n"+
				"point one of them elsewhere", strings.Join(lines, "\n"))
	}
	for bak := range backups {
		if filepath.IsAbs(bak) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(root, bak)); err != nil {
			continue
		}
		for _, path := range spellings[strings.ToLower(filepath.Clean(filepath.FromSlash(bak)))] {
			lines = append(lines, fmt.Sprintf("  %s is a backup sync saved for %s", label(path), filepath.ToSlash(strings.TrimSuffix(bak, ".bak"))))
		}
	}
	if len(lines) == 0 {
		return nil
	}
	sort.Strings(lines)
	return errs.Coded(errs.CodeOutputCollision,
		"output collision: an output sits on a backup that holds a saved hand edit\n%s\n"+
			"move or delete that backup once you have kept what you need from it, or point the output elsewhere", strings.Join(lines, "\n"))
}

// directorySetting reports whether an outputs.<target>.<key> names a
// folder. Sync writes files beneath it and never backs the folder up.
func directorySetting(key string) bool {
	return strings.HasSuffix(key, ".dir") || strings.HasSuffix(key, "-dir")
}
