package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitexclude"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type builtinFileChange struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Diff   string `json:"diff"`
	before editFileSnapshot
	after  []byte
	remove bool
	mode   os.FileMode
}

type builtinChangePlan struct {
	view     builtinChangeView
	audience editAudience
	change   projectChange
	inputs   builtinProjectSnapshot
	changes  []builtinFileChange
	// excludeBase holds the info/exclude bytes each planned exclude was
	// built from, read before the plan's snapshot. Apply refuses when the
	// file no longer holds them, as after a nested project's sync.
	excludeBase map[string][]byte
}

// projectChange is one source edit the web flow previews and applies:
// a built-in switch, or a scalar settings field when setting is set.
type projectChange struct {
	builtin builtinRequest
	setting *settingRequest
}

func planBuiltinChange(root string, req builtinRequest, where editAudience) (*builtinChangePlan, error) {
	return planProjectChange(root, projectChange{builtin: req}, where)
}

func planProjectChange(root string, change projectChange, where editAudience) (*builtinChangePlan, error) {
	return planProjectChangeLocked(root, change, where, nil)
}

func planProjectChangeLocked(root string, change projectChange, where editAudience, guard *gitexclude.Guard) (*builtinChangePlan, error) {
	req := change.builtin
	defer adapters.SetAsideNotes()()
	initial, err := snapshotBuiltinProject(root, []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName, stateFilePath(".")})
	if err != nil {
		return nil, err
	}
	lockPath := filepath.Join(root, ".agnostic-ai", projectLockName)
	if err := builtinSafeParents(root, lockPath); err != nil {
		return nil, err
	}
	if _, err := builtinLockMissing(lockPath); err != nil {
		return nil, err
	}
	cfg, err := config.Load(".")
	if err != nil {
		return nil, err
	}
	for _, target := range cfg.Targets {
		if !slices.Contains(adapters.Names(), target) {
			return nil, fmt.Errorf("%s: web changes require built-in adapters; use the CLI to change this project", target)
		}
	}
	if err := validateBuiltinConfigPaths(root, cfg); err != nil {
		return nil, err
	}
	if cfg.Sync.SharedSkills {
		return nil, fmt.Errorf("sync.shared-skills: web changes do not replace linked skill folders; use the CLI to change built-ins")
	}
	if change.setting == nil && !req.reset && slices.Contains(cfg.Builtins, req.name) == req.enabled {
		return nil, fmt.Errorf("%s already has the requested value; refresh the overview", req.name)
	}
	previous, err := readStateFileStrict(".")
	if err != nil {
		return nil, err
	}
	if previous.Version == 0 && initial.files[filepath.ToSlash(stateFilePath("."))].info != nil {
		return nil, fmt.Errorf("run agnostic-ai sync before previewing a web change")
	}
	inputs, err := builtinProjectInputs(cfg)
	if err != nil {
		return nil, err
	}
	inputRoots := slices.Clone(inputs)
	inputs = append(inputs, previous.Outputs...)
	inputs = append(inputs, gitignoreRel(cfg), worktreeIncludeFile, stateFilePath("."))
	before, err := snapshotBuiltinProject(root, inputs)
	if err != nil {
		return nil, err
	}
	if err := initial.check(root); err != nil {
		return nil, err
	}
	freshInputs, err := builtinProjectInputs(cfg)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(inputRoots, freshInputs) {
		return nil, fmt.Errorf("source references changed while preparing preview; preview again")
	}
	cfg, bundle, err := loadProject(".")
	if err != nil {
		return nil, err
	}
	baseline, err := collectLoadedDrift(cfg, bundle, cfg.Targets, nil, "")
	if err != nil {
		return nil, err
	}
	if err := validateBuiltinOutputs(root, baseline, previous); err != nil {
		return nil, err
	}
	var file string
	var edit func([]byte) ([]byte, error)
	if change.setting != nil {
		file, edit, err = settingSourceEdit(root, where, *change.setting)
	} else {
		file, edit, err = (&editHandler{root: root}).builtinsEdit(where, req)
	}
	if err != nil {
		return nil, err
	}
	configPath, err := builtinRelativePath(root, file)
	if err != nil {
		return nil, err
	}
	original, err := snapshotEditFile(filepath.Join(root, configPath))
	if err != nil {
		return nil, err
	}
	edited, err := edit(original.data)
	var stale staleFieldError
	if errors.As(err, &stale) {
		return nil, builtinChanged(fmt.Errorf("%s: %w", configPath, err))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	if len(edited) == 0 {
		edited = nil
	}
	if change.setting != nil && (edited == nil) == (original.info == nil) && bytes.Equal(edited, original.data) {
		return nil, fmt.Errorf("%s already has the requested value; refresh the overview", change.setting.addr.label())
	}
	cfg.Builtins = slices.Clone(cfg.Builtins)
	switch {
	case change.setting != nil:
	case req.reset:
		if cfg.Builtins, err = config.TeamBuiltins(root); err != nil {
			return nil, err
		}
	case req.enabled:
		cfg.Builtins = append(cfg.Builtins, req.name)
	default:
		cfg.Builtins = slices.DeleteFunc(cfg.Builtins, func(current string) bool { return current == req.name })
	}
	layers, err := resolveLayers(".", cfg)
	if err != nil {
		return nil, err
	}
	planned := map[string][]byte{}
	if change.setting != nil {
		planned[filepath.Clean(filepath.FromSlash(configPath))] = edited
		layers = plannedSourceLayers(layers, planned)
	}
	bundle, err = spec.LoadLayered(layers)
	if err != nil {
		return nil, err
	}
	bundle.ApplyModelTiers(cfg.Models)
	scope := loadedProjectCheckScope(cfg, bundle)
	scope.bundle.Hooks = slices.DeleteFunc(slices.Clone(scope.bundle.Hooks), func(hook spec.Entry) bool {
		return hook.Layer == layerNameBuiltin && !slices.ContainsFunc(scope.hookTargets, hook.EmitsTo)
	})
	if issues := scopeValidationIssues(scope); len(issues) > 0 {
		return nil, fmt.Errorf("%s: %s", issues[0].Path, issues[0].Message)
	}
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	reports, err := collectLoadedDrift(cfg, bundle, cfg.Targets, nil, "")
	if err != nil {
		return nil, err
	}
	if err := validateBuiltinMemory(root, cfg, reports); err != nil {
		return nil, err
	}
	outputBefore, err := snapshotBuiltinProject(root, driftGeneratedPaths(reports))
	if err != nil {
		return nil, err
	}
	if err := validateBuiltinOutputs(root, reports, previous); err != nil {
		return nil, err
	}
	if slices.Contains(cfg.Targets, "codex") {
		hooksPath := codex.HooksFilePath(cfg)
		for _, report := range reports {
			for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
				for _, file := range files {
					if file.Path == hooksPath {
						codex.NoteHookTrust(hooksPath, []byte(file.Content))
					}
				}
			}
		}
	}
	if _, err := applyCoverageAccept(cfg, cfg.Targets); err != nil {
		return nil, err
	}
	plan := &builtinChangePlan{audience: where, change: change, excludeBase: map[string][]byte{}, view: builtinChangeView{SchemaVersion: "builtin-change/v1", Status: "proposed", Name: req.name, Enabled: req.enabled, Reset: req.reset, Audience: string(where), Scope: where.legacyScope(), Config: configPath, Targets: slices.Clone(cfg.Targets), Changes: []builtinFileChange{}, Findings: []string{}}}
	if change.setting != nil {
		plan.view.Name, plan.view.Key, plan.view.Target, plan.view.Title = change.setting.addr.key, change.setting.addr.key, change.setting.addr.target, change.setting.title(where)
	}
	if slices.ContainsFunc(baseline, func(r driftReport) bool { return r.hasDrift() }) {
		plan.view.Findings = append(plan.view.Findings, "This preview also applies pending source changes.")
	}
	for _, drop := range append(adapters.PendingCapabilityWarnings(), adapters.PendingCoverageNotes()...) {
		plan.view.Findings = append(plan.view.Findings, preview.RedactText(drop.Target+": "+drop.Message))
	}
	desired := map[string][]byte{configPath: edited}
	sums := map[string]string{}
	modes := map[string]os.FileMode{}
	merged := map[string]mergedOutput{}
	var outputs []string
	for _, report := range reports {
		for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
			for _, file := range files {
				path, err := builtinRelativePath(root, file.Path)
				if err != nil {
					return nil, err
				}
				if old, exists := desired[path]; exists && !bytes.Equal(old, []byte(file.Content)) {
					return nil, fmt.Errorf("%s: conflicting planned output", path)
				}
				desired[path] = []byte(file.Content)
				if file.Mode != 0 {
					if old := modes[path]; old != 0 && old != file.Mode {
						return nil, fmt.Errorf("%s: conflicting planned permissions", path)
					}
					modes[path] = file.Mode
				}
				inputs = append(inputs, path)
				sum := adapters.ContentSum(file.Content)
				sums[file.Path] = sum
				outputs = append(outputs, file.Path)
				snap, err := snapshotEditFile(filepath.Join(root, path))
				if err != nil {
					return nil, err
				}
				action := "skip"
				if snap.info == nil {
					action = "create"
				} else if !bytes.Equal(snap.data, []byte(file.Content)) {
					action = "update"
				}
				recordMergedWrites([]adapters.WrittenFile{{Path: file.Path, Action: action, Sum: sum, Merged: file.Merged, Keys: file.Keys, Released: file.Released}}, merged)
			}
		}
		for _, path := range report.Leftover {
			rel, err := builtinRelativePath(root, path)
			if err != nil {
				return nil, err
			}
			current, err := snapshotEditFile(filepath.Join(root, rel))
			if err != nil {
				return nil, err
			}
			if current.info == nil || previous.OutputSums[path] != adapters.ContentSum(string(current.data)) {
				return nil, fmt.Errorf("%s: sync cannot prove ownership; use agnostic-ai doctor", rel)
			}
			if ownership, isMerged := previous.Merged[path]; isMerged {
				data, write, err := captureBuiltinMergedRelease(path, ownership)
				if err != nil {
					return nil, err
				}
				if write {
					desired[rel] = data
				}
			} else {
				desired[rel] = nil
			}

			inputs = append(inputs, rel)
		}
	}
	if fileExists(adapters.AgnosticEntryPointPath) {
		outputs = append(outputs, adapters.AgnosticEntryPointPath)
	}
	outputs = finalizeLedger(outputs)
	// Sync adds these ignore entries every time it takes the project lock.
	rootIgnore, err := snapshotEditFile(".gitignore")
	if err != nil {
		return nil, err
	}
	desired[".gitignore"] = []byte(withRuntimeIgnores(string(rootIgnore.data)))
	if resolveGitignore(cfg, "") {
		team := cfg.Builtins
		if configPath == config.LocalOverrideFileName {
			if team, err = config.TeamBuiltins("."); err != nil {
				return nil, err
			}
		}
		prev, err := previousPersonalLocked(root, guard)
		if err != nil {
			return nil, err
		}
		block, personal, err := splitManagedBlock(".", cfg, newTargetRenders(cfg, bundle), team, prev, cfg.Targets, outputs, ledgerMissing("."))
		if err != nil {
			return nil, err
		}
		if err := planPersonalExcludes(root, personal, withLeftoverBackups(nil, block, syncBackups(previous.Backups, nil)), desired, plan.excludeBase); err != nil {
			return nil, err
		}
		ignore, err := builtinRelativePath(root, gitignoreRel(cfg))
		if err != nil {
			return nil, err
		}
		existing, err := snapshotEditFile(ignore)
		if err != nil {
			return nil, err
		}
		base := existing.data
		if planned, ok := desired[ignore]; ok {
			base = planned
		}
		desired[ignore] = []byte(replaceManagedBlock(stripLooseFixedDuplicates(string(base)), block))
		existing, err = snapshotEditFile(worktreeIncludeFile)
		if err != nil {
			return nil, err
		}
		rendered := renderWorktreeInclude(string(existing.data), cfg, block)
		if strings.TrimSpace(rendered) == "" && existing.info != nil {
			desired[worktreeIncludeFile] = nil
		} else if rendered != "" {
			desired[worktreeIncludeFile] = []byte(rendered)
		}
	}
	excludePath, exclusion, err := planBuiltinMemoryExclusion(root, cfg, desired, plan.excludeBase)
	if err != nil {
		return nil, err
	}
	baselineSums := specSums(cfg, bundle, cfg.Targets)
	next := previous
	next.Version = syncStateVersion
	next.FilesChanged = 0
	next.SyncedAt = time.Now().UTC()
	next.Outputs = outputs
	next.OutputSums = ledgerSums(outputs, sums, previous.OutputSums)
	next.Merged = ledgerMerged(outputs, merged, sums, previous, nil)
	next.SpecSums, next.SpecTargets = baselineSums.sums, baselineSums.targets
	next.SpecFileSums = syncedSpecFileSumsFrom(".", previous.SpecFileSums, bundle, cfg.Targets, cfg, plannedRead(planned))
	next.Orphans, next.Unledgered = nil, nil
	adapters.OrderBufferedDropsByTarget(cfg.Targets)
	next.WarningsDigest = adapters.CapabilityWarningsDigest()
	next.NotesDigest = adapters.CoverageNotesDigest()
	next.WarningRecords = dropFingerprints(adapters.PendingCapabilityWarnings())
	next.NoteRecords = dropFingerprints(adapters.PendingCoverageNotes())
	for path, data := range desired {
		snap, err := snapshotEditFile(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}
		if data == nil && snap.info != nil || data != nil && (snap.info == nil || !bytes.Equal(snap.data, data)) {
			next.FilesChanged++
		}
	}
	state, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", stateFilePath("."), err)
	}
	desired[filepath.ToSlash(stateFilePath("."))] = state
	if excludePath != "" {
		desired[excludePath] = exclusion
	}
	for path := range desired {
		inputs = append(inputs, path)
	}
	plan.inputs, err = snapshotBuiltinProject(root, inputs)
	if err != nil {
		return nil, err
	}
	if err := outputBefore.check(root); err != nil {
		return nil, err
	}
	if err := before.check(root); err != nil {
		return nil, err
	}
	if err := initial.check(root); err != nil {
		return nil, err
	}
	for _, path := range slices.Sorted(maps.Keys(desired)) {
		data := desired[path]
		snap, recorded := plan.inputs.files[path]
		if !recorded {
			return nil, fmt.Errorf("%s: preview did not record the original file state", path)
		}
		if data == nil && snap.info == nil || data != nil && snap.info != nil && bytes.Equal(snap.data, data) && (modes[path] == 0 || snap.info.Mode().Perm() == modes[path]) {
			continue
		}
		action := "update"
		if data == nil {
			action = "delete"
		} else if snap.info == nil {
			action = "create"
		}
		change := builtinFileChange{Path: path, Action: action, before: snap, after: data, remove: data == nil, mode: modes[path]}
		change.Diff = preview.RedactText(previewDiff(path, path+" (before)", path+" (proposed)", string(snap.data), string(data), snap.info == nil, len(snap.data)+len(data)+1))
		if snap.info != nil && modes[path] != 0 && snap.info.Mode().Perm() != modes[path] {
			change.Diff += fmt.Sprintf("\nPermissions: %04o -> %04o\n", snap.info.Mode().Perm(), modes[path])
		}
		plan.changes = append(plan.changes, change)
		plan.view.Changes = append(plan.view.Changes, change)
	}
	return plan, nil
}

func builtinProjectInputs(cfg *config.Config) ([]string, error) {
	inputs, err := projectInputs(cfg)
	if err != nil {
		return nil, err
	}
	// Web previews track source trees, not unrelated local working files.
	inputs = slices.DeleteFunc(inputs, func(path string) bool { return path == config.SourceBaseDir+"/**" })
	inputs = append(inputs,
		adapters.AgnosticEntryPointPath,
		filepath.Join(defaultProjectUser, "AGNOSTIC_AI.md"),
		agnosticOverlayDir+"/**",
		agnosticScriptsDir+"/**",
		packsDir+"/**",
		config.SourceBaseDir+"/memory/**",
		defaultProjectUser+"/memory/**",
	)
	for _, layer := range []struct {
		root    string
		sources config.Sources
	}{
		{".", cfg.Sources},
		{defaultProjectUser, defaultLayerSources()},
	} {
		for _, source := range sourceDirsByKind(layer.sources) {
			if source != "" {
				inputs = append(inputs, filepath.ToSlash(config.ResolveSourcePath(layer.root, source))+"/**")
			}
		}
	}
	slices.Sort(inputs)
	return slices.Compact(inputs), nil
}

func builtinRelativePath(root, path string) (string, error) {
	absolute := path
	if !filepath.IsAbs(path) {
		absolute = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(root, absolute)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: web changes require paths inside this project; use the CLI", path)
	}
	return filepath.ToSlash(rel), nil
}

func builtinLockMissing(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s: project lock is not a regular file; use the CLI", path)
	}
	return false, nil
}
