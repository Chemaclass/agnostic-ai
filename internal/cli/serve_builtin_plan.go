package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
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
	view    builtinChangeView
	inputs  builtinProjectSnapshot
	changes []builtinFileChange
}

func planBuiltinChange(root string, enabled bool) (*builtinChangePlan, error) {
	defer adapters.SetAsideNotes()()
	initial, err := snapshotBuiltinProject(root, []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName, stateFilePath(".")})
	if err != nil {
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
	if cfg.Sync.SharedSkills {
		return nil, fmt.Errorf("sync.shared-skills: web changes do not replace linked skill folders; use the CLI to change Caveman")
	}
	if slices.Contains(cfg.Builtins, "caveman") == enabled {
		return nil, fmt.Errorf("caveman already has the requested value; refresh the overview")
	}
	previous, err := readStateFileStrict(".")
	if err != nil {
		return nil, err
	}
	if previous.Version == 0 {
		return nil, fmt.Errorf("run agnostic-ai sync before previewing a web change")
	}
	inputs, err := projectInputs(cfg)
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
	freshInputs, err := projectInputs(cfg)
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
	if slices.ContainsFunc(baseline, func(r driftReport) bool { return r.hasDrift() }) {
		return nil, fmt.Errorf("generated files differ from the sources; run agnostic-ai doctor and sync before previewing a web change")
	}
	edit := &editHandler{root: root}
	configPath, err := builtinRelativePath(root, edit.builtinsFile())
	if err != nil {
		return nil, err
	}
	original, err := snapshotEditFile(filepath.Join(root, configPath))
	if err != nil {
		return nil, err
	}
	edited, err := setBuiltin(original.data, "caveman", enabled)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", configPath, err)
	}
	cfg.Builtins = slices.Clone(cfg.Builtins)
	if enabled {
		cfg.Builtins = append(cfg.Builtins, "caveman")
	} else {
		cfg.Builtins = slices.DeleteFunc(cfg.Builtins, func(name string) bool { return name == "caveman" })
	}
	layers, err := resolveLayers(".", cfg)
	if err != nil {
		return nil, err
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
	if _, err := applyCoverageAccept(cfg, cfg.Targets); err != nil {
		return nil, err
	}
	plan := &builtinChangePlan{view: builtinChangeView{SchemaVersion: "builtin-change/v1", Status: "proposed", Name: "caveman", Enabled: enabled, Config: configPath, Targets: slices.Clone(cfg.Targets), Changes: []builtinFileChange{}, Findings: []string{}}}
	for _, drop := range append(adapters.PendingCapabilityWarnings(), adapters.PendingCoverageNotes()...) {
		plan.view.Findings = append(plan.view.Findings, preview.RedactText(drop.Target+": "+drop.Message))
	}
	desired := map[string][]byte{configPath: edited}
	sums := map[string]string{}
	modes := map[string]os.FileMode{}
	merged := map[string]mergedOutput{}
	var outputs []string
	for _, report := range reports {
		if len(report.Blocking) > 0 || len(report.Orphaned) > 0 || report.Unledgered {
			return nil, fmt.Errorf("%s: output ownership needs manual review; run agnostic-ai doctor before changing Caveman", report.Target)
		}
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
					modes[path] = file.Mode
				}
				inputs = append(inputs, path)
				sum := adapters.ContentSum(file.Content)
				sums[file.Path] = sum
				outputs = append(outputs, file.Path)
				recordMergedWrites([]adapters.WrittenFile{{Path: file.Path, Action: "skip", Sum: sum, Merged: file.Merged, Keys: file.Keys, Released: file.Released}}, merged)
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
			if _, isMerged := previous.Merged[path]; isMerged {
				return nil, fmt.Errorf("%s: removing merged configuration requires a CLI sync", rel)
			}
			desired[rel] = nil
			inputs = append(inputs, rel)
		}
	}
	if fileExists(adapters.AgnosticEntryPointPath) {
		outputs = append(outputs, adapters.AgnosticEntryPointPath)
	}
	outputs = finalizeLedger(outputs)
	if resolveGitignore(cfg, "") {
		block, err := syncManagedBlock(".", cfg, bundle, cfg.Targets, outputs)
		if err != nil {
			return nil, err
		}
		ignore := gitignoreRel(cfg)
		existing, err := snapshotEditFile(ignore)
		if err != nil {
			return nil, err
		}
		desired[ignore] = []byte(replaceManagedBlock(stripLooseFixedDuplicates(string(existing.data)), block))
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
	baselineSums := specSums(cfg, bundle, cfg.Targets)
	next := previous
	next.Version = syncStateVersion
	next.FilesChanged = 0
	next.SyncedAt = time.Now().UTC()
	next.Outputs = outputs
	next.OutputSums = ledgerSums(outputs, sums, previous.OutputSums)
	next.Merged = ledgerMerged(outputs, merged, sums, previous, nil)
	next.SpecSums, next.SpecTargets = baselineSums.sums, baselineSums.targets
	next.SpecFileSums = syncedSpecFileSums(".", previous.SpecFileSums, bundle, cfg.Targets, cfg)
	next.Orphans, next.Unledgered = nil, nil
	next.WarningsDigest = adapters.CapabilityWarningsDigest()
	next.NotesDigest = adapters.CoverageNotesDigest()
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
	desired[stateFilePath(".")] = state
	for path := range desired {
		inputs = append(inputs, path)
	}
	plan.inputs, err = snapshotBuiltinProject(root, inputs)
	if err != nil {
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
		snap := plan.inputs.files[path]
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
