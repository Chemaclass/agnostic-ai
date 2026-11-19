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

// builtinPlanner carries one preview's state through its steps. Each step
// runs in order: later steps read what earlier ones recorded, and the
// adapter note buffers they reset and fill are shared process state.
type builtinPlanner struct {
	root         string
	change       projectChange
	where        editAudience
	guard        *gitexclude.Guard
	initial      builtinProjectSnapshot
	before       builtinProjectSnapshot
	previous     syncStateFile
	inputs       []string
	cfg          *config.Config
	bundle       spec.Bundle
	baseline     []driftReport
	configPath   string
	edited       []byte
	planned      map[string][]byte
	reports      []driftReport
	outputBefore builtinProjectSnapshot
	plan         *builtinChangePlan
	desired      map[string][]byte
	sums         map[string]string
	modes        map[string]os.FileMode
	merged       map[string]mergedOutput
	outputs      []string
}

func planProjectChangeLocked(root string, change projectChange, where editAudience, guard *gitexclude.Guard) (*builtinChangePlan, error) {
	defer adapters.SetAsideNotes()()
	p := &builtinPlanner{root: root, change: change, where: where, guard: guard}
	for _, step := range []func() error{
		p.loadBaseline,
		p.editSource,
		p.renderOutputs,
		p.collectOutputs,
		p.planIgnores,
		p.planState,
		p.recordChanges,
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}
	return p.plan, nil
}

func (p *builtinPlanner) loadBaseline() error {
	req := p.change.builtin
	initial, err := snapshotBuiltinProject(p.root, []string{config.ConfigFileName, config.LegacyConfigFileName, config.LocalOverrideFileName, stateFilePath(".")})
	if err != nil {
		return err
	}
	p.initial = initial
	lockPath := filepath.Join(p.root, ".agnostic-ai", projectLockName)
	if err := builtinSafeParents(p.root, lockPath); err != nil {
		return err
	}
	if _, err := builtinLockMissing(lockPath); err != nil {
		return err
	}
	cfg, err := config.Load(".")
	if err != nil {
		return err
	}
	for _, target := range cfg.Targets {
		if !slices.Contains(adapters.Names(), target) {
			return fmt.Errorf("%s: web changes require built-in adapters; use the CLI to change this project", target)
		}
	}
	if err := validateBuiltinConfigPaths(p.root, cfg); err != nil {
		return err
	}
	if cfg.Sync.SharedSkills {
		return fmt.Errorf("sync.shared-skills: web changes do not replace linked skill folders; use the CLI to change built-ins")
	}
	if p.change.setting == nil && !req.reset && slices.Contains(cfg.Builtins, req.name) == req.enabled {
		return fmt.Errorf("%s already has the requested value; refresh the overview", req.name)
	}
	if p.previous, err = readStateFileStrict("."); err != nil {
		return err
	}
	if p.previous.Version == 0 && initial.files[filepath.ToSlash(stateFilePath("."))].info != nil {
		return fmt.Errorf("run agnostic-ai sync before previewing a web change")
	}
	return p.snapshotInputs(cfg)
}

func (p *builtinPlanner) snapshotInputs(cfg *config.Config) error {
	inputs, err := builtinProjectInputs(cfg)
	if err != nil {
		return err
	}
	inputRoots := slices.Clone(inputs)
	inputs = append(inputs, p.previous.Outputs...)
	inputs = append(inputs, gitignoreRel(cfg), worktreeIncludeFile, stateFilePath("."))
	p.inputs = inputs
	if p.before, err = snapshotBuiltinProject(p.root, inputs); err != nil {
		return err
	}
	if err := p.initial.check(p.root); err != nil {
		return err
	}
	freshInputs, err := builtinProjectInputs(cfg)
	if err != nil {
		return err
	}
	if !slices.Equal(inputRoots, freshInputs) {
		return fmt.Errorf("source references changed while preparing preview; preview again")
	}
	if p.cfg, p.bundle, err = loadProject("."); err != nil {
		return err
	}
	if p.baseline, err = collectLoadedDrift(p.cfg, p.bundle, p.cfg.Targets, nil, ""); err != nil {
		return err
	}
	return validateBuiltinOutputs(p.root, p.baseline, p.previous)
}

func (p *builtinPlanner) editSource() error {
	req := p.change.builtin
	var file string
	var edit func([]byte) ([]byte, error)
	var err error
	if p.change.setting != nil {
		file, edit, err = settingSourceEdit(p.root, p.where, *p.change.setting)
	} else {
		file, edit, err = (&editHandler{root: p.root}).builtinsEdit(p.where, req)
	}
	if err != nil {
		return err
	}
	if p.configPath, err = builtinRelativePath(p.root, file); err != nil {
		return err
	}
	original, err := snapshotEditFile(filepath.Join(p.root, p.configPath))
	if err != nil {
		return err
	}
	edited, err := edit(original.data)
	var stale staleFieldError
	if errors.As(err, &stale) {
		return builtinChanged(fmt.Errorf("%s: %w", p.configPath, err))
	}
	if err != nil {
		return fmt.Errorf("%s: %w", p.configPath, err)
	}
	if len(edited) == 0 {
		edited = nil
	}
	if p.change.setting != nil && (edited == nil) == (original.info == nil) && bytes.Equal(edited, original.data) {
		return fmt.Errorf("%s already has the requested value; refresh the overview", p.change.setting.addr.label())
	}
	p.edited = edited
	p.cfg.Builtins = slices.Clone(p.cfg.Builtins)
	switch {
	case p.change.setting != nil:
	case req.reset:
		if p.cfg.Builtins, err = config.TeamBuiltins(p.root); err != nil {
			return err
		}
	case req.enabled:
		p.cfg.Builtins = append(p.cfg.Builtins, req.name)
	default:
		p.cfg.Builtins = slices.DeleteFunc(p.cfg.Builtins, func(current string) bool { return current == req.name })
	}
	return nil
}

func (p *builtinPlanner) renderOutputs() error {
	cfg := p.cfg
	layers, err := resolveLayers(".", cfg)
	if err != nil {
		return err
	}
	p.planned = map[string][]byte{}
	if p.change.setting != nil {
		p.planned[filepath.Clean(filepath.FromSlash(p.configPath))] = p.edited
		layers = plannedSourceLayers(layers, p.planned)
	}
	if p.bundle, err = spec.LoadLayered(layers); err != nil {
		return err
	}
	p.bundle.ApplyModelTiers(cfg.Models)
	scope := loadedProjectCheckScope(cfg, p.bundle)
	scope.bundle.Hooks = slices.DeleteFunc(slices.Clone(scope.bundle.Hooks), func(hook spec.Entry) bool {
		return hook.Layer == layerNameBuiltin && !slices.ContainsFunc(scope.hookTargets, hook.EmitsTo)
	})
	if issues := scopeValidationIssues(scope); len(issues) > 0 {
		return fmt.Errorf("%s: %s", issues[0].Path, issues[0].Message)
	}
	adapters.ResetCapabilityWarnings()
	adapters.ResetCoverageNotes()
	if p.reports, err = collectLoadedDrift(cfg, p.bundle, cfg.Targets, nil, ""); err != nil {
		return err
	}
	if err := validateBuiltinMemory(p.root, cfg, p.reports); err != nil {
		return err
	}
	if p.outputBefore, err = snapshotBuiltinProject(p.root, driftGeneratedPaths(p.reports)); err != nil {
		return err
	}
	if err := validateBuiltinOutputs(p.root, p.reports, p.previous); err != nil {
		return err
	}
	if slices.Contains(cfg.Targets, "codex") {
		hooksPath := codex.HooksFilePath(cfg)
		for _, report := range p.reports {
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
		return err
	}
	p.startView()
	return nil
}

func (p *builtinPlanner) startView() {
	req, setting := p.change.builtin, p.change.setting
	p.plan = &builtinChangePlan{audience: p.where, change: p.change, excludeBase: map[string][]byte{}, view: builtinChangeView{SchemaVersion: "builtin-change/v1", Status: "proposed", Name: req.name, Enabled: req.enabled, Reset: req.reset, Audience: string(p.where), Scope: p.where.legacyScope(), Config: p.configPath, Targets: slices.Clone(p.cfg.Targets), Changes: []builtinFileChange{}, Findings: []string{}}}
	if setting != nil {
		p.plan.view.Name, p.plan.view.Key, p.plan.view.Target, p.plan.view.Title = setting.addr.key, setting.addr.key, setting.addr.target, setting.title(p.where)
	}
	if slices.ContainsFunc(p.baseline, func(r driftReport) bool { return r.hasDrift() }) {
		p.plan.view.Findings = append(p.plan.view.Findings, "This preview also applies pending source changes.")
	}
	for _, drop := range append(adapters.PendingCapabilityWarnings(), adapters.PendingCoverageNotes()...) {
		p.plan.view.Findings = append(p.plan.view.Findings, preview.RedactText(drop.Target+": "+drop.Message))
	}
}

func (p *builtinPlanner) collectOutputs() error {
	p.desired = map[string][]byte{p.configPath: p.edited}
	p.sums = map[string]string{}
	p.modes = map[string]os.FileMode{}
	p.merged = map[string]mergedOutput{}
	for _, report := range p.reports {
		for _, files := range [][]adapters.CapturedFile{report.Current, report.Missing, report.Stale, report.Edited} {
			for _, file := range files {
				if err := p.addRendered(file); err != nil {
					return err
				}
			}
		}
		for _, path := range report.Leftover {
			if err := p.addLeftover(path); err != nil {
				return err
			}
		}
	}
	if fileExists(adapters.AgnosticEntryPointPath) {
		p.outputs = append(p.outputs, adapters.AgnosticEntryPointPath)
	}
	p.outputs = finalizeLedger(p.outputs)
	return nil
}

func (p *builtinPlanner) addRendered(file adapters.CapturedFile) error {
	path, err := builtinRelativePath(p.root, file.Path)
	if err != nil {
		return err
	}
	if old, exists := p.desired[path]; exists && !bytes.Equal(old, []byte(file.Content)) {
		return fmt.Errorf("%s: conflicting planned output", path)
	}
	p.desired[path] = []byte(file.Content)
	if file.Mode != 0 {
		if old := p.modes[path]; old != 0 && old != file.Mode {
			return fmt.Errorf("%s: conflicting planned permissions", path)
		}
		p.modes[path] = file.Mode
	}
	p.inputs = append(p.inputs, path)
	sum := adapters.ContentSum(file.Content)
	p.sums[file.Path] = sum
	p.outputs = append(p.outputs, file.Path)
	snap, err := snapshotEditFile(filepath.Join(p.root, path))
	if err != nil {
		return err
	}
	action := "skip"
	if snap.info == nil {
		action = "create"
	} else if !bytes.Equal(snap.data, []byte(file.Content)) {
		action = "update"
	}
	recordMergedWrites([]adapters.WrittenFile{{Path: file.Path, Action: action, Sum: sum, Merged: file.Merged, Keys: file.Keys, Released: file.Released}}, p.merged)
	return nil
}

func (p *builtinPlanner) addLeftover(path string) error {
	rel, err := builtinRelativePath(p.root, path)
	if err != nil {
		return err
	}
	current, err := snapshotEditFile(filepath.Join(p.root, rel))
	if err != nil {
		return err
	}
	if current.info == nil || p.previous.OutputSums[path] != adapters.ContentSum(string(current.data)) {
		return fmt.Errorf("%s: sync cannot prove ownership; use agnostic-ai doctor", rel)
	}
	if ownership, isMerged := p.previous.Merged[path]; isMerged {
		data, write, err := captureBuiltinMergedRelease(path, ownership)
		if err != nil {
			return err
		}
		if write {
			p.desired[rel] = data
		}
	} else {
		p.desired[rel] = nil
	}
	p.inputs = append(p.inputs, rel)
	return nil
}

func (p *builtinPlanner) planIgnores() error {
	cfg := p.cfg
	rootIgnore, err := snapshotEditFile(".gitignore")
	if err != nil {
		return err
	}
	p.desired[".gitignore"] = []byte(withRuntimeIgnores(string(rootIgnore.data)))
	if !resolveGitignore(cfg, "") {
		return nil
	}
	team := cfg.Builtins
	if p.configPath == config.LocalOverrideFileName {
		if team, err = config.TeamBuiltins("."); err != nil {
			return err
		}
	}
	prev, err := previousPersonalLocked(p.root, p.guard)
	if err != nil {
		return err
	}
	block, personal, err := splitManagedBlock(".", cfg, newTargetRenders(cfg, p.bundle), team, prev, cfg.Targets, p.outputs, ledgerMissing("."))
	if err != nil {
		return err
	}
	if err := planPersonalExcludes(p.root, personal, withLeftoverBackups(nil, block, syncBackups(p.previous.Backups, nil)), p.desired, p.plan.excludeBase); err != nil {
		return err
	}
	ignore, err := builtinRelativePath(p.root, gitignoreRel(cfg))
	if err != nil {
		return err
	}
	existing, err := snapshotEditFile(ignore)
	if err != nil {
		return err
	}
	base := existing.data
	if planned, ok := p.desired[ignore]; ok {
		base = planned
	}
	p.desired[ignore] = []byte(replaceManagedBlock(stripLooseFixedDuplicates(string(base)), block))
	existing, err = snapshotEditFile(worktreeIncludeFile)
	if err != nil {
		return err
	}
	rendered := renderWorktreeInclude(string(existing.data), cfg, block)
	if strings.TrimSpace(rendered) == "" && existing.info != nil {
		p.desired[worktreeIncludeFile] = nil
	} else if rendered != "" {
		p.desired[worktreeIncludeFile] = []byte(rendered)
	}
	return nil
}

func (p *builtinPlanner) planState() error {
	cfg, previous := p.cfg, p.previous
	excludePath, exclusion, err := planBuiltinMemoryExclusion(p.root, cfg, p.desired, p.plan.excludeBase)
	if err != nil {
		return err
	}
	baselineSums := specSums(cfg, p.bundle, cfg.Targets)
	next := previous
	next.Version = syncStateVersion
	next.FilesChanged = 0
	next.SyncedAt = time.Now().UTC()
	next.Outputs = p.outputs
	next.OutputSums = ledgerSums(p.outputs, p.sums, previous.OutputSums)
	next.Merged = ledgerMerged(p.outputs, p.merged, p.sums, previous, nil)
	next.SpecSums, next.SpecTargets = baselineSums.sums, baselineSums.targets
	next.SpecFileSums = syncedSpecFileSumsFrom(".", previous.SpecFileSums, p.bundle, cfg.Targets, cfg, plannedRead(p.planned))
	next.Orphans, next.Unledgered = nil, nil
	adapters.OrderBufferedDropsByTarget(cfg.Targets)
	next.WarningsDigest = adapters.CapabilityWarningsDigest()
	next.NotesDigest = adapters.CoverageNotesDigest()
	next.WarningRecords = dropFingerprints(adapters.PendingCapabilityWarnings())
	next.NoteRecords = dropFingerprints(adapters.PendingCoverageNotes())
	for path, data := range p.desired {
		snap, err := snapshotEditFile(filepath.Join(p.root, path))
		if err != nil {
			return err
		}
		if data == nil && snap.info != nil || data != nil && (snap.info == nil || !bytes.Equal(snap.data, data)) {
			next.FilesChanged++
		}
	}
	state, err := json.Marshal(next)
	if err != nil {
		return fmt.Errorf("%s: %w", stateFilePath("."), err)
	}
	p.desired[filepath.ToSlash(stateFilePath("."))] = state
	if excludePath != "" {
		p.desired[excludePath] = exclusion
	}
	return nil
}

func (p *builtinPlanner) recordChanges() error {
	inputs := p.inputs
	for path := range p.desired {
		inputs = append(inputs, path)
	}
	var err error
	if p.plan.inputs, err = snapshotBuiltinProject(p.root, inputs); err != nil {
		return err
	}
	for _, snapshot := range []builtinProjectSnapshot{p.outputBefore, p.before, p.initial} {
		if err := snapshot.check(p.root); err != nil {
			return err
		}
	}
	for _, path := range slices.Sorted(maps.Keys(p.desired)) {
		data, mode := p.desired[path], p.modes[path]
		snap, recorded := p.plan.inputs.files[path]
		if !recorded {
			return fmt.Errorf("%s: preview did not record the original file state", path)
		}
		if data == nil && snap.info == nil || data != nil && snap.info != nil && bytes.Equal(snap.data, data) && (mode == 0 || snap.info.Mode().Perm() == mode) {
			continue
		}
		action := "update"
		if data == nil {
			action = "delete"
		} else if snap.info == nil {
			action = "create"
		}
		change := builtinFileChange{Path: path, Action: action, before: snap, after: data, remove: data == nil, mode: mode}
		change.Diff = preview.RedactText(previewDiff(path, path+" (before)", path+" (proposed)", string(snap.data), string(data), snap.info == nil, len(snap.data)+len(data)+1))
		if snap.info != nil && mode != 0 && snap.info.Mode().Perm() != mode {
			change.Diff += fmt.Sprintf("\nPermissions: %04o -> %04o\n", snap.info.Mode().Perm(), mode)
		}
		p.plan.changes = append(p.plan.changes, change)
		p.plan.view.Changes = append(p.plan.view.Changes, change)
	}
	return nil
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
