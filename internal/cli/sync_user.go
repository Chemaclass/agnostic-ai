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
	userStart   = "<!-- agnostic-ai:global:start -->"
	userEnd     = "<!-- agnostic-ai:global:end -->"
	envUserRoot = "AGNOSTIC_AI_HOME"
	// Version 6 records the hooks file containers sync created; version
	// 8 records the permission rules sync wrote; version 9 records the
	// settings parents sync created; version 10 records the original of
	// each key a personal field writes, per target and key; version 11
	// records the reviewer preset sync listed in a permissions file.
	userStateVersion = 11
)

type userSyncOptions struct {
	targets, only, except                                                    []string
	dryRun, check, backup, plan, watch, watchPoll, jsonOut, allTargets, diff bool
	format, gitignore                                                        string
	jobs                                                                     int
}

type userWrite struct {
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
func (w *userWrite) addTarget(target string) {
	if target != "" && !slices.Contains(w.targets, target) {
		w.targets = append(w.targets, target)
	}
}

func runUserSync(cmd *cobra.Command, o userSyncOptions) error {
	if err := validateUserFlags(o); err != nil {
		return err
	}
	home, err := userHomeDir()
	if err != nil {
		return err
	}
	source := userSourceHome(home)
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
	if err := requireUserVersion(source, skipBroken); err != nil {
		return err
	}
	targets, configured, explicit, err := resolveUserTargets(o, source, warn)
	if err != nil {
		return err
	}
	names, err := loadUserBuiltins(source, skipBroken)
	if err != nil {
		return err
	}
	bundle, err := loadUserBundle(source, names)
	if err != nil {
		return err
	}
	warnShadowedSpecs(warn, bundle)
	if err := checkUserSpecIssues(bundle, targets); err != nil {
		return err
	}
	tiers, unloaded, err := loadUserModels(source, skipBroken)
	if err != nil {
		return err
	}
	if err := applyUserTiers(&bundle, tiers, unloaded, warn); err != nil {
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
	userAgentWarnings, err := warnUnsupportedUserAgents(diagnostics, source, targets, bundle.Agents)
	if err != nil {
		return err
	}
	userMCPWarnings, err := warnUnsupportedUserMCP(diagnostics, targets, bundle.MCPs)
	if err != nil {
		return err
	}
	if err := checkUserRules(bundle.Rules, targets); err != nil {
		return err
	}
	instructions, err := userInstructions(source, tiers, bundle.Rules)
	if err != nil {
		return err
	}

	statePath := filepath.Join(source, "state", "global.json")
	old, err := loadUserState(statePath)
	if err != nil {
		return err
	}
	if foreign := foreignUserPath(old, home); foreign != "" {
		return fmt.Errorf("%s: recorded under another home, since %s is outside %s; sync under the HOME that recorded it, or remove the state file and the files it lists", statePath, foreign, home)
	}
	adapters.ResetCoverageNotes()
	adapters.SetWarner(diagnostics)
	defer adapters.SetWarner(os.Stderr)
	defer adapters.ResetCoverageNotes()
	if slices.Contains(targets, "claude") && slices.Contains(configured, "cursor") {
		claude.NoteCursorDropsArgs(bundle.HooksFor("claude"), "~/.claude/settings.json")
	}
	// requireUserVersion already warned about a broken config under -t.
	onUnsupported, err := loadUserOnUnsupported(source)
	if err != nil && skipBroken == nil {
		return err
	}
	writes, next, err := buildUserWrites(home, source, targets, instructions, bundle, old, agentFailure(explicit, warn), warn, onUnsupported)
	if err != nil {
		return err
	}
	drops := syncDrops{Warnings: append(userAgentWarnings, userMCPWarnings...), Notes: dropRecords(adapters.PendingCoverageNotes())}
	adapters.FlushCoverageNotes()
	if err := checkUnselectedUserAgents(writes, old, targets); err != nil {
		return err
	}
	stateData, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", statePath, err)
	}
	writes = append(writes, userWrite{path: statePath, data: append(stateData, '\n'), mode: 0o644})
	var trees []string
	for _, target := range targets {
		trees = append(trees, userTargets[target].trees(home)...)
	}
	adopted, err := preflightUserWrites(writes, trees, old)
	if err != nil {
		return err
	}
	removals := removedUserFiles(old.Files, next.Files, writes...)
	run := userRun{
		cmd: cmd, o: o, warn: warn, source: source, statePath: statePath,
		writes: writes, removals: removals, existing: existingPaths(removals), adopted: adopted, trees: trees,
		old: old, next: next, drops: drops, targets: len(targets), jsonWritten: &jsonWritten,
	}
	return run.output()
}

// validateUserFlags rejects flags --scope user cannot honor. The first
// failing check wins, so the order of the checks is part of the message.
func validateUserFlags(o userSyncOptions) error {
	if o.watch || o.watchPoll || o.allTargets || o.gitignore != "" || o.jobs != 0 {
		return errs.Coded(errs.CodeFlagConflict, "--scope user does not support --watch, --watch-poll, --all, --gitignore, or --jobs")
	}
	if o.plan && o.dryRun {
		return errs.Coded(errs.CodeFlagConflict, "--plan cannot be combined with --dry-run")
	}
	if o.jsonOut && o.diff {
		return errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --diff")
	}
	if o.diff && !o.check && !o.dryRun && !o.plan {
		return errs.Coded(errs.CodeFlagConflict, "--diff requires --check, --plan, or --dry-run with --scope user")
	}
	if len(o.only) > 0 && len(o.except) > 0 {
		return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
	}
	if err := validateCheckFormat(o.format); err != nil {
		return err
	}
	if o.format != checkFormatHuman && !o.check {
		return errs.Coded(errs.CodeFlagConflict, "--format requires --check with --scope user")
	}
	return nil
}

// resolveUserTargets returns the targets this run syncs, the home
// config's targets before --only and --except, and whether the user
// named the targets.
func resolveUserTargets(o userSyncOptions, source string, warn io.Writer) (targets, configured []string, explicit bool, err error) {
	targets = o.targets
	// A default run spans every supported target, so one target's
	// problem (a relative root variable, a name its native format
	// rejects) warns and skips that target instead of failing the rest.
	// A home config names its targets, as --only does.
	explicit = len(o.targets) > 0 || len(o.only) > 0
	if len(targets) == 0 {
		// -t replaces the home config, so a broken one does not block it.
		if targets, err = loadUserTargets(source, warn); err != nil {
			return nil, nil, false, err
		}
		explicit = explicit || targets != nil
	}
	if len(targets) == 0 {
		targets = userTargetNames()
	}
	// The home config's targets, before --only and --except, as project
	// sync reads agnostic-ai.yaml.
	configured = slices.Clone(targets)
	targets, err = filterTargets(targets, o.only, o.except)
	if err != nil {
		return nil, nil, false, err
	}
	for _, target := range targets {
		if err := unsupportedUserTarget("--scope user", target); err != nil {
			return nil, nil, false, err
		}
	}

	var usable []string
	for _, target := range targets {
		err := userTargets[target].rootError(target)
		if err == nil {
			usable = append(usable, target)
			continue
		}
		if explicit {
			return nil, nil, false, err
		}
		if _, werr := fmt.Fprintf(warn, "warning: %v; skipping %s\n", err, target); werr != nil {
			return nil, nil, false, fmt.Errorf("write user target warning: %w", werr)
		}
	}
	return usable, configured, explicit, nil
}

// checkUserSpecIssues stops on a spec the selected targets cannot
// carry at user scope.
func checkUserSpecIssues(bundle spec.Bundle, targets []string) error {
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
	if slices.ContainsFunc(targets, func(t string) bool { return userTargets[t].settings.permissionLists }) {
		issues = append(issues, permissionCapabilityIssues(bundle.Settings)...)
	}
	return stopOnIssues(issues)
}

// checkUserRules requires every user rule to apply unconditionally.
func checkUserRules(rules []spec.Entry, targets []string) error {
	for _, rule := range rules {
		if slices.Contains(targets, "codebuddy") && !(codebuddy.Adapter{}).AlwaysOnRule(rule) {
			return fmt.Errorf("user rule %q is conditional for codebuddy; user rules must apply unconditionally", rule.Name)
		}
		if userRuleConditional(rule, targets) {
			return fmt.Errorf("user rule %q is scoped or conditional; user rules must apply unconditionally", rule.Name)
		}
	}
	return nil
}

// userRun is a planned user sync, ready for one output mode.
type userRun struct {
	cmd                *cobra.Command
	o                  userSyncOptions
	warn               io.Writer
	source, statePath  string
	writes             []userWrite
	removals, existing []string
	adopted, trees     []string
	old, next          userState
	drops              syncDrops
	targets            int
	// jsonWritten tells runUserSync the JSON carried its held warnings.
	jsonWritten *bool
}

// output checks, plans, previews, or applies the run, as the flags ask.
func (r userRun) output() error {
	o := r.o
	switch {
	case o.check && o.jsonOut && !o.plan:
		return r.checkJSON()
	case o.check && !o.plan:
		return checkUserWrites(r.cmd, r.writes, r.existing, r.statePath, r.next, o.diff)
	case o.plan || (o.dryRun && o.jsonOut):
		return r.plan()
	case o.dryRun && o.diff:
		return r.dryRunDiff()
	case o.dryRun:
		return r.dryRun()
	default:
		return r.apply()
	}
}

func (r userRun) checkJSON() error {
	records := userFileRecords(r.writes, r.existing, r.statePath, r.next, true)
	*r.jsonWritten = true
	if len(records) == 0 && !userStateCurrent(r.statePath, r.next) {
		r.drops.Notes = append(r.drops.Notes, dropRecord{Kind: "ownership", Count: 1, Message: userOwnershipNote(r.statePath)})
	}
	if err := emitUserJSON(r.cmd, "sync --scope user --check", records, r.drops); err != nil {
		return err
	}
	if len(records) > 0 {
		return errDriftDetected()
	}
	return nil
}

func (r userRun) plan() error {
	o, writes := r.o, r.writes
	records := userFileRecords(writes, r.existing, r.statePath, r.next, false)
	// Mark what would stop the real run, as a key conflict is.
	edited, err := handEditedUserFiles(writes, r.removals, r.old)
	if err != nil {
		return err
	}
	for i := range records {
		path := filepath.FromSlash(records[i].Path)
		if slices.Contains(edited, path) {
			records[i].Keys = append(records[i].Keys, "conflict: edited since the last user sync; move the edit into the source or use --backup")
		}
		if j := slices.IndexFunc(writes, func(w userWrite) bool { return w.path == path }); j >= 0 && writes[j].dropsComments {
			records[i].Keys = append(records[i].Keys, "conflict: the rewrite drops the file's comments; use --backup")
		}
	}
	var driftErr error
	if o.check && len(userFileRecords(writes, r.existing, r.statePath, r.next, true)) > 0 {
		driftErr = errDriftDetected()
	}
	if o.jsonOut {
		command := "sync --scope user --plan"
		if o.dryRun {
			command = "sync --scope user --dry-run"
		}
		*r.jsonWritten = true
		return errors.Join(emitUserJSON(r.cmd, command, records, r.drops), driftErr)
	}
	if err := printUserPlan(r.cmd, records); err != nil {
		return err
	}
	if o.diff {
		if _, err := userDrift(r.cmd, writes, r.existing, r.statePath, r.next, true); err != nil {
			return err
		}
	}
	return driftErr
}

func (r userRun) dryRunDiff() error {
	if !r.o.backup {
		if err := refuseUserOverwrites(r.writes, r.removals, r.old, r.source); err != nil {
			return err
		}
	}
	if _, err := userDrift(r.cmd, r.writes, r.existing, r.statePath, r.next, true); err != nil {
		return err
	}
	return printUserAdoptions(r.cmd, r.adopted, r.writes)
}

func (r userRun) dryRun() error {
	out := r.cmd.OutOrStdout()
	for _, w := range r.writes {
		if data, err := os.ReadFile(w.path); err == nil && bytes.Equal(data, w.data) {
			continue
		}
		if _, err := fmt.Fprintf(out, "dry-run: write %s\n", w.path); err != nil {
			return fmt.Errorf("write dry-run output: %w", err)
		}
		for _, change := range w.changes {
			if _, err := fmt.Fprintf(out, "dry-run:   %s\n", change); err != nil {
				return fmt.Errorf("write dry-run output: %w", err)
			}
		}
	}
	for _, path := range r.existing {
		if _, err := fmt.Fprintf(out, "dry-run: remove %s\n", path); err != nil {
			return fmt.Errorf("write dry-run output: %w", err)
		}
	}
	return printUserAdoptions(r.cmd, r.adopted, r.writes)
}

func (r userRun) apply() error {
	if !r.o.backup {
		if err := refuseUserOverwrites(r.writes, r.removals, r.old, r.source); err != nil {
			return err
		}
	}
	var applied []userFileRecord
	if r.o.jsonOut {
		applied = userFileRecords(r.writes, r.existing, r.statePath, r.next, false)
	}
	linked, err := applyUserChanges(r.writes, r.removals, r.o.backup, r.next.removalGuards)
	if err != nil {
		return err
	}
	pruneEmptyUserDirs(r.removals, r.trees)
	if r.o.jsonOut {
		*r.jsonWritten = true
		return emitUserJSON(r.cmd, "sync --scope user", applied, r.drops)
	}
	for _, link := range linked {
		if _, err := fmt.Fprintln(r.warn, link); err != nil {
			return fmt.Errorf("write symlink note: %w", err)
		}
	}
	for _, path := range r.adopted {
		if _, err := fmt.Fprintf(r.warn, "adopted %s: matches what sync writes\n", path); err != nil {
			return fmt.Errorf("write adoption note: %w", err)
		}
	}
	for _, line := range settingsAdoptions(r.writes) {
		if _, err := fmt.Fprintf(r.warn, "adopted %s: matches what sync writes\n", line); err != nil {
			return fmt.Errorf("write adoption note: %w", err)
		}
	}
	if _, err := fmt.Fprintf(r.cmd.OutOrStdout(), "Synced user configuration to %d target(s).\n", r.targets); err != nil {
		return fmt.Errorf("write sync summary: %w", err)
	}
	return nil
}

func warnUnsupportedUserAgents(w io.Writer, source string, targets []string, agents []spec.Entry) ([]dropRecord, error) {
	warnings := []dropRecord{}
	var unsupported []string
	for _, target := range targets {
		if userTargets[target].agents != "" {
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
		message := "user agents are unsupported; skipping " + strings.Join(skipped, ", ")
		if _, err := fmt.Fprintf(w, "warning: %s: %s\n", target, message); err != nil {
			return nil, fmt.Errorf("write user agent warning: %w", err)
		}
		warnings = append(warnings, dropRecord{Target: target, Kind: string(spec.KindAgent), Count: len(skipped), Message: message})
		unsupported = append(unsupported, target)
	}
	if len(warnings) > 0 {
		configs := userConfigPaths(source)
		suggestion := fmt.Sprintf("set `targets:` to the tools you use in %s (or in %s, which overrides it), or add `targets-exclude: [%s]` to each affected agent", configs[0], configs[1], strings.Join(unsupported, ", "))
		for i := range warnings {
			warnings[i].Suggestion = suggestion
		}
		if _, err := fmt.Fprintf(w, "    fix: %s\n", suggestion); err != nil {
			return nil, fmt.Errorf("write user agent suggestion: %w", err)
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
			return fmt.Errorf("write user agent warning: %w", werr)
		}
		return nil
	}
}

func userRuleConditional(rule spec.Entry, targets []string) bool {
	if rule.Scope != "" || hasUserRuleCondition(rule.Meta) {
		return true
	}
	return slices.ContainsFunc(targets, func(target string) bool {
		return hasUserRuleCondition(adapters.ResolveMeta(rule.Meta, target))
	})
}

func hasUserRuleCondition(meta map[string]any) bool {
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

// userSum fingerprints what sync owns in a file: the managed block
// when the file has one, otherwise the whole content.
func userSum(data []byte) string {
	return adapters.ContentSum(userBlock(data))
}

// userBlock returns the managed block when data carries one, otherwise
// all of data.
func userBlock(data []byte) string {
	body := string(data)
	start, end := strings.Index(body, userStart), strings.Index(body, userEnd)
	if start >= 0 && end > start {
		return body[start : end+len(userEnd)]
	}
	return body
}

func sharedUserSkillsDir(home, dir string) bool {
	readers := 0
	for _, target := range userTargets {
		if target.skills != "" && target.path(home, target.skills) == dir {
			readers++
			if readers > 1 {
				return true
			}
		}
	}
	return false
}

// userSkillOverlays maps the relative paths any target reading dir
// renders beside SKILL.md for skill to their rendered content. Every
// co-writer of a shared tree ships that content in place of a bundled
// asset at the same path, so they agree and the spec-rendered file
// wins, as in project sync.
func userSkillOverlays(home, dir string, skill spec.Entry) (map[string]string, error) {
	overlays := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(userTargets)) {
		g := userTargets[name]
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

func addUserSkill(dst string, skill spec.Entry, target string, shared bool, overlays map[string]string, add func(string, []byte, fs.FileMode) error) error {
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

// mergeUserBlock edits only the managed block and the one newline
// separator sync inserts before it, so the user's bytes around it stay
// as they were and add then remove restores the file.
func mergeUserBlock(path, managed string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return mergeUserBlockData(path, managed, data)
}

func mergeUserBlockData(path, managed string, data []byte) (string, error) {
	body := string(data)
	start, end := strings.Index(body, userStart), strings.Index(body, userEnd)
	if (start >= 0) != (end >= 0) || (start >= 0 && end < start) {
		return "", fmt.Errorf("%s: corrupt agnostic-ai managed block", path)
	}
	removing := strings.TrimSpace(managed) == userStart+"\n\n"+userEnd
	if start < 0 {
		return insertUserBlock(body, managed, removing), nil
	}
	end += len(userEnd)
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

func aliasedUserInstructions(a, b string) bool {
	return resolveImportSourceExisting(a) == resolveImportSourceExisting(b)
}

func insertUserBlock(body, managed string, removing bool) string {
	switch {
	case removing:
		return body
	case body == "":
		return managed + "\n"
	default:
		return body + "\n" + managed + "\n"
	}
}

// mergeUserAgentEfforts sets subagents.agents.<name>.effortLevel for
// each managed effort in a JSONC user settings file, removing values an
// earlier sync placed. A value set by hand stops the run. It returns nil
// when the file needs no change; a rewrite keeps key order and indent,
// and dropsComments reports that it loses the file's comments.
func mergeUserAgentEfforts(path string, efforts, previous map[string]string) (doc []byte, dropsComments bool, err error) {
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

func userHomeDir() (string, error) {
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return home, nil
}

// userBridgePath returns the managed context-bridge script path for a
// target whose hooks file lives in base.
func userBridgePath(base string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(base, "hooks", "agnostic-ai-global-context.ps1")
	}
	return filepath.Join(base, "hooks", "agnostic-ai-global-context.sh")
}

func userContextBridge(base, body, key string) (path, command, script string, mode fs.FileMode) {
	payload := `{` + jsonString(key) + `:` + jsonString(body) + `}`
	path = userBridgePath(base)
	if runtime.GOOS == "windows" {
		command = `powershell -NoProfile -ExecutionPolicy Bypass -File "` + strings.ReplaceAll(path, `"`, `\"`) + `"`
		script = "$payload = '" + strings.ReplaceAll(payload, "'", "''") + "'\r\n[Console]::Out.WriteLine($payload)\r\n"
		return path, command, script, 0o644
	}
	return path, path, "#!/bin/sh\nprintf '%s\\n' " + adapters.ShellQuote(payload) + "\n", 0o755
}
