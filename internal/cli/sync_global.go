package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	globalStart       = "<!-- agnostic-ai:global:start -->"
	globalEnd         = "<!-- agnostic-ai:global:end -->"
	envUserGlobalRoot = "AGNOSTIC_AI_HOME"
	defaultUserGlobal = ".agnostic-ai"
	// Version 5 records a content sum per owned file to catch hand edits.
	globalStateVersion = 5
)

type globalSyncOptions struct {
	targets, only, except                                                    []string
	dryRun, check, backup, plan, watch, watchPoll, jsonOut, allTargets, diff bool
	format, gitignore                                                        string
	jobs                                                                     int
}

type globalState struct {
	Version int                 `json:"version"`
	Files   []string            `json:"files"`
	Agents  map[string][]string `json:"agents,omitempty"`
	// Skills records the files each target placed under its skills
	// directory. Several targets share ~/.agents/skills/, so a sync of
	// one must keep what the others placed there.
	Skills map[string][]string `json:"skills,omitempty"`
	// Sums maps each owned file to the sum of what sync last wrote there,
	// covering only the managed block of an instructions file.
	Sums map[string]string `json:"sums,omitempty"`
	// AgentEfforts records the effortLevel written per target and agent
	// name, so a later sync removes only values it placed.
	AgentEfforts map[string]map[string]string `json:"agentEfforts,omitempty"`
	// Hooks records the managed hook entries per target, keyed by
	// target name then event, so a later sync can remove exactly what
	// it added and leave user-authored entries alone.
	Hooks map[string]map[string][]any `json:"hooks,omitempty"`
	// ClaudeHooks and CursorHooks are the version-1 layout, read for
	// migration only.
	ClaudeHooks map[string][]any `json:"claudeHooks,omitempty"`
	CursorHooks map[string][]any `json:"cursorHooks,omitempty"`
}

type globalWrite struct {
	path string
	data []byte
	mode fs.FileMode
	// owned marks a file sync writes whole, or a managed block in it.
	owned bool
	// dropsComments marks a JSONC rewrite that loses the file's comments.
	dropsComments bool
}

func runGlobalSync(cmd *cobra.Command, o globalSyncOptions) error {
	if o.watch || o.watchPoll || o.plan || o.jsonOut || o.allTargets || o.gitignore != "" || o.jobs != 0 {
		return errs.Coded(errs.CodeFlagConflict, "--global does not support --watch, --watch-poll, --plan, --json, --all, --gitignore, or --jobs")
	}
	if o.diff && !o.check {
		return errs.Coded(errs.CodeFlagConflict, "--diff requires --check with --global")
	}
	if len(o.only) > 0 && len(o.except) > 0 {
		return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
	}
	if err := validateCheckFormat(o.format); err != nil {
		return err
	}
	if o.format != checkFormatHuman && !o.check {
		return errs.Coded(errs.CodeFlagConflict, "--format requires --check with --global")
	}
	targets := o.targets
	// A default run spans every supported target, so one target's
	// problem (a relative root variable, a name its native format
	// rejects) warns and skips that target instead of failing the rest.
	explicit := len(o.targets) > 0 || len(o.only) > 0
	if len(targets) == 0 {
		targets = globalTargetNames()
	}
	var err error
	targets, err = filterTargets(targets, o.only, o.except)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if _, ok := globalTargets[target]; !ok {
			return fmt.Errorf("--global: unsupported target %q (supported: %s)", target, strings.Join(globalTargetNames(), ", "))
		}
	}

	home, err := globalUserHome()
	if err != nil {
		return err
	}
	warn := cmd.ErrOrStderr()
	if verbosity < levelDefault || o.check {
		warn = io.Discard
	}
	var usable []string
	for _, target := range targets {
		err := globalTargets[target].rootError(target)
		if err == nil {
			usable = append(usable, target)
			continue
		}
		if explicit {
			return err
		}
		if _, werr := fmt.Fprintf(warn, "warning: %v; skipping %s\n", err, target); werr != nil {
			return fmt.Errorf("write global target warning: %w", werr)
		}
	}
	targets = usable
	source := globalSourceHome(home)
	bundle, err := spec.LoadLayered(globalLayers(source))
	if err != nil {
		return err
	}
	for _, target := range targets {
		if globalTargets[target].agents != "" {
			continue
		}
		var skipped []string
		for _, agent := range bundle.Agents {
			if agent.EmitsTo(target) {
				skipped = append(skipped, agent.Name)
			}
		}
		if len(skipped) == 0 {
			continue
		}
		if _, err := fmt.Fprintf(warn, "warning: %s: global agents are unsupported; skipping %s\n", target, strings.Join(skipped, ", ")); err != nil {
			return fmt.Errorf("write global agent warning: %w", err)
		}
	}
	for _, rule := range bundle.Rules {
		if rule.Scope != "" || hasGlobalRuleCondition(rule.Meta) {
			return fmt.Errorf("global rule %q is scoped or conditional; global rules must apply unconditionally", rule.Name)
		}
	}
	instructions, err := globalInstructions(source, bundle.Rules)
	if err != nil {
		return err
	}

	statePath := filepath.Join(source, "state", "global.json")
	old, err := loadGlobalState(statePath)
	if err != nil {
		return err
	}
	adapters.ResetCoverageNotes()
	adapters.SetWarner(warn)
	defer adapters.SetWarner(os.Stderr)
	defer adapters.ResetCoverageNotes()
	writes, next, err := buildGlobalWrites(home, source, targets, instructions, bundle, old, agentFailure(explicit, warn))
	if err != nil {
		return err
	}
	adapters.FlushCoverageNotes()
	if err := checkUnselectedGlobalAgents(writes, old, targets); err != nil {
		return err
	}
	stateData, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", statePath, err)
	}
	writes = append(writes, globalWrite{path: statePath, data: append(stateData, '\n'), mode: 0o644})
	var trees []string
	for _, target := range targets {
		trees = append(trees, globalTargets[target].trees(home)...)
	}
	if err := preflightGlobalWrites(writes, trees, old); err != nil {
		return err
	}
	removals := removedGlobalFiles(old.Files, next.Files)
	if o.check {
		return checkGlobalWrites(cmd, writes, existingPaths(removals), statePath, next, o.diff)
	}
	if o.dryRun {
		for _, w := range writes {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: write %s\n", w.path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		for _, path := range existingPaths(removals) {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: remove %s\n", path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		return nil
	}
	if !o.backup {
		edited, err := handEditedGlobalFiles(writes, removals, old)
		if err != nil {
			return err
		}
		if len(edited) > 0 {
			return fmt.Errorf("edited since the last global sync: %s; move the edit into %s, or rerun with --backup to overwrite and keep a .bak copy", strings.Join(edited, ", "), source)
		}
		var commented []string
		for _, w := range writes {
			if w.dropsComments {
				commented = append(commented, w.path)
			}
		}
		if len(commented) > 0 {
			return fmt.Errorf("rewriting %s would drop its comments; rerun with --backup to rewrite and keep a .bak copy", strings.Join(commented, ", "))
		}
	}
	if err := applyGlobalChanges(writes, removals, o.backup); err != nil {
		return err
	}
	pruneEmptyGlobalDirs(removals, trees)
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Synced global configuration to %d target(s).\n", len(targets)); err != nil {
		return fmt.Errorf("write sync summary: %w", err)
	}
	return nil
}

// checkGlobalWrites reports every planned write that differs from disk
// and every file a sync would remove. With diff it prints a unified diff
// per file, limited to the managed block when both sides carry one.
func checkGlobalWrites(cmd *cobra.Command, writes []globalWrite, removals []string, statePath string, next globalState, diff bool) error {
	var drifted []string
	out := cmd.OutOrStdout()
	for _, w := range writes {
		if w.path == statePath {
			if globalStateCurrent(statePath, next) {
				continue
			}
			drifted = append(drifted, w.path)
			if diff {
				if _, err := fmt.Fprintf(out, "would update ownership state %s\n", filepath.ToSlash(w.path)); err != nil {
					return fmt.Errorf("write global diff: %w", err)
				}
			}
			continue
		}
		data, err := os.ReadFile(w.path)
		if err == nil && bytes.Equal(data, w.data) {
			continue
		}
		drifted = append(drifted, w.path)
		if !diff {
			continue
		}
		if err != nil {
			if _, werr := fmt.Fprintf(out, "would create %s (%d bytes)\n", filepath.ToSlash(w.path), len(w.data)); werr != nil {
				return fmt.Errorf("write global diff: %w", werr)
			}
			continue
		}
		have, want := globalBlock(data), globalBlock(w.data)
		if have == want {
			have, want = string(data), string(w.data)
		}
		if _, werr := fmt.Fprint(out, unifiedDiff(w.path, have, want, diffBodyMax)); werr != nil {
			return fmt.Errorf("write global diff: %w", werr)
		}
	}
	for _, path := range removals {
		drifted = append(drifted, path)
		if diff {
			if _, err := fmt.Fprintf(out, "would remove %s\n", filepath.ToSlash(path)); err != nil {
				return fmt.Errorf("write global diff: %w", err)
			}
		}
	}
	if len(drifted) > 0 {
		return fmt.Errorf("global configuration drift: %s", strings.Join(drifted, ", "))
	}
	return nil
}

// existingPaths keeps the paths still on disk, the only ones a removal
// changes.
func existingPaths(paths []string) []string {
	var out []string
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			out = append(out, path)
		}
	}
	return out
}

// agentFailure decides what a target's agent render error does: an
// explicitly selected target fails the run, while a default run warns
// and keeps that target's previously synced agents.
func agentFailure(explicit bool, warn io.Writer) func(target string, err error) error {
	return func(target string, err error) error {
		if explicit {
			return err
		}
		if _, werr := fmt.Fprintf(warn, "warning: %v; skipping %s agents\n", err, target); werr != nil {
			return fmt.Errorf("write global agent warning: %w", werr)
		}
		return nil
	}
}

// globalStateCurrent reports whether the recorded state already
// describes next. It compares normalized content, so an older state
// version holding the same ownership is not drift. Sums are left out:
// they mirror file content, which check compares directly.
func globalStateCurrent(path string, next globalState) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	recorded, err := loadGlobalState(path)
	if err != nil {
		return false
	}
	recorded.Sums, next.Sums = nil, nil
	a, errA := json.Marshal(recorded)
	b, errB := json.Marshal(next)
	return errA == nil && errB == nil && bytes.Equal(a, b)
}

// pruneEmptyGlobalDirs removes directories a removal left empty, such as
// a deleted skill folder or an Antigravity agent folder, stopping at the
// managed tree root. Best effort: a directory holding anything stays.
func pruneEmptyGlobalDirs(removals, trees []string) {
	for _, path := range removals {
		for dir := filepath.Dir(path); inManagedTree(dir, trees); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
	}
}

func hasGlobalRuleCondition(meta map[string]any) bool {
	for _, key := range []string{"scope", "globs", "paths", "target", "targets", "target-exclude", "targets-exclude"} {
		if _, ok := meta[key]; ok {
			return true
		}
	}
	return false
}

func loadGlobalState(path string) (globalState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return globalState{Version: globalStateVersion}, nil
	}
	if err != nil {
		return globalState{}, fmt.Errorf("read %s: %w", path, err)
	}
	var state globalState
	if err := json.Unmarshal(data, &state); err != nil || state.Version < 1 || state.Version > globalStateVersion {
		return globalState{}, fmt.Errorf("parse %s: corrupt global ownership state", path)
	}
	if state.Hooks == nil {
		state.Hooks = map[string]map[string][]any{}
	}
	for target, hooks := range map[string]map[string][]any{"claude": state.ClaudeHooks, "cursor": state.CursorHooks} {
		if len(hooks) > 0 && state.Hooks[target] == nil {
			state.Hooks[target] = hooks
		}
	}
	state.ClaudeHooks, state.CursorHooks = nil, nil
	state.Version = globalStateVersion
	return state, nil
}

func buildGlobalWrites(home, source string, targets []string, intro []byte, b spec.Bundle, old globalState, agentErr func(string, error) error) ([]globalWrite, globalState, error) {
	next := globalState{Version: globalStateVersion, Files: append([]string(nil), old.Files...), Hooks: map[string]map[string][]any{}, Agents: map[string][]string{}, Skills: map[string][]string{}, AgentEfforts: map[string]map[string]string{}}
	for target, paths := range old.Agents {
		if !slices.Contains(targets, target) {
			next.Agents[target] = append([]string(nil), paths...)
		}
	}
	for target, paths := range old.Skills {
		if !slices.Contains(targets, target) {
			next.Skills[target] = append([]string(nil), paths...)
		}
	}
	for target, efforts := range old.AgentEfforts {
		if !slices.Contains(targets, target) {
			next.AgentEfforts[target] = efforts
		}
	}
	for target, hooks := range old.Hooks {
		next.Hooks[target] = cloneHookState(hooks)
	}
	// Drop every surface the synced targets own before emitting any of
	// them, so a file the sources no longer produce is swept rather
	// than kept alive by its own prior record. Targets left out of this
	// run keep theirs. This runs before the emit loop because several
	// targets share one skills directory.
	for _, target := range targets {
		g := globalTargets[target]
		next.Files = removePaths(next.Files, old.Agents[target])
		next.Files = removePaths(next.Files, old.Skills[target])
		for _, tree := range g.trees(home) {
			// A state without per-target skill records predates them,
			// so its skills tree is swept whole, as before.
			if old.Skills != nil && g.skills != "" && tree == g.path(home, g.skills) {
				continue
			}
			next.Files = removePathPrefix(next.Files, tree+string(filepath.Separator))
		}
		next.Files = removePaths(next.Files, g.files(home))
	}
	var writes []globalWrite
	seen := map[string]int{}
	place := func(path string, data []byte, mode fs.FileMode) (bool, error) {
		if i, ok := seen[path]; ok {
			if !bytes.Equal(writes[i].data, data) {
				return false, fmt.Errorf("%s: two global targets emit different content to one path", path)
			}
			return false, nil
		}
		seen[path] = len(writes)
		writes = append(writes, globalWrite{path: path, data: data, mode: mode})
		return true, nil
	}
	add := func(path string, data []byte, mode fs.FileMode) error {
		placed, err := place(path, data, mode)
		if placed {
			writes[len(writes)-1].owned = true
			next.Files = append(next.Files, path)
		}
		return err
	}
	body := strings.TrimSpace(string(intro))
	managed := globalStart + "\n" + body + "\n" + globalEnd
	for _, target := range targets {
		g := globalTargets[target]
		if g.agents != "" {
			dir := g.agentsPath(home)
			files, renderErr := adapters.RenderAgents(target, b.Agents, dir)
			if renderErr != nil {
				if err := agentErr(target, renderErr); err != nil {
					return nil, next, err
				}
				// Keep what the last successful sync placed.
				next.Agents[target] = append([]string(nil), old.Agents[target]...)
			}
			for _, file := range files {
				if err := add(file.Path, []byte(file.Content), 0o644); err != nil {
					return nil, next, err
				}
				next.Agents[target] = append(next.Agents[target], file.Path)
			}
			if g.agentEfforts != "" {
				efforts := old.AgentEfforts[target]
				if renderErr == nil {
					var err error
					if efforts, err = adapters.AgentEffortLevels(target, b.Agents); err != nil {
						return nil, next, err
					}
				}
				// The settings file is the user's own, so ownership is per
				// key and the file never enters Files or removal.
				path := g.path(home, g.agentEfforts)
				doc, dropsComments, err := mergeGlobalAgentEfforts(path, efforts, old.AgentEfforts[target])
				if err != nil {
					return nil, next, err
				}
				if doc != nil {
					placed, err := place(path, doc, 0o644)
					if err != nil {
						return nil, next, err
					}
					if placed {
						writes[len(writes)-1].dropsComments = dropsComments
					}
				}
				if len(efforts) > 0 {
					next.AgentEfforts[target] = efforts
				}
			}
		}
		if g.instructions != "" {
			path := g.path(home, g.instructions)
			merged, err := mergeGlobalBlock(path, managed)
			if err != nil {
				return nil, next, err
			}
			// An empty merge means no global instructions, no global
			// rules, and no user text left around the managed block.
			// Emit nothing so the file is never seeded, and so one
			// already recorded is swept instead of left empty.
			if merged != "" {
				if err := add(path, []byte(merged), 0o644); err != nil {
					return nil, next, err
				}
			}
		}
		if g.rules != "" {
			dir := g.path(home, g.rules)
			for _, rule := range b.Rules {
				out := filepath.Join(dir, rule.Name+".md")
				if err := add(out, []byte(strings.TrimSpace(rule.Body)+"\n"), 0o644); err != nil {
					return nil, next, err
				}
			}
		}
		if g.skills != "" {
			adapters.NoteDroppedSkillFields(target, b.Skills)
			dir := g.path(home, g.skills)
			if err := adapters.NoteManualOnlySkillDrops(target, b.Skills, sharedGlobalSkillsDir(home, dir)); err != nil {
				return nil, next, err
			}
			addSkill := func(path string, data []byte, mode fs.FileMode) error {
				if err := add(path, data, mode); err != nil {
					return err
				}
				next.Skills[target] = append(next.Skills[target], path)
				return nil
			}
			for _, skill := range b.Skills {
				if !skill.EmitsTo(target) {
					continue
				}
				overlays, err := globalSkillOverlays(home, dir, skill)
				if err != nil {
					return nil, next, err
				}
				if err := addGlobalSkill(filepath.Join(dir, skill.Name), skill, target, sharedGlobalSkillsDir(home, dir), overlays, addSkill); err != nil {
					return nil, next, err
				}
			}
		}
		if g.hooks == "" {
			continue
		}
		next.Hooks[target] = map[string][]any{}
		path := g.path(home, g.hooks)
		hooks := b.HooksFor(target)
		if g.bridge && body != "" {
			bridge, command, script, mode := globalContextBridge(filepath.Dir(path), body, g.bridgeKey)
			if err := add(bridge, []byte(script), mode); err != nil {
				return nil, next, err
			}
			hooks = append(append([]spec.Entry{}, hooks...), spec.Entry{Meta: map[string]any{"event": g.bridgeEvent, "command": command}})
		}
		doc, err := mergeGlobalHooks(path, g.hooksFormat, hooks, old.Hooks[target], next.Hooks[target])
		if errors.Is(err, errGlobalFileUnchanged) {
			if slices.Contains(old.Files, path) {
				next.Files = append(next.Files, path)
			}
			continue
		}
		if err != nil {
			return nil, next, err
		}
		if doc != nil {
			// Ownership of a hooks file is per entry, so it carries no sum.
			placed, err := place(path, doc, 0o644)
			if err != nil {
				return nil, next, err
			}
			if placed {
				next.Files = append(next.Files, path)
			}
		}
	}
	for _, paths := range next.Agents {
		next.Files = append(next.Files, paths...)
	}
	for _, paths := range next.Skills {
		next.Files = append(next.Files, paths...)
	}
	sort.Strings(next.Files)
	next.Files = slices.Compact(next.Files)
	next.Sums = map[string]string{}
	for _, path := range next.Files {
		if sum, ok := old.Sums[path]; ok {
			next.Sums[path] = sum
		}
	}
	for _, w := range writes {
		delete(next.Sums, w.path)
		if w.owned {
			next.Sums[w.path] = globalSum(w.data)
		}
	}
	return writes, next, nil
}

// globalSum fingerprints what sync owns in a file: the managed block
// when the file has one, otherwise the whole content.
func globalSum(data []byte) string {
	return adapters.ContentSum(globalBlock(data))
}

// globalBlock returns the managed block when data carries one, otherwise
// all of data.
func globalBlock(data []byte) string {
	body := string(data)
	start, end := strings.Index(body, globalStart), strings.Index(body, globalEnd)
	if start >= 0 && end > start {
		return body[start : end+len(globalEnd)]
	}
	return body
}

// handEditedGlobalFiles lists owned files whose content no longer
// matches what the last sync wrote and that this run would overwrite or
// remove. A file whose record predates sums is never listed.
func handEditedGlobalFiles(writes []globalWrite, removals []string, old globalState) ([]string, error) {
	var edited []string
	check := func(path string, planned []byte) error {
		recorded, ok := old.Sums[path]
		if !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		current := globalSum(data)
		if current != recorded && (planned == nil || current != globalSum(planned)) {
			edited = append(edited, path)
		}
		return nil
	}
	for _, w := range writes {
		if err := check(w.path, w.data); err != nil {
			return nil, err
		}
	}
	for _, path := range removals {
		if err := check(path, nil); err != nil {
			return nil, err
		}
	}
	return edited, nil
}

func sharedGlobalSkillsDir(home, dir string) bool {
	readers := 0
	for _, target := range globalTargets {
		if target.skills != "" && target.path(home, target.skills) == dir {
			readers++
			if readers > 1 {
				return true
			}
		}
	}
	return false
}

// globalSkillOverlays lists the relative paths any target reading dir
// renders beside SKILL.md for skill. A bundled asset at one of those
// paths never ships there, so every co-writer of a shared tree agrees
// and the spec-rendered file wins, as in project sync.
func globalSkillOverlays(home, dir string, skill spec.Entry) (map[string]bool, error) {
	overlays := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(globalTargets)) {
		g := globalTargets[name]
		if g.skills == "" || g.path(home, g.skills) != dir || !skill.EmitsTo(name) {
			continue
		}
		sidecars, err := adapters.RenderSkillSidecars(name, skill)
		if err != nil {
			return nil, err
		}
		for rel := range sidecars {
			overlays[rel] = true
		}
	}
	return overlays, nil
}

func addGlobalSkill(dst string, skill spec.Entry, target string, shared bool, overlays map[string]bool, add func(string, []byte, fs.FileMode) error) error {
	rendered, err := adapters.RenderSkillMarkdown(target, skill, shared)
	if err != nil {
		return err
	}
	if err := add(filepath.Join(dst, "SKILL.md"), []byte(rendered), 0o644); err != nil {
		return err
	}
	sidecars, err := adapters.RenderSkillSidecars(target, skill)
	if err != nil {
		return err
	}
	for _, rel := range slices.Sorted(maps.Keys(sidecars)) {
		if err := add(filepath.Join(dst, rel), []byte(sidecars[rel]), 0o644); err != nil {
			return err
		}
	}
	if filepath.Base(skill.Path) != "SKILL.md" {
		return nil
	}
	root := filepath.Dir(skill.Path)
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || path == skill.Path {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if overlays[rel] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}
		return add(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
}

func mergeGlobalBlock(path, managed string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	body := string(data)
	start, end := strings.Index(body, globalStart), strings.Index(body, globalEnd)
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return "", fmt.Errorf("%s: corrupt agnostic-ai managed block", path)
	}
	if start >= 0 {
		end += len(globalEnd)
		body = strings.TrimSpace(body[:start] + body[end:])
	}
	body = strings.TrimSpace(body)
	if strings.TrimSpace(managed) == globalStart+"\n\n"+globalEnd {
		if body == "" {
			return "", nil
		}
		// The file is now the user's text alone. Keep it a well-formed
		// text file rather than handing it back without its newline.
		return body + "\n", nil
	}
	if body != "" {
		body += "\n\n"
	}
	return body + managed + "\n", nil
}

// errGlobalFileUnchanged reports that a merge leaves an existing file's
// content as it is, so sync must neither rewrite nor remove it.
var errGlobalFileUnchanged = errors.New("global file unchanged")

// mergeGlobalHooks returns the hooks file with the managed entries
// replaced, nil when the file should not exist, or errGlobalFileUnchanged
// when its parsed content would not change. A rewrite keeps the file's
// key order and indent.
func mergeGlobalHooks(path, format string, entries []spec.Entry, previous map[string][]any, next map[string][]any) ([]byte, error) {
	doc := map[string]any{}
	before := map[string]any{}
	ordered := adapters.NewOrderedJSON()
	data, err := os.ReadFile(path)
	absent := os.IsNotExist(err)
	if err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := json.Unmarshal(data, &before); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := json.Unmarshal(data, ordered); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !absent {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	// Nothing managed, nothing previously managed, no file: leave the
	// user's home directory untouched rather than seeding an empty one.
	if absent && len(entries) == 0 && len(previous) == 0 {
		return nil, nil
	}
	hooks, _ := doc["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for event, oldEntries := range previous {
		current, _ := hooks[event].([]any)
		for _, oldEntry := range oldEntries {
			var found bool
			current, found = removeEqual(current, oldEntry)
			if !found {
				return nil, fmt.Errorf("%s: managed hook ownership is corrupt for %s", path, event)
			}
		}
		if len(current) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = current
		}
	}
	for _, entry := range entries {
		event, _ := entry.Meta["event"].(string)
		if event == "" {
			continue
		}
		for _, command := range globalHookCommands(entry.Meta["command"]) {
			var item any
			if format == "claude" {
				commandHook := map[string]any{"type": "command", "command": command}
				for _, key := range []string{"timeout", "statusMessage", "async", "asyncRewake", "shell", "if", "once"} {
					if value, ok := entry.Meta[key]; ok {
						commandHook[key] = value
					}
				}
				matcher, _ := entry.Meta["matcher"].(string)
				item = map[string]any{"matcher": matcher, "hooks": []any{commandHook}}
			} else {
				cursorHook := map[string]any{"command": command}
				for _, key := range []string{"matcher", "timeout", "loop_limit", "failClosed"} {
					if value, ok := entry.Meta[key]; ok {
						cursorHook[key] = value
					}
				}
				item = cursorHook
			}
			current, _ := hooks[event].([]any)
			hooks[event] = append(current, item)
			next[event] = append(next[event], item)
		}
	}
	if len(hooks) > 0 {
		doc["hooks"] = hooks
	} else {
		delete(doc, "hooks")
	}
	// Nothing of ours left and nothing of the user's either: drop the
	// file rather than leave a shell behind. Cursor's schema version is
	// ours as well, so an earlier run's copy of it is not user content.
	remaining := len(doc)
	if format == "cursor" {
		if version, ok := doc["version"]; ok && version == float64(1) {
			remaining--
		}
	}
	if remaining == 0 {
		return nil, nil
	}
	if format == "cursor" {
		doc["version"] = float64(1)
	}
	if !absent {
		// Round-trip so spec integers compare equal to decoded JSON numbers.
		raw, err := json.Marshal(doc)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", path, err)
		}
		var merged map[string]any
		if err := json.Unmarshal(raw, &merged); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if reflect.DeepEqual(merged, before) {
			return nil, errGlobalFileUnchanged
		}
	}
	for _, key := range []string{"version", "hooks"} {
		value, ok := doc[key]
		if !ok {
			ordered.Delete(key)
			continue
		}
		if err := ordered.Set(key, value); err != nil {
			return nil, fmt.Errorf("marshal %s: %w", path, err)
		}
	}
	out, err := adapters.MarshalJSONIndentWith(ordered, adapters.DetectJSONIndent(data))
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", path, err)
	}
	return append(out, '\n'), nil
}

// mergeGlobalAgentEfforts sets subagents.agents.<name>.effortLevel for
// each managed effort in a JSONC user settings file, removing values an
// earlier sync placed. A value set by hand stops the run. It returns nil
// when the file needs no change; a rewrite keeps key order and indent,
// and dropsComments reports that it loses the file's comments.
func mergeGlobalAgentEfforts(path string, efforts, previous map[string]string) (doc []byte, dropsComments bool, err error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if len(efforts) == 0 {
			return nil, false, nil
		}
		data = []byte("{}")
	} else if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", path, err)
	}
	stripped, hadComments := adapters.StripJSONC(data)
	var before map[string]any
	if err := json.Unmarshal(stripped, &before); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	root := adapters.NewOrderedJSON()
	if err := json.Unmarshal(stripped, root); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	subagents, _ := orderedChild(root, "subagents")
	agents, _ := orderedChild(subagents, "agents")
	for name, level := range previous {
		agent, isObject := orderedChild(agents, name)
		if !isObject {
			continue
		}
		if current, _ := orderedValue(agent, "effortLevel"); current == level {
			agent.Delete("effortLevel")
		}
		if err := putOrderedChild(agents, name, agent); err != nil {
			return nil, false, fmt.Errorf("marshal %s: %w", path, err)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(efforts)) {
		level := efforts[name]
		agent, _ := orderedChild(agents, name)
		if current, ok := orderedValue(agent, "effortLevel"); ok && current != level {
			return nil, false, fmt.Errorf("%s: subagents.agents.%s.effortLevel is %v, set outside agnostic-ai; remove it or drop the agent's effort", path, name, current)
		}
		if err := agent.Set("effortLevel", level); err != nil {
			return nil, false, fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := putOrderedChild(agents, name, agent); err != nil {
			return nil, false, fmt.Errorf("marshal %s: %w", path, err)
		}
	}
	if err := putOrderedChild(subagents, "agents", agents); err != nil {
		return nil, false, fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := putOrderedChild(root, "subagents", subagents); err != nil {
		return nil, false, fmt.Errorf("marshal %s: %w", path, err)
	}
	raw, err := json.Marshal(root)
	if err != nil {
		return nil, false, fmt.Errorf("marshal %s: %w", path, err)
	}
	var merged map[string]any
	if err := json.Unmarshal(raw, &merged); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if reflect.DeepEqual(merged, before) {
		return nil, false, nil
	}
	out, err := adapters.MarshalJSONIndentWith(root, adapters.DetectJSONIndent(data))
	if err != nil {
		return nil, false, fmt.Errorf("marshal %s: %w", path, err)
	}
	return append(out, '\n'), hadComments, nil
}

// orderedChild returns the object under key, or an empty one when the key
// is absent or holds another shape. isObject is false only for another
// shape, which a caller may replace but must not clear.
func orderedChild(parent *adapters.OrderedJSON, key string) (child *adapters.OrderedJSON, isObject bool) {
	child = adapters.NewOrderedJSON()
	raw, ok := parent.Get(key)
	if !ok {
		return child, true
	}
	if json.Unmarshal(raw, child) != nil {
		return adapters.NewOrderedJSON(), false
	}
	return child, true
}

// putOrderedChild stores child under key in place, or removes the key
// once child is empty.
func putOrderedChild(parent *adapters.OrderedJSON, key string, child *adapters.OrderedJSON) error {
	if child.Len() == 0 {
		parent.Delete(key)
		return nil
	}
	return parent.Set(key, child)
}

// orderedValue decodes the value under key.
func orderedValue(o *adapters.OrderedJSON, key string) (any, bool) {
	raw, ok := o.Get(key)
	if !ok {
		return nil, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	return value, true
}

func removeEqual(items []any, want any) ([]any, bool) {
	for i, item := range items {
		if reflect.DeepEqual(item, want) {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

func globalHookCommands(raw any) []string {
	switch value := raw.(type) {
	case string:
		if value != "" {
			return []string{value}
		}
	case []any:
		var out []string
		for _, item := range value {
			if command, ok := item.(string); ok && command != "" {
				out = append(out, command)
			}
		}
		return out
	case []string:
		return value
	}
	return nil
}
func jsonString(s string) string { raw, _ := json.Marshal(s); return string(raw) }
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

func globalUserHome() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return home, nil
}

// globalBridgePath returns the managed context-bridge script path for a
// target whose hooks file lives in base.
func globalBridgePath(base string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(base, "hooks", "agnostic-ai-global-context.ps1")
	}
	return filepath.Join(base, "hooks", "agnostic-ai-global-context.sh")
}

func globalContextBridge(base, body, key string) (path, command, script string, mode fs.FileMode) {
	payload := `{` + jsonString(key) + `:` + jsonString(body) + `}`
	path = globalBridgePath(base)
	if runtime.GOOS == "windows" {
		command = `powershell -NoProfile -ExecutionPolicy Bypass -File "` + strings.ReplaceAll(path, `"`, `\"`) + `"`
		script = "$payload = '" + strings.ReplaceAll(payload, "'", "''") + "'\r\n[Console]::Out.WriteLine($payload)\r\n"
		return path, command, script, 0o644
	}
	return path, path, "#!/bin/sh\nprintf '%s\\n' " + shellQuote(payload) + "\n", 0o755
}

func preflightGlobalWrites(writes []globalWrite, trees []string, old globalState) error {
	owned := map[string]bool{}
	for _, p := range old.Files {
		owned[p] = true
	}
	for _, w := range writes {
		if inManagedTree(w.path, trees) && !owned[w.path] {
			if filepath.Base(w.path) == "SKILL.md" {
				if _, err := os.Stat(filepath.Dir(w.path)); err == nil {
					return fmt.Errorf("%s: unmanaged global skill collision", filepath.Dir(w.path))
				}
			}
			if _, err := os.Stat(w.path); err == nil {
				return fmt.Errorf("%s: unmanaged global spec collision", w.path)
			}
		}
		if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s: refusing to replace symlink", w.path)
		}
	}
	return nil
}

// inManagedTree reports whether path sits under one of the directory
// surfaces sync --global owns for the targets in this run.
func inManagedTree(path string, trees []string) bool {
	for _, tree := range trees {
		if strings.HasPrefix(path, tree+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func cloneHookState(in map[string][]any) map[string][]any {
	out := map[string][]any{}
	for k, v := range in {
		out[k] = append([]any(nil), v...)
	}
	return out
}
func removePaths(paths, drop []string) []string {
	unwanted := map[string]bool{}
	for _, p := range drop {
		unwanted[p] = true
	}
	out := paths[:0]
	for _, p := range paths {
		if !unwanted[p] {
			out = append(out, p)
		}
	}
	return out
}

func removePathPrefix(paths []string, prefix string) []string {
	out := paths[:0]
	for _, p := range paths {
		if !strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func removedGlobalFiles(old, next []string) []string {
	keep := map[string]bool{}
	for _, p := range next {
		keep[p] = true
	}
	var out []string
	for _, p := range old {
		if !keep[p] {
			out = append(out, p)
		}
	}
	return out
}

func applyGlobalChanges(writes []globalWrite, removals []string, backup bool) error {
	type prior struct {
		path   string
		data   []byte
		mode   fs.FileMode
		absent bool
	}
	var done []prior
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			p := done[i]
			if p.absent {
				_ = os.Remove(p.path)
			} else {
				_ = os.WriteFile(p.path, p.data, p.mode)
			}
		}
	}
	for _, w := range writes {
		old, err := os.ReadFile(w.path)
		absent := os.IsNotExist(err)
		if err != nil && !absent {
			rollback()
			return fmt.Errorf("read %s: %w", w.path, err)
		}
		if !absent && reflect.DeepEqual(old, w.data) {
			continue
		}
		mode := fs.FileMode(0o644)
		if info, statErr := os.Stat(w.path); statErr == nil {
			mode = info.Mode()
		}
		done = append(done, prior{w.path, old, mode, absent})
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			rollback()
			return fmt.Errorf("create directory for %s: %w", w.path, err)
		}
		if backup && !absent {
			if err := os.WriteFile(w.path+".bak", old, mode); err != nil {
				rollback()
				return fmt.Errorf("backup %s: %w", w.path, err)
			}
		}
		tmp, err := os.CreateTemp(filepath.Dir(w.path), ".agnostic-ai-global-*")
		if err != nil {
			rollback()
			return fmt.Errorf("create temporary file for %s: %w", w.path, err)
		}
		tmpName := tmp.Name()
		writeErr := func() error {
			defer func() { _ = os.Remove(tmpName) }()
			if _, err := tmp.Write(w.data); err != nil {
				return err
			}
			if err := tmp.Chmod(w.mode); err != nil {
				return err
			}
			if err := tmp.Close(); err != nil {
				return err
			}
			return os.Rename(tmpName, w.path)
		}()
		if writeErr != nil {
			_ = tmp.Close()
			rollback()
			return fmt.Errorf("write %s: %w", w.path, writeErr)
		}
	}
	for _, path := range removals {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			rollback()
			return fmt.Errorf("read managed %s: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			rollback()
			return fmt.Errorf("stat managed %s: %w", path, err)
		}
		done = append(done, prior{path: path, data: data, mode: info.Mode()})
		if err := os.Remove(path); err != nil {
			rollback()
			return fmt.Errorf("remove managed %s: %w", path, err)
		}
	}
	return nil
}
