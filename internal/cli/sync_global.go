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
	"strings"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const (
	globalStart       = "<!-- agnostic-ai:global:start -->"
	globalEnd         = "<!-- agnostic-ai:global:end -->"
	envUserGlobalRoot = "AGNOSTIC_AI_HOME"
	// Version 6 records the hooks file containers sync created; version
	// 8 records the permission rules sync wrote; version 9 records the
	// settings parents sync created; version 10 records the original of
	// each key a personal field writes, per target and key.
	globalStateVersion = 10
)

type globalSyncOptions struct {
	targets, only, except                                                    []string
	dryRun, check, backup, plan, watch, watchPoll, jsonOut, allTargets, diff bool
	format, gitignore                                                        string
	jobs                                                                     int
}

type globalWrite struct {
	path string
	data []byte
	mode fs.FileMode
	// owned marks a file sync writes whole, or a managed block in it.
	owned    bool
	keepMode bool
	// keepAliases retires old ownership without removing the same active file.
	keepAliases []string
	// dropsComments marks a JSONC rewrite that loses the file's comments.
	dropsComments bool
	// changes, adopted, and conflicts describe a key-level edit of a
	// user settings file; conflicts need --backup.
	changes, adopted, conflicts []string
	// targets names the targets whose surfaces this write carries.
	targets []string
	// planned is the file as a merged edit read it, for a file
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
	if o.plan && o.dryRun {
		return errs.Coded(errs.CodeFlagConflict, "--plan cannot be combined with --dry-run")
	}
	if o.jsonOut && o.diff {
		return errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --diff")
	}
	if o.diff && !o.check && !o.dryRun && !o.plan {
		return errs.Coded(errs.CodeFlagConflict, "--diff requires --check, --plan, or --dry-run with --global")
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
	names, err := loadGlobalBuiltins(source, skipBroken)
	if err != nil {
		return err
	}
	bundle, err := loadGlobalBundle(source, names)
	if err != nil {
		return err
	}
	own := ownSpecs(bundle)
	issues := append(portableHookProblems(own.Hooks), agentCapabilityIssues(own.Agents)...)
	issues = append(issues, agentLimitIssues(own.Agents)...)
	issues = append(issues, skillCapabilityIssues(bundle.Skills)...)
	issues = append(issues, sessionRetentionIssues(bundle.Settings)...)
	issues = append(issues, voiceIssues(bundle.Settings)...)
	issues = append(issues, agentOnlyIssues(bundle.MCPs)...)
	issues = append(issues, mcpRootsIssues(own.MCPs)...)
	// A permission rule the target cannot read would be written as it
	// stands, so a deny rule could stop guarding anything.
	if slices.ContainsFunc(targets, func(t string) bool { return globalTargets[t].settings.permissionLists }) {
		issues = append(issues, permissionCapabilityIssues(bundle.Settings)...)
	}
	if err := stopOnIssues(issues); err != nil {
		return err
	}
	tiers, unloaded, err := loadGlobalModels(source, skipBroken)
	if err != nil {
		return err
	}
	if err := applyGlobalTiers(&bundle, tiers, unloaded, warn); err != nil {
		return err
	}
	// With --json, warnings and notes wait here and reach warn only when
	// the run fails before it prints the JSON that carries them.
	diagnostics := warn
	jsonWritten := false
	if o.jsonOut {
		var held bytes.Buffer
		diagnostics = &held
		defer func() {
			if !jsonWritten {
				_, _ = warn.Write(held.Bytes())
			}
		}()
	}
	globalAgentWarnings, err := warnUnsupportedGlobalAgents(diagnostics, source, targets, bundle.Agents)
	if err != nil {
		return err
	}
	globalMCPWarnings, err := warnUnsupportedGlobalMCP(diagnostics, targets, bundle.MCPs)
	if err != nil {
		return err
	}
	for _, rule := range bundle.Rules {
		if slices.Contains(targets, "codebuddy") && !(codebuddy.Adapter{}).AlwaysOnRule(rule) {
			return fmt.Errorf("global rule %q is conditional for codebuddy; global rules must apply unconditionally", rule.Name)
		}
		if globalRuleConditional(rule, targets) {
			return fmt.Errorf("global rule %q is scoped or conditional; global rules must apply unconditionally", rule.Name)
		}
	}
	instructions, err := globalInstructions(source, tiers, bundle.Rules)
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
	adapters.SetWarner(diagnostics)
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
	drops := syncDrops{Warnings: append(globalAgentWarnings, globalMCPWarnings...), Notes: dropRecords(adapters.PendingCoverageNotes())}
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
	removals := removedGlobalFiles(old.Files, next.Files, writes...)
	if o.check && o.jsonOut && !o.plan {
		records := globalFileRecords(writes, existingPaths(removals), statePath, next, true)
		jsonWritten = true
		if len(records) == 0 && !globalStateCurrent(statePath, next) {
			drops.Notes = append(drops.Notes, dropRecord{Kind: "ownership", Count: 1, Message: globalOwnershipNote(statePath)})
		}
		if err := emitGlobalJSON(cmd, "sync --global --check", records, drops); err != nil {
			return err
		}
		if len(records) > 0 {
			return errDriftDetected()
		}
		return nil
	}
	if o.check && !o.plan {
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
		var driftErr error
		if o.check && len(globalFileRecords(writes, existingPaths(removals), statePath, next, true)) > 0 {
			driftErr = errDriftDetected()
		}
		if o.jsonOut {
			command := "sync --global --plan"
			if o.dryRun {
				command = "sync --global --dry-run"
			}
			jsonWritten = true
			return errors.Join(emitGlobalJSON(cmd, command, records, drops), driftErr)
		}
		if err := printGlobalPlan(cmd, records); err != nil {
			return err
		}
		if o.diff {
			if _, err := globalDrift(cmd, writes, existingPaths(removals), statePath, next, true); err != nil {
				return err
			}
		}
		return driftErr
	}
	if o.dryRun && o.diff {
		if !o.backup {
			if err := refuseGlobalOverwrites(writes, removals, old, source); err != nil {
				return err
			}
		}
		if _, err := globalDrift(cmd, writes, existingPaths(removals), statePath, next, true); err != nil {
			return err
		}
		return printGlobalAdoptions(cmd, adopted, writes)
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
		return printGlobalAdoptions(cmd, adopted, writes)
	}
	if !o.backup {
		if err := refuseGlobalOverwrites(writes, removals, old, source); err != nil {
			return err
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
		jsonWritten = true
		return emitGlobalJSON(cmd, "sync --global", applied, drops)
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

func warnUnsupportedGlobalAgents(w io.Writer, source string, targets []string, agents []spec.Entry) ([]dropRecord, error) {
	warnings := []dropRecord{}
	var unsupported []string
	for _, target := range targets {
		if globalTargets[target].agents != "" {
			continue
		}
		var skipped []string
		for _, agent := range agents {
			if agent.EmitsTo(target) {
				skipped = append(skipped, agent.Name)
			}
		}
		if len(skipped) == 0 {
			continue
		}
		message := "global agents are unsupported; skipping " + strings.Join(skipped, ", ")
		if _, err := fmt.Fprintf(w, "warning: %s: %s\n", target, message); err != nil {
			return nil, fmt.Errorf("write global agent warning: %w", err)
		}
		warnings = append(warnings, dropRecord{Target: target, Kind: string(spec.KindAgent), Count: len(skipped), Message: message})
		unsupported = append(unsupported, target)
	}
	if len(warnings) > 0 {
		configs := globalConfigPaths(source)
		suggestion := fmt.Sprintf("set `targets:` to the tools you use in %s (or in %s, which overrides it), or add `targets-exclude: [%s]` to each affected agent", configs[0], configs[1], strings.Join(unsupported, ", "))
		for i := range warnings {
			warnings[i].Suggestion = suggestion
		}
		if _, err := fmt.Fprintf(w, "    fix: %s\n", suggestion); err != nil {
			return nil, fmt.Errorf("write global agent suggestion: %w", err)
		}
	}
	return warnings, nil
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

func globalRuleConditional(rule spec.Entry, targets []string) bool {
	if rule.Scope != "" || hasGlobalRuleCondition(rule.Meta) {
		return true
	}
	return slices.ContainsFunc(targets, func(target string) bool {
		return hasGlobalRuleCondition(adapters.ResolveMeta(rule.Meta, target))
	})
}

func hasGlobalRuleCondition(meta map[string]any) bool {
	for _, key := range []string{"scope", "globs", "paths", "target", "targets", "target-exclude", "targets-exclude"} {
		if _, ok := meta[key]; ok {
			return true
		}
	}
	return false
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
	return spec.WalkSourceRoot(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
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

// mergeGlobalBlock edits only the managed block and the one newline
// separator sync inserts before it, so the user's bytes around it stay
// as they were and add then remove restores the file.
func mergeGlobalBlock(path, managed string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return mergeGlobalBlockData(path, managed, data)
}

func mergeGlobalBlockData(path, managed string, data []byte) (string, error) {
	body := string(data)
	start, end := strings.Index(body, globalStart), strings.Index(body, globalEnd)
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return "", fmt.Errorf("%s: corrupt agnostic-ai managed block", path)
	}
	removing := strings.TrimSpace(managed) == globalStart+"\n\n"+globalEnd
	if start < 0 {
		return insertGlobalBlock(body, managed, removing), nil
	}
	end += len(globalEnd)
	if !removing {
		return body[:start] + managed + body[end:], nil
	}
	before, after := body[:start], body[end:]
	if rest, ok := strings.CutPrefix(after, "\r\n"); ok {
		after = rest
	} else {
		after = strings.TrimPrefix(after, "\n")
	}
	switch {
	case strings.HasSuffix(before, "\r\n\r\n"):
		before = before[:len(before)-2]
	case strings.HasSuffix(before, "\n\n"):
		before = before[:len(before)-1]
	case after == "" && strings.HasSuffix(before, "\n") && !strings.HasSuffix(before, "\r\n"):
		before = before[:len(before)-1]
	}
	return before + after, nil
}

func aliasedGlobalInstructions(a, b string) bool {
	return resolveImportSourceExisting(a) == resolveImportSourceExisting(b)
}

func insertGlobalBlock(body, managed string, removing bool) string {
	switch {
	case removing:
		return body
	case body == "":
		return managed + "\n"
	default:
		return body + "\n" + managed + "\n"
	}
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
