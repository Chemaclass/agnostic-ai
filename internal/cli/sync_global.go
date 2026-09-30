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
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/cursor"
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
	// Settings records the value written per target and native key of
	// its user settings file, so a later sync changes or removes only
	// keys it placed.
	Settings map[string]map[string]any `json:"settings,omitempty"`
	// MCP records each MCP server written per target, by its key in the
	// target's user MCP file, so a later sync removes only its own.
	MCP map[string]map[string]any `json:"mcp,omitempty"`
	// SettingsPaths and MCPPaths record the file each target's owned keys
	// live in, so a moved configuration root sweeps the old file.
	SettingsPaths map[string]string `json:"settingsPaths,omitempty"`
	MCPPaths      map[string]string `json:"mcpPaths,omitempty"`
	// Created lists the user files sync created, which it removes once
	// nothing is left in them. They also sit in Files, but a moved
	// configuration root is expected for them, so they are not proof of
	// another HOME.
	Created []string `json:"created,omitempty"`
	// removalGuards is each created file this run removes, as the plan
	// read it, so a write another program made since stops the removal.
	removalGuards map[string]*diskSnapshot
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
	// changes, adopted, and conflicts describe a key-level edit of a
	// user settings file; conflicts need --backup.
	changes, adopted, conflicts []string
	// targets names the targets whose surfaces this write carries.
	targets []string
	// planned is the file as a key-level edit read it, for a file
	// another program also writes, such as ~/.claude.json. The write
	// stops if the file changed since.
	planned *diskSnapshot
}

// diskSnapshot is a file's content, or its absence, at one moment.
type diskSnapshot struct {
	data   []byte
	absent bool
}

func snapshotFile(path string) (*diskSnapshot, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &diskSnapshot{absent: true}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &diskSnapshot{data: data}, nil
}

// addTarget records that target writes w.
func (w *globalWrite) addTarget(target string) {
	if target != "" && !slices.Contains(w.targets, target) {
		w.targets = append(w.targets, target)
	}
}

func runGlobalSync(cmd *cobra.Command, o globalSyncOptions) error {
	if o.watch || o.watchPoll || o.allTargets || o.gitignore != "" || o.jobs != 0 {
		return errs.Coded(errs.CodeFlagConflict, "--global does not support --watch, --watch-poll, --all, --gitignore, or --jobs")
	}
	if o.plan && (o.check || o.dryRun) {
		return errs.Coded(errs.CodeFlagConflict, "--plan cannot be combined with --check or --dry-run")
	}
	if o.jsonOut && o.diff {
		return errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --diff")
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
	home, err := globalUserHome()
	if err != nil {
		return err
	}
	source := globalSourceHome(home)
	warn := cmd.ErrOrStderr()
	if verbosity < levelDefault || o.check {
		warn = io.Discard
	}
	// -t bypasses the home config, so one that does not parse only
	// warns, but a requires it sets still holds for the specs.
	var skipBroken io.Writer
	if len(o.targets) > 0 {
		skipBroken = warn
	}
	if err := requireGlobalVersion(source, skipBroken); err != nil {
		return err
	}
	targets := o.targets
	// A default run spans every supported target, so one target's
	// problem (a relative root variable, a name its native format
	// rejects) warns and skips that target instead of failing the rest.
	// A home config names its targets, as --only does.
	explicit := len(o.targets) > 0 || len(o.only) > 0
	if len(targets) == 0 {
		// -t replaces the home config, so a broken one does not block it.
		if targets, err = loadGlobalTargets(source, warn); err != nil {
			return err
		}
		explicit = explicit || targets != nil
	}
	if len(targets) == 0 {
		targets = globalTargetNames()
	}
	// The home config's targets, before --only and --except, as project
	// sync reads agnostic-ai.yaml.
	configured := slices.Clone(targets)
	targets, err = filterTargets(targets, o.only, o.except)
	if err != nil {
		return err
	}
	for _, target := range targets {
		if err := unsupportedGlobalTarget("--global", target); err != nil {
			return err
		}
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
	if err := warnUnsupportedGlobalMCP(warn, targets, bundle.MCPs); err != nil {
		return err
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
	if foreign := foreignGlobalPath(old, home); foreign != "" {
		return fmt.Errorf("%s: recorded under another home, since %s is outside %s; sync under the HOME that recorded it, or remove the state file and the files it lists", statePath, foreign, home)
	}
	adapters.ResetCoverageNotes()
	adapters.SetWarner(warn)
	defer adapters.SetWarner(os.Stderr)
	defer adapters.ResetCoverageNotes()
	if slices.Contains(targets, "claude") && slices.Contains(configured, "cursor") {
		claude.NoteCursorDropsArgs(bundle.HooksFor("claude"), "~/.claude/settings.json")
	}
	// requireGlobalVersion already warned about a broken config under -t.
	onUnsupported, err := loadGlobalOnUnsupported(source)
	if err != nil && skipBroken == nil {
		return err
	}
	writes, next, err := buildGlobalWrites(home, source, targets, instructions, bundle, old, agentFailure(explicit, warn), warn, onUnsupported)
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
	adopted, err := preflightGlobalWrites(writes, trees, old)
	if err != nil {
		return err
	}
	removals := removedGlobalFiles(old.Files, next.Files)
	if o.check && o.jsonOut {
		records := globalFileRecords(writes, existingPaths(removals), statePath, next, true)
		if err := emitGlobalJSON(cmd, "sync --global --check", records); err != nil {
			return err
		}
		if len(records) > 0 {
			return errDriftDetected()
		}
		return nil
	}
	if o.check {
		return checkGlobalWrites(cmd, writes, existingPaths(removals), statePath, next, o.diff)
	}
	if o.plan || (o.dryRun && o.jsonOut) {
		records := globalFileRecords(writes, existingPaths(removals), statePath, next, false)
		// Mark what would stop the real run, as a key conflict is.
		edited, err := handEditedGlobalFiles(writes, removals, old)
		if err != nil {
			return err
		}
		for i := range records {
			path := filepath.FromSlash(records[i].Path)
			if slices.Contains(edited, path) {
				records[i].Keys = append(records[i].Keys, "conflict: edited since the last global sync; move the edit into the source or use --backup")
			}
			if j := slices.IndexFunc(writes, func(w globalWrite) bool { return w.path == path }); j >= 0 && writes[j].dropsComments {
				records[i].Keys = append(records[i].Keys, "conflict: the rewrite drops the file's comments; use --backup")
			}
		}
		if o.jsonOut {
			command := "sync --global --plan"
			if o.dryRun {
				command = "sync --global --dry-run"
			}
			return emitGlobalJSON(cmd, command, records)
		}
		return printGlobalPlan(cmd, records)
	}
	if o.dryRun {
		for _, w := range writes {
			if data, err := os.ReadFile(w.path); err == nil && bytes.Equal(data, w.data) {
				continue
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: write %s\n", w.path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
			for _, change := range w.changes {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run:   %s\n", change); err != nil {
					return fmt.Errorf("write dry-run output: %w", err)
				}
			}
		}
		for _, path := range existingPaths(removals) {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: remove %s\n", path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		for _, path := range adopted {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: adopt %s\n", path); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		for _, line := range settingsAdoptions(writes) {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "dry-run: adopt %s\n", line); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
		return nil
	}
	if !o.backup {
		var conflicts []string
		for _, w := range writes {
			for _, c := range w.conflicts {
				conflicts = append(conflicts, w.path+": "+c)
			}
		}
		if len(conflicts) > 0 {
			return fmt.Errorf("%s; or rerun with --backup to overwrite and keep a .bak copy", strings.Join(conflicts, "; "))
		}
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
	var applied []globalFileRecord
	if o.jsonOut {
		applied = globalFileRecords(writes, existingPaths(removals), statePath, next, false)
	}
	linked, err := applyGlobalChanges(writes, removals, o.backup, next.removalGuards)
	if err != nil {
		return err
	}
	pruneEmptyGlobalDirs(removals, trees)
	if o.jsonOut {
		return emitGlobalJSON(cmd, "sync --global", applied)
	}
	for _, link := range linked {
		if _, err := fmt.Fprintln(warn, link); err != nil {
			return fmt.Errorf("write symlink note: %w", err)
		}
	}
	for _, path := range adopted {
		if _, err := fmt.Fprintf(warn, "adopted %s: matches what sync writes\n", path); err != nil {
			return fmt.Errorf("write adoption note: %w", err)
		}
	}
	for _, line := range settingsAdoptions(writes) {
		if _, err := fmt.Fprintf(warn, "adopted %s: matches what sync writes\n", line); err != nil {
			return fmt.Errorf("write adoption note: %w", err)
		}
	}
	if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Synced global configuration to %d target(s).\n", len(targets)); err != nil {
		return fmt.Errorf("write sync summary: %w", err)
	}
	return nil
}

// globalFileRecord is a fileRecord with the key-level edits a user
// settings or MCP file carries.
type globalFileRecord struct {
	fileRecord
	Keys []string `json:"keys,omitempty"`
}

// emitGlobalJSON prints records in the jsonOutput layout: version,
// command, writes, skipped, and errors.
func emitGlobalJSON(cmd *cobra.Command, command string, records []globalFileRecord) error {
	if records == nil {
		records = []globalFileRecord{}
	}
	return writeIndentedJSON(cmd, struct {
		Version string             `json:"version"`
		Command string             `json:"command"`
		Writes  []globalFileRecord `json:"writes"`
		Skipped []fileRecord       `json:"skipped"`
		Errors  []errorRecord      `json:"errors"`
	}{"1", command, records, []fileRecord{}, []errorRecord{}})
}

// globalFileRecords describes each file a run would change, in the
// project sync schema: create, update, or delete, or with check the
// drift words missing, stale, and leftover. A user settings or MCP file
// lists its key-level edits, and a conflict is marked in them.
func globalFileRecords(writes []globalWrite, removals []string, statePath string, next globalState, check bool) []globalFileRecord {
	create, update, remove := "create", "update", "delete"
	if check {
		create, update, remove = "missing", "stale", "leftover"
	}
	out := []globalFileRecord{}
	for _, w := range writes {
		action := update
		// Check counts the state file as drift only when its ownership
		// differs; a plan reports the bytes a sync writes.
		if check && w.path == statePath && globalStateCurrent(statePath, next) {
			continue
		}
		data, err := os.ReadFile(w.path)
		if err == nil && bytes.Equal(data, w.data) {
			continue
		}
		if err != nil {
			action = create
		}
		keys := slices.Clone(w.changes)
		for _, c := range w.conflicts {
			keys = append(keys, "conflict: "+c)
		}
		out = append(out, globalFileRecord{fileRecord: fileRecord{Target: strings.Join(w.targets, ","), Path: filepath.ToSlash(w.path), Action: action, Bytes: len(w.data)}, Keys: keys})
	}
	for _, path := range removals {
		out = append(out, globalFileRecord{fileRecord: fileRecord{Path: filepath.ToSlash(path), Action: remove}})
	}
	return out
}

// printGlobalPlan prints one line per file a sync would change, with its
// key-level edits under it, and writes nothing.
func printGlobalPlan(cmd *cobra.Command, records []globalFileRecord) error {
	out := cmd.OutOrStdout()
	if len(records) == 0 {
		_, err := fmt.Fprintln(out, "No changes.")
		return err
	}
	for _, r := range records {
		target := ""
		if r.Target != "" {
			target = " [" + r.Target + "]"
		}
		if _, err := fmt.Fprintf(out, "%s %s%s\n", r.Action, r.Path, target); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
		for _, key := range r.Keys {
			if _, err := fmt.Fprintf(out, "  %s\n", key); err != nil {
				return fmt.Errorf("write plan: %w", err)
			}
		}
	}
	return nil
}

// settingsAdoptions names each user settings key that already held the
// value sync writes, as "<path> <key>".
func settingsAdoptions(writes []globalWrite) []string {
	var out []string
	for _, w := range writes {
		for _, key := range w.adopted {
			out = append(out, w.path+" "+key)
		}
	}
	return out
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
	// An empty list and a missing one record the same ownership.
	if len(recorded.Files) == 0 {
		recorded.Files = []string{}
	}
	if len(next.Files) == 0 {
		next.Files = []string{}
	}
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

// foreignGlobalPath returns a recorded path outside every root this run
// resolves, which means the state belongs to another HOME. Ownership is
// by absolute path, so syncing on would sweep that home's files.
func foreignGlobalPath(old globalState, home string) string {
	roots := globalRoots(home)
	paths := slices.DeleteFunc(slices.Clone(old.Files), func(p string) bool { return slices.Contains(old.Created, p) })
	for _, owned := range []map[string][]string{old.Agents, old.Skills} {
		for _, target := range slices.Sorted(maps.Keys(owned)) {
			paths = append(paths, owned[target]...)
		}
	}
	for _, path := range paths {
		if !slices.ContainsFunc(roots, func(root string) bool { return path == root || inManagedTree(path, []string{root}) }) {
			return path
		}
	}
	return ""
}

func buildGlobalWrites(home, source string, targets []string, intro []byte, b spec.Bundle, old globalState, agentErr func(string, error) error, warn io.Writer, onUnsupported string) ([]globalWrite, globalState, error) {
	next := globalState{Version: globalStateVersion, Files: append([]string(nil), old.Files...), Hooks: map[string]map[string][]any{}, Agents: map[string][]string{}, Skills: map[string][]string{}, AgentEfforts: map[string]map[string]string{}, Settings: map[string]map[string]any{}, MCP: map[string]map[string]any{}, SettingsPaths: map[string]string{}, MCPPaths: map[string]string{}, Created: slices.Clone(old.Created)}
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
	for target, settings := range old.Settings {
		if !slices.Contains(targets, target) {
			next.Settings[target] = settings
		}
	}
	for target, servers := range old.MCP {
		if !slices.Contains(targets, target) {
			next.MCP[target] = servers
		}
	}
	for _, kept := range []struct{ from, into map[string]string }{{old.SettingsPaths, next.SettingsPaths}, {old.MCPPaths, next.MCPPaths}} {
		for target, path := range kept.from {
			if !slices.Contains(targets, target) {
				kept.into[target] = path
			}
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
	// emptied holds hooks files the hooks merge left with nothing in
	// them, which a settings edit must start from instead of the disk.
	emptied := map[string]bool{}
	// current is the target whose surfaces the loops below are placing.
	var current string
	place := func(path string, data []byte, mode fs.FileMode) (bool, error) {
		if i, ok := seen[path]; ok {
			if !bytes.Equal(writes[i].data, data) {
				return false, fmt.Errorf("%s: two global targets emit different content to one path", path)
			}
			writes[i].addTarget(current)
			return false, nil
		}
		seen[path] = len(writes)
		writes = append(writes, globalWrite{path: path, data: data, mode: mode, targets: []string{current}})
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
		current = target
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
			if err := adapters.ReportClaudeSkillSyntax(target, b.Skills, onUnsupported); err != nil {
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
		if err := adapters.ReportHookProjectRoot(target, hooks, onUnsupported, true); err != nil {
			return nil, next, err
		}
		if g.hooksFormat == "augment" {
			augment.NoteUserHookGaps(hooks)
		}
		hookTarget := globalHookTarget{name: target, mode: g.hookTarget, args: g.hookArgs, foldArgs: g.hookFoldArgs, timeout: g.hookTimeout, specHooks: len(hooks)}
		if g.bridge && body != "" {
			bridge, command, script, mode := globalContextBridge(filepath.Dir(path), body, g.bridgeKey)
			if err := add(bridge, []byte(script), mode); err != nil {
				return nil, next, err
			}
			hooks = append(append([]spec.Entry{}, hooks...), spec.Entry{Meta: map[string]any{"event": g.bridgeEvent, "command": command}})
		}
		doc, err := mergeGlobalHooks(path, g.hooksFormat, hookTarget, hooks, old.Hooks[target], next.Hooks[target], warn)
		if errors.Is(err, errGlobalFileUnchanged) {
			if slices.Contains(old.Files, path) {
				next.Files = append(next.Files, path)
			}
			continue
		}
		if err != nil {
			return nil, next, err
		}
		if doc == nil {
			emptied[path] = true
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
	// User settings and MCP servers merge last: a file may also hold
	// hooks or agent efforts this run changed, so each edit starts from
	// that planned content.
	placeEdit := func(path string, m globalSettingsMerge, mode fs.FileMode) error {
		if m.data == nil && len(m.adopted) == 0 {
			return nil
		}
		i, planned := seen[path]
		if !planned {
			data := m.data
			if data == nil {
				var err error
				if data, err = os.ReadFile(path); err != nil {
					return fmt.Errorf("read %s: %w", path, err)
				}
			}
			seen[path] = len(writes)
			writes = append(writes, globalWrite{path: path, data: data, mode: mode})
			i = len(writes) - 1
		} else if m.data != nil {
			writes[i].data = m.data
		}
		w := &writes[i]
		if w.planned == nil {
			snapshot, err := snapshotFile(path)
			if err != nil {
				return err
			}
			w.planned = snapshot
		}
		// A user file this run creates is recorded, so a later run can
		// remove it once nothing is left in it.
		if w.planned.absent {
			if !slices.Contains(next.Files, path) {
				next.Files = append(next.Files, path)
			}
			if !slices.Contains(next.Created, path) {
				next.Created = append(next.Created, path)
			}
		}
		w.addTarget(current)
		w.changes = append(w.changes, m.changes...)
		w.adopted = append(w.adopted, m.adopted...)
		w.conflicts = append(w.conflicts, m.conflicts...)
		// A file a removal would sweep, such as a hooks file with no
		// hooks left, stays while it holds managed keys.
		if slices.Contains(old.Files, path) {
			next.Files = append(next.Files, path)
		}
		return nil
	}
	baseFor := func(path string) []byte {
		if i, planned := seen[path]; planned {
			return writes[i].data
		}
		if emptied[path] {
			return []byte("{}\n")
		}
		return nil
	}
	for _, target := range targets {
		current = target
		g := globalTargets[target]
		want := globalSettingsFor(target, g, b.Settings)
		if g.settings.path != "" {
			path := g.path(home, g.settings.path)
			previous := old.Settings[target]
			// Keys recorded in a file the root no longer resolves to go
			// from that file, and the new one starts unowned.
			if moved := old.SettingsPaths[target]; moved != "" && moved != path {
				m, err := mergeGlobalSettings(moved, g.settings.format, baseFor(moved), nil, previous)
				if err != nil {
					return nil, next, err
				}
				if err := placeEdit(moved, m, 0o644); err != nil {
					return nil, next, err
				}
				previous = nil
			}
			m, err := mergeGlobalSettings(path, g.settings.format, baseFor(path), want, previous)
			if err != nil {
				return nil, next, err
			}
			if len(m.owned) > 0 {
				next.Settings[target] = m.owned
				next.SettingsPaths[target] = path
			}
			if err := placeEdit(path, m, 0o644); err != nil {
				return nil, next, err
			}
		}
		if g.mcp.path != "" {
			path := g.mcpPath(home)
			previous := old.MCP[target]
			if moved := old.MCPPaths[target]; moved != "" && moved != path {
				m, err := mergeGlobalMCP(moved, g.mcp, baseFor(moved), target, nil, previous)
				if err != nil {
					return nil, next, err
				}
				if err := placeEdit(moved, m, g.mcp.fileMode()); err != nil {
					return nil, next, err
				}
				previous = nil
			}
			m, err := mergeGlobalMCP(path, g.mcp, baseFor(path), target, b.MCPs, previous)
			if err != nil {
				return nil, next, err
			}
			if len(m.owned) > 0 {
				next.MCP[target] = m.owned
				next.MCPPaths[target] = path
			}
			if err := placeEdit(path, m, g.mcp.fileMode()); err != nil {
				return nil, next, err
			}
		}
	}
	// A user file sync created goes once nothing is left in it, rather
	// than staying behind as an empty document.
	for i := 0; i < len(writes); i++ {
		w := writes[i]
		if w.owned || !emptyUserDocument(w.data) || !slices.Contains(next.Created, w.path) {
			continue
		}
		writes = slices.Delete(writes, i, i+1)
		i--
		next.Files = removePaths(next.Files, []string{w.path})
		next.Created = removePaths(next.Created, []string{w.path})
		if w.planned != nil && !w.planned.absent {
			if next.removalGuards == nil {
				next.removalGuards = map[string]*diskSnapshot{}
			}
			next.removalGuards[w.path] = w.planned
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
	// Always a list, so the state file keeps one spelling for no files.
	if next.Files == nil {
		next.Files = []string{}
	}
	next.Created = slices.DeleteFunc(next.Created, func(p string) bool { return !slices.Contains(next.Files, p) })
	sort.Strings(next.Created)
	next.Created = slices.Compact(next.Created)
	if len(next.Created) == 0 {
		next.Created = nil
	}
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

// emptyUserDocument reports whether a user settings or MCP file holds
// nothing: no text, or a JSON object with no members.
func emptyUserDocument(data []byte) bool {
	text := strings.Join(strings.Fields(string(data)), "")
	return text == "" || text == "{}"
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

// globalSkillOverlays maps the relative paths any target reading dir
// renders beside SKILL.md for skill to their rendered content. Every
// co-writer of a shared tree ships that content in place of a bundled
// asset at the same path, so they agree and the spec-rendered file
// wins, as in project sync.
func globalSkillOverlays(home, dir string, skill spec.Entry) (map[string]string, error) {
	overlays := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(globalTargets)) {
		g := globalTargets[name]
		if g.skills == "" || g.path(home, g.skills) != dir || !skill.EmitsTo(name) {
			continue
		}
		sidecars, err := adapters.RenderSkillSidecars(name, skill)
		if err != nil {
			return nil, err
		}
		maps.Copy(overlays, sidecars)
	}
	return overlays, nil
}

func addGlobalSkill(dst string, skill spec.Entry, target string, shared bool, overlays map[string]string, add func(string, []byte, fs.FileMode) error) error {
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
	root := skill.SkillAssetDir()
	if root == "" {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		// The rendered SKILL.md, not the asset folder's own copy, which
		// is the shared one when a local skill inherits its assets.
		if rel == "SKILL.md" {
			return nil
		}
		if overlay, ok := overlays[rel]; ok {
			if _, own := sidecars[rel]; own {
				return nil
			}
			return add(filepath.Join(dst, rel), []byte(overlay), 0o644)
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
// key order and indent. A recorded entry the file no longer holds counts
// as removed, with a warning, so a reset settings file is rebuilt. One
// still there with the same matcher and command but other fields was
// edited by hand, and stops the run: replacing it would run it twice.
// An unrecorded entry exactly as sync would write it satisfies the
// source and stays the user's, since nothing in it shows sync wrote it.
// One with the same matcher and command but other fields stops the run.
func mergeGlobalHooks(path, format string, target globalHookTarget, entries []spec.Entry, previous map[string][]any, next map[string][]any, warn io.Writer) ([]byte, error) {
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
	// planned holds each entry as sync writes it now, one per command.
	type plannedHook struct {
		event       string
		item, plain any
		spec        bool
	}
	var planned []plannedHook
	for i, entry := range entries {
		event, _ := entry.Meta["event"].(string)
		if event == "" {
			continue
		}
		for _, command := range globalHookCommands(entry.Meta["command"]) {
			command = adapters.RewriteGlobalHookRoot(command, target.name, entry.Meta)
			var item, plain any
			switch format {
			case "augment":
				// Augment documents type, command, and timeout (ms) on a
				// hook, and a matcher only on tool events.
				commandHook := map[string]any{"type": "command", "command": command}
				if timeout, ok := augment.HookTimeout(entry.Meta); ok {
					commandHook["timeout"] = timeout
				}
				group := map[string]any{"hooks": []any{commandHook}}
				if matcher, _ := entry.Meta["matcher"].(string); matcher != "" && !augment.SessionOnlyEvent(event) {
					group["matcher"] = matcher
				}
				item, plain = group, group
				// An empty matcher means the same as none, so a hand-written
				// `"matcher": ""` entry satisfies the spec.
				if _, has := group["matcher"]; !has {
					plain = map[string]any{"matcher": "", "hooks": []any{maps.Clone(commandHook)}}
				}
			case "claude":
				commandHook := map[string]any{"type": "command", "command": command}
				for _, key := range []string{"timeout", "statusMessage", "async", "asyncRewake", "shell", "if", "once"} {
					if value, ok := entry.Meta[key]; ok {
						commandHook[key] = value
					}
				}
				if args := stringSliceFromAny(entry.Meta["args"]); len(args) > 0 {
					switch {
					case target.args:
						commandHook["args"] = args
					case target.foldArgs:
						commandHook["command"] = adapters.ExecFormCommand(command, args)
					}
				}
				if target.timeout != nil {
					delete(commandHook, "timeout")
					if timeout, ok := target.timeout(entry.Meta); ok {
						commandHook["timeout"] = timeout
					}
				}
				matcher, _ := entry.Meta["matcher"].(string)
				plain = map[string]any{"matcher": matcher, "hooks": []any{maps.Clone(commandHook)}}
				target.tell(commandHook, entry.Meta)
				item = map[string]any{"matcher": matcher, "hooks": []any{commandHook}}
			default:
				if args := stringSliceFromAny(entry.Meta["args"]); target.foldArgs {
					command = adapters.ExecFormCommand(command, args)
				}
				cursorHook := map[string]any{"command": command}
				for _, key := range []string{"matcher", "timeout", "loop_limit", "failClosed"} {
					if value, ok := entry.Meta[key]; ok {
						cursorHook[key] = value
					}
				}
				item, plain = cursorHook, cursorHook
			}
			planned = append(planned, plannedHook{event: event, item: item, plain: plain, spec: i < target.specHooks})
		}
	}
	// writesNow reports that an edited copy of a recorded entry is what
	// sync writes now. The form without the target signal counts only
	// for a record that lacked it too, as versions before it wrote:
	// removing the signal from a record that had it is an edit.
	writesNow := func(event string, existing, recorded any) bool {
		return slices.ContainsFunc(planned, func(p plannedHook) bool {
			if p.event != event {
				return false
			}
			return reflect.DeepEqual(existing, jsonRoundTrip(p.item)) ||
				(!carriesHookTarget(recorded) && reflect.DeepEqual(existing, jsonRoundTrip(p.plain)))
		})
	}
	for _, event := range slices.Sorted(maps.Keys(previous)) {
		current, _ := hooks[event].([]any)
		var lost []any
		for _, oldEntry := range previous[event] {
			var found bool
			if current, found = removeEqual(current, oldEntry); !found {
				lost = append(lost, oldEntry)
			}
		}
		// An edit that matches what sync writes now, such as a timeout
		// an older version wrote in the wrong unit and the user fixed,
		// is not a conflict: it goes, and the planned entry replaces it.
		stillLost := lost[:0]
		for _, oldEntry := range lost {
			edited := slices.IndexFunc(current, func(item any) bool {
				return sameGlobalHook(item, oldEntry) || extendsGlobalHook(item, oldEntry)
			})
			switch {
			case edited < 0:
				stillLost = append(stillLost, oldEntry)
			case writesNow(event, current[edited], oldEntry):
				current = slices.Delete(current, edited, edited+1)
			default:
				return nil, fmt.Errorf("%s: managed %s hook was edited; restore it or remove it, then sync", path, event)
			}
		}
		lost = stillLost
		if len(lost) > 0 {
			if _, err := fmt.Fprintf(warn, "warning: %s: managed %s hook is missing, so it counts as removed\n", path, event); err != nil {
				return nil, fmt.Errorf("write global hook warning: %w", err)
			}
		}
		if len(current) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = current
		}
	}
	unrecorded := map[string][]any{}
	for event, items := range hooks {
		current, _ := items.([]any)
		unrecorded[event] = slices.Clone(current)
	}
	// place adds one managed item unless an unrecorded entry already
	// matches it, or matches plain, the item without the target signal:
	// a hook the user wrote stays exactly as written. Either way the
	// source hook is in the file.
	place := func(event string, item, plain any) error {
		for _, want := range []any{item, plain} {
			var satisfied bool
			if unrecorded[event], satisfied = removeEqual(unrecorded[event], jsonRoundTrip(want)); satisfied {
				return nil
			}
		}
		if slices.ContainsFunc(unrecorded[event], func(existing any) bool { return sameGlobalHook(existing, item) || sameGlobalHook(existing, plain) }) {
			return fmt.Errorf("%s: a %s hook not recorded as managed runs a source hook's matcher and command with other settings; remove that entry, or give the command its own entry that matches the source, then sync", path, event)
		}
		current, _ := hooks[event].([]any)
		hooks[event] = append(current, item)
		next[event] = append(next[event], item)
		return nil
	}
	// told reports that a spec hook is in the file, written by sync or
	// by the user, so the target signal outside the entries (settings
	// env, session hook) is due. An adopted entry keeps its own form,
	// so a codex, gemini, or qoder hook the user wrote gets no signal.
	told := false
	for _, p := range planned {
		if err := place(p.event, p.item, p.plain); err != nil {
			return nil, err
		}
		told = told || p.spec
	}
	if target.mode == hookTargetSessionEnv && told {
		item := map[string]any{"command": cursor.HookTargetCommand}
		if err := place(cursor.HookTargetEvent, item, item); err != nil {
			return nil, err
		}
	}
	if len(hooks) > 0 {
		doc["hooks"] = hooks
	} else {
		delete(doc, "hooks")
	}
	if target.mode == hookTargetSettingsEnv {
		target.setSettingsEnv(doc, told)
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
		if key == "hooks" {
			original, _ := ordered.Get("hooks")
			raw, err := orderedHooksRaw(original, hooks)
			if err != nil {
				return nil, fmt.Errorf("marshal %s: %w", path, err)
			}
			ordered.SetRaw("hooks", raw)
			continue
		}
		if err := ordered.Set(key, value); err != nil {
			return nil, fmt.Errorf("marshal %s: %w", path, err)
		}
	}
	if target.mode == hookTargetSettingsEnv {
		if err := adapters.SetHookTargetEnv(ordered, target.name, told); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
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

// orderedHooksRaw renders the hooks object keeping the file's event
// order and each unchanged entry's own bytes, so a rewrite does not
// reorder keys inside hooks the user wrote. New events and entries,
// sync's own, are encoded fresh.
func orderedHooksRaw(original json.RawMessage, hooks map[string]any) (json.RawMessage, error) {
	orig := adapters.NewOrderedJSON()
	if len(original) > 0 && json.Unmarshal(original, orig) != nil {
		orig = adapters.NewOrderedJSON()
	}
	var events []string
	for _, event := range orig.Keys() {
		if _, ok := hooks[event]; ok {
			events = append(events, event)
		}
	}
	for _, event := range slices.Sorted(maps.Keys(hooks)) {
		if !slices.Contains(events, event) {
			events = append(events, event)
		}
	}
	encode := func(v any) (string, error) {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return "", err
		}
		return strings.TrimSuffix(buf.String(), "\n"), nil
	}
	out := adapters.NewOrderedJSON()
	for _, event := range events {
		items, isList := hooks[event].([]any)
		if !isList {
			if raw, ok := orig.Get(event); ok {
				var have any
				if json.Unmarshal(raw, &have) == nil && reflect.DeepEqual(have, jsonRoundTrip(hooks[event])) {
					out.SetRaw(event, raw)
					continue
				}
			}
			if err := out.Set(event, hooks[event]); err != nil {
				return nil, err
			}
			continue
		}
		var existing []json.RawMessage
		if raw, ok := orig.Get(event); ok {
			_ = json.Unmarshal(raw, &existing)
		}
		used := make([]bool, len(existing))
		parts := make([]string, 0, len(items))
		for _, item := range items {
			want := jsonRoundTrip(item)
			kept := false
			for i, raw := range existing {
				var have any
				if used[i] || json.Unmarshal(raw, &have) != nil || !reflect.DeepEqual(have, want) {
					continue
				}
				used[i], kept = true, true
				parts = append(parts, string(raw))
				break
			}
			if kept {
				continue
			}
			text, err := encode(item)
			if err != nil {
				return nil, err
			}
			parts = append(parts, text)
		}
		out.SetRaw(event, json.RawMessage("["+strings.Join(parts, ",")+"]"))
	}
	text, err := encode(out)
	return json.RawMessage(text), err
}

// jsonRoundTrip returns v as JSON decodes it, so a spec integer compares
// equal to the number read back from a native file.
func jsonRoundTrip(v any) any {
	raw, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return v
	}
	return out
}

func removeEqual(items []any, want any) ([]any, bool) {
	for i, item := range items {
		if reflect.DeepEqual(item, want) {
			return append(items[:i], items[i+1:]...), true
		}
	}
	return items, false
}

// globalHookTarget tells a global hook which target ran it, the way the
// target's globalTarget.hookTarget says.
type globalHookTarget struct {
	name, mode string
	// timeout converts the spec's timeout, when the target reads
	// another unit than seconds.
	timeout func(meta map[string]any) (any, bool)
	// args says the handler takes exec-form `args`; foldArgs, that it
	// has no such field and the args fold into the command.
	args, foldArgs bool
	// specHooks counts the leading entries that come from hook specs;
	// the rest, such as Cursor's context bridge, are sync's own.
	specHooks int
}

// tell adds the target to one Claude-shaped command handler.
func (t globalHookTarget) tell(handler, meta map[string]any) {
	switch t.mode {
	case hookTargetExport:
		command, _ := handler["command"].(string)
		windows, _ := meta["commandWindows"].(string)
		if windows == "" {
			windows = command
		}
		handler["command"] = adapters.ExportHookTarget(command, t.name)
		handler["commandWindows"] = windows
	case hookTargetHandlerEnv:
		handler["env"] = map[string]any{adapters.HookTargetEnv: t.name}
	}
}

// setSettingsEnv keeps the target in the settings `env` while managed
// hooks exist, and drops the value sync wrote once none are left.
func (t globalHookTarget) setSettingsEnv(doc map[string]any, want bool) {
	env, _ := doc["env"].(map[string]any)
	next := adapters.WithoutHookTarget(env, any(t.name))
	if want {
		next = adapters.WithHookTarget(env, any(t.name))
	}
	if len(next) == 0 {
		delete(doc, "env")
		return
	}
	doc["env"] = next
}

// carriesHookTarget reports whether a native hook entry holds the target
// signal a handler carries itself: an `env` naming the target, or the
// export prefix.
func carriesHookTarget(entry any) bool {
	item, _ := entry.(map[string]any)
	handlers := []any{item}
	if group, ok := item["hooks"].([]any); ok {
		handlers = group
	}
	for _, raw := range handlers {
		handler, _ := raw.(map[string]any)
		if env, ok := handler["env"].(map[string]any); ok {
			if _, ok := env[adapters.HookTargetEnv]; ok {
				return true
			}
		}
		if command, _ := handler["command"].(string); strings.HasPrefix(command, "export "+adapters.HookTargetEnv+"=") {
			return true
		}
	}
	return false
}

// sameGlobalHook reports whether item runs the recorded entry's command
// under its matcher, whatever its other fields say.
func sameGlobalHook(item, recorded any) bool {
	want := globalHookKeys(recorded)
	return slices.ContainsFunc(globalHookKeys(item), func(key [2]string) bool { return slices.Contains(want, key) })
}

// extendsGlobalHook reports whether item runs the recorded entry's
// command with more words after it, under the same matcher: a bare
// interpreter an older version wrote for an exec-form spec, with its
// args added back by hand.
func extendsGlobalHook(item, recorded any) bool {
	want := globalHookKeys(recorded)
	return slices.ContainsFunc(globalHookKeys(item), func(key [2]string) bool {
		return slices.ContainsFunc(want, func(w [2]string) bool {
			return key[0] == w[0] && strings.HasPrefix(key[1], w[1]+" ")
		})
	})
}

// globalHookKeys lists the matcher and command pairs a native hook entry
// runs: a Claude-style group holds several commands, a Cursor entry one.
func globalHookKeys(item any) [][2]string {
	entry, _ := item.(map[string]any)
	matcher, _ := entry["matcher"].(string)
	commands := []any{entry}
	if group, ok := entry["hooks"].([]any); ok {
		commands = group
	}
	var out [][2]string
	for _, raw := range commands {
		hook, _ := raw.(map[string]any)
		if command, ok := hook["command"].(string); ok {
			out = append(out, [2]string{matcher, command})
		}
	}
	return out
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
	return path, path, "#!/bin/sh\nprintf '%s\\n' " + adapters.ShellQuote(payload) + "\n", 0o755
}

// preflightGlobalWrites stops on an unrecorded file in a managed tree
// before anything is written. A file, or a whole skill folder, holding
// exactly what this run writes there is adopted instead, so a lost state
// does not strand earlier output. It returns the adopted paths.
func preflightGlobalWrites(writes []globalWrite, trees []string, old globalState) ([]string, error) {
	owned := map[string]bool{}
	for _, p := range old.Files {
		owned[p] = true
	}
	planned := map[string][]byte{}
	for _, w := range writes {
		planned[w.path] = w.data
	}
	var adopted, folders []string
	for _, w := range writes {
		if inManagedTree(w.path, trees) && !owned[w.path] {
			if filepath.Base(w.path) == "SKILL.md" {
				dir := filepath.Dir(w.path)
				if _, err := os.Stat(dir); err == nil {
					same, err := globalFolderMatches(dir, planned)
					if err != nil {
						return nil, err
					}
					if !same {
						return nil, fmt.Errorf("%s: unmanaged global skill collision", dir)
					}
					if _, err := os.Stat(w.path); err == nil {
						adopted = append(adopted, dir)
						folders = append(folders, dir)
					}
				}
			}
			if _, err := os.Stat(w.path); err == nil {
				if data, err := os.ReadFile(w.path); err != nil || !bytes.Equal(data, w.data) {
					return nil, fmt.Errorf("%s: unmanaged global spec collision", w.path)
				}
				if !inManagedTree(w.path, folders) {
					adopted = append(adopted, w.path)
				}
			}
		}
		// A single user file, such as CLAUDE.md or settings.json kept in
		// a dotfiles repo, is written through its link. A link inside a
		// tree sync owns file by file is refused.
		if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if data, err := os.ReadFile(w.path); err == nil && bytes.Equal(data, w.data) {
				continue
			}
			if inManagedTree(w.path, trees) {
				return nil, fmt.Errorf("%s: refusing to replace symlink", w.path)
			}
			if _, err := filepath.EvalSymlinks(w.path); err != nil {
				return nil, fmt.Errorf("%s: broken symlink: %w", w.path, err)
			}
		}
	}
	return adopted, nil
}

// globalFolderMatches reports whether every file in dir is one this run
// writes, with the same bytes. Files the run adds are allowed; a file of
// any other content, or one the run does not write, is not.
func globalFolderMatches(dir string, planned map[string][]byte) (bool, error) {
	same := true
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		want, ok := planned[path]
		if !ok || !d.Type().IsRegular() {
			same = false
			return filepath.SkipAll
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(data, want) {
			same = false
			return filepath.SkipAll
		}
		return nil
	})
	return same, err
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

// applyGlobalChanges writes and removes files, rolling everything back
// on the first failure. It returns a note for each symlink it wrote
// through or removed.
func applyGlobalChanges(writes []globalWrite, removals []string, backup bool, guards map[string]*diskSnapshot) (linked []string, err error) {
	type prior struct {
		path   string
		data   []byte
		mode   fs.FileMode
		absent bool
		// link is the text of a symlink removed at path.
		link string
	}
	var done []prior
	rollback := func() {
		for i := len(done) - 1; i >= 0; i-- {
			p := done[i]
			if p.link != "" {
				_ = os.Symlink(p.link, p.path)
				continue
			}
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
			return nil, fmt.Errorf("read %s: %w", w.path, err)
		}
		if !absent && reflect.DeepEqual(old, w.data) {
			continue
		}
		if w.planned != nil && (w.planned.absent != absent || !bytes.Equal(w.planned.data, old)) {
			rollback()
			return nil, fmt.Errorf("%s changed while sync ran, likely written by the tool itself; nothing was written, so rerun the sync", w.path)
		}
		mode := fs.FileMode(0o644)
		if info, statErr := os.Stat(w.path); statErr == nil {
			mode = info.Mode()
		}
		// Replace the file a symlink points at, so the link stays.
		target := w.path
		if info, err := os.Lstat(w.path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			if target, err = filepath.EvalSymlinks(w.path); err != nil {
				rollback()
				return nil, fmt.Errorf("%s: broken symlink: %w", w.path, err)
			}
			linked = append(linked, "wrote through symlink "+w.path+" -> "+target)
		}
		done = append(done, prior{path: target, data: old, mode: mode, absent: absent})
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			rollback()
			return nil, fmt.Errorf("create directory for %s: %w", w.path, err)
		}
		if backup && !absent {
			if err := os.WriteFile(w.path+".bak", old, mode); err != nil {
				rollback()
				return nil, fmt.Errorf("backup %s: %w", w.path, err)
			}
			// An existing .bak keeps its own mode on write; the copy of a
			// private file must be private too.
			if err := os.Chmod(w.path+".bak", mode.Perm()); err != nil {
				rollback()
				return nil, fmt.Errorf("backup %s: %w", w.path, err)
			}
		}
		tmp, err := os.CreateTemp(filepath.Dir(target), ".agnostic-ai-global-*")
		if err != nil {
			rollback()
			return nil, fmt.Errorf("create temporary file for %s: %w", w.path, err)
		}
		tmpName := tmp.Name()
		// A user's own file, such as an MCP config holding API keys at
		// 0600, keeps its permission bits.
		perm := w.mode
		if !w.owned && !absent {
			perm = mode.Perm()
		}
		writeErr := func() error {
			defer func() { _ = os.Remove(tmpName) }()
			if _, err := tmp.Write(w.data); err != nil {
				return err
			}
			if err := tmp.Chmod(perm); err != nil {
				return err
			}
			if err := tmp.Close(); err != nil {
				return err
			}
			return os.Rename(tmpName, target)
		}()
		if writeErr != nil {
			_ = tmp.Close()
			rollback()
			return nil, fmt.Errorf("write %s: %w", w.path, writeErr)
		}
	}
	for _, path := range removals {
		// Removing a file reached through a symlink removes the file the
		// link points at, then the link, so no stale copy stays behind
		// in a dotfiles repository.
		if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
			text, err := os.Readlink(path)
			if err != nil {
				rollback()
				return nil, fmt.Errorf("read symlink %s: %w", path, err)
			}
			if target, err := filepath.EvalSymlinks(path); err == nil {
				data, readErr := os.ReadFile(target)
				targetInfo, statErr := os.Stat(target)
				if readErr != nil || statErr != nil {
					rollback()
					return nil, fmt.Errorf("read managed %s: %w", target, errors.Join(readErr, statErr))
				}
				done = append(done, prior{path: target, data: data, mode: targetInfo.Mode()})
				if err := os.Remove(target); err != nil {
					rollback()
					return nil, fmt.Errorf("remove managed %s: %w", target, err)
				}
				linked = append(linked, "removed "+target+" and its symlink "+path)
			}
			done = append(done, prior{path: path, link: text})
			if err := os.Remove(path); err != nil {
				rollback()
				return nil, fmt.Errorf("remove managed %s: %w", path, err)
			}
			continue
		}
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			rollback()
			return nil, fmt.Errorf("read managed %s: %w", path, err)
		}
		if guard, ok := guards[path]; ok && !bytes.Equal(guard.data, data) {
			rollback()
			return nil, fmt.Errorf("%s changed while sync ran, likely written by the tool itself; nothing was written, so rerun the sync", path)
		}
		info, err := os.Stat(path)
		if err != nil {
			rollback()
			return nil, fmt.Errorf("stat managed %s: %w", path, err)
		}
		done = append(done, prior{path: path, data: data, mode: info.Mode()})
		if err := os.Remove(path); err != nil {
			rollback()
			return nil, fmt.Errorf("remove managed %s: %w", path, err)
		}
	}
	return linked, nil
}
