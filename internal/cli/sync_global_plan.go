package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/augment"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codebuddy"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func buildGlobalWrites(home, source string, targets []string, intro []byte, b spec.Bundle, old globalState, agentErr func(string, error) error, warn io.Writer, onUnsupported string) ([]globalWrite, globalState, error) {
	return buildGlobalWritesForPlatform(home, source, targets, intro, b, old, agentErr, warn, onUnsupported, runtime.GOOS)
}
func buildGlobalWritesForPlatform(home, source string, targets []string, intro []byte, b spec.Bundle, old globalState, agentErr func(string, error) error, warn io.Writer, onUnsupported, platform string) ([]globalWrite, globalState, error) {
	for _, target := range targets {
		if err := globalTargets[target].instructionsRootError(platform); err != nil {
			return nil, globalState{}, fmt.Errorf("%s: %w", target, err)
		}
		if validate := globalTargets[target].mcp.validate; validate != nil {
			if err := validate(b.For(target).MCPs); err != nil {
				return nil, globalState{}, err
			}
		}
	}
	p := newGlobalPlan(home, source, platform, targets, intro, b, old, agentErr, warn, onUnsupported)
	if err := p.loadPlanApprovals(); err != nil {
		return nil, globalState{}, err
	}
	p.carryOverState()
	p.sweepOwnedSurfaces()
	if err := p.eachTarget(p.planAgents, p.planInstructions, p.planRules, p.planSkills, p.planHookFiles, p.planJSONHooks); err != nil {
		return nil, p.next, err
	}
	// User settings and MCP servers merge last: a file may also hold
	// hooks or agent efforts this run changed, so each edit starts from
	// that planned content.
	if slices.Contains(targets, "grok") {
		g := globalTargets["grok"]
		if err := p.snapshotTOML("grok", []string{g.path(home, g.settings.path), g.mcpPath(home), old.SettingsPaths["grok"], old.MCPPaths["grok"]}); err != nil {
			return nil, p.next, err
		}
	}
	if err := p.eachTarget(p.snapshotTargetTOML, p.planSettings, p.planKiroReview, p.planTOMLArrays, p.planMCP); err != nil {
		return nil, p.next, err
	}
	p.finalize()
	return p.writes, p.next, nil
}

// globalPlan is one run of the global planner: the writes it plans and
// the ownership state it records for the next run.
type globalPlan struct {
	home, source, platform string
	targets                []string
	bundle                 spec.Bundle
	old, next              globalState
	writes                 []globalWrite
	seen                   map[string]int
	// emptied holds hooks files the hooks merge left with nothing in
	// them, which a settings edit must start from instead of the disk.
	emptied map[string]bool
	// current is the target whose surfaces the planner is placing.
	current string
	// initialTOML is each TOML file as it was before any merge edited it.
	initialTOML   map[string]*diskSnapshot
	warn          io.Writer
	onUnsupported string
	agentErr      func(string, error) error
	// approvals is the home approvals block, which picks native reviewers.
	approvals config.ApprovalsConfig
	// body is the instructions text; managed wraps it in the global block.
	body, managed string
}

func newGlobalPlan(home, source, platform string, targets []string, intro []byte, b spec.Bundle, old globalState, agentErr func(string, error) error, warn io.Writer, onUnsupported string) *globalPlan {
	next := globalState{TOMLArrays: maps.Clone(old.TOMLArrays), HookPaths: maps.Clone(old.HookPaths), Version: globalStateVersion, Files: append([]string(nil), old.Files...), Hooks: map[string]map[string][]any{}, Agents: map[string][]string{}, Skills: map[string][]string{}, HookFiles: map[string][]string{}, AgentEfforts: map[string]map[string]string{}, Settings: map[string]map[string]any{}, PermissionRules: map[string]map[string][]string{}, SettingsOriginals: map[string]map[string]globalSettingOriginal{}, SettingsParents: map[string][]string{}, MCP: map[string]map[string]any{}, SettingsPaths: map[string]string{}, MCPPaths: map[string]string{}, ReviewPolicies: map[string]globalReviewPolicy{}, Created: slices.Clone(old.Created)}
	if next.TOMLArrays == nil {
		next.TOMLArrays = map[string]map[string]tomlArrayLayout{}
	}
	if next.HookPaths == nil {
		next.HookPaths = map[string]string{}
	}
	body := strings.TrimSpace(string(intro))
	return &globalPlan{
		home: home, source: source, platform: platform, targets: targets, bundle: b, old: old, next: next,
		seen: map[string]int{}, emptied: map[string]bool{}, initialTOML: map[string]*diskSnapshot{},
		warn: warn, onUnsupported: onUnsupported, agentErr: agentErr,
		body: body, managed: globalStart + "\n" + body + "\n" + globalEnd,
	}
}

// eachTarget runs every step for one target before the next target.
func (p *globalPlan) eachTarget(steps ...func(target string, g globalTarget) error) error {
	for _, target := range p.targets {
		p.current = target
		g := globalTargets[target]
		for _, step := range steps {
			if err := step(target, g); err != nil {
				return err
			}
		}
	}
	return nil
}

// keepUnselected copies into dst what src records for each target left
// out of this run.
func keepUnselected[V any](dst, src map[string]V, targets []string, clone func(V) V) {
	for target, value := range src {
		if !slices.Contains(targets, target) {
			dst[target] = clone(value)
		}
	}
}

func sharedValue[V any](v V) V { return v }

func copyPaths(paths []string) []string { return append([]string(nil), paths...) }

// carryOverState keeps what the last run recorded for targets this run
// leaves out.
func (p *globalPlan) carryOverState() {
	old, next, targets := p.old, &p.next, p.targets
	keepUnselected(next.Agents, old.Agents, targets, copyPaths)
	keepUnselected(next.Skills, old.Skills, targets, copyPaths)
	keepUnselected(next.HookFiles, old.HookFiles, targets, slices.Clone[[]string])
	keepUnselected(next.AgentEfforts, old.AgentEfforts, targets, sharedValue)
	keepUnselected(next.Settings, old.Settings, targets, sharedValue)
	keepUnselected(next.SettingsParents, old.SettingsParents, targets, slices.Clone[[]string])
	keepUnselected(next.PermissionRules, old.PermissionRules, targets, sharedValue)
	keepUnselected(next.SettingsOriginals, old.SettingsOriginals, targets, sharedValue)
	keepUnselected(next.MCP, old.MCP, targets, sharedValue)
	keepUnselected(next.SettingsPaths, old.SettingsPaths, targets, sharedValue)
	keepUnselected(next.MCPPaths, old.MCPPaths, targets, sharedValue)
	keepUnselected(next.ReviewPolicies, old.ReviewPolicies, targets, sharedValue)
	for target, hooks := range old.Hooks {
		next.Hooks[target] = cloneHookState(hooks)
	}
	for target, created := range old.HookContainers {
		if next.HookContainers == nil {
			next.HookContainers = map[string][]string{}
		}
		next.HookContainers[target] = slices.Clone(created)
	}
}

// sweepOwnedSurfaces drops every surface the synced targets own before
// emitting any of them, so a file the sources no longer produce is swept
// rather than kept alive by its own prior record. Targets left out of
// this run keep theirs. This runs before the emit pass because several
// targets share one skills directory.
func (p *globalPlan) sweepOwnedSurfaces() {
	old, next := p.old, &p.next
	for _, target := range p.targets {
		g := globalTargets[target]
		next.Files = removePaths(next.Files, old.Agents[target])
		next.Files = removePaths(next.Files, old.Skills[target])
		next.Files = removePaths(next.Files, old.HookFiles[target])
		for _, tree := range g.trees(p.home) {
			// A state without per-target skill records predates them,
			// so its skills tree is swept whole, as before.
			if old.Skills != nil && g.skills != "" && tree == g.path(p.home, g.skills) {
				continue
			}
			next.Files = removePathPrefix(next.Files, tree+string(filepath.Separator))
		}
		next.Files = removePaths(next.Files, g.filesForOS(p.home, p.platform))
	}
}

// guardRemoval records a file this run removes as the plan read it, so a
// write another program made since stops the removal.
func (p *globalPlan) guardRemoval(path string, snap *diskSnapshot) {
	if p.next.removalGuards == nil {
		p.next.removalGuards = map[string]*diskSnapshot{}
	}
	p.next.removalGuards[path] = snap
}

// place plans data at path for the current target. It reports false when
// another target already placed the same content there.
func (p *globalPlan) place(path string, data []byte, mode fs.FileMode) (bool, error) {
	if i, ok := p.seen[path]; ok {
		if !bytes.Equal(p.writes[i].data, data) {
			return false, fmt.Errorf("%s: two global targets emit different content to one path", path)
		}
		p.writes[i].addTarget(p.current)
		return false, nil
	}
	p.seen[path] = len(p.writes)
	p.writes = append(p.writes, globalWrite{path: path, data: data, mode: mode, targets: []string{p.current}})
	return true, nil
}

// add places a file sync owns whole.
func (p *globalPlan) add(path string, data []byte, mode fs.FileMode) error {
	placed, err := p.place(path, data, mode)
	if placed {
		p.writes[len(p.writes)-1].owned = true
		p.next.Files = append(p.next.Files, path)
	}
	return err
}

// addInstructions places the managed block in an instructions file and
// returns the file as it read it.
func (p *globalPlan) addInstructions(path, managed string) (*diskSnapshot, error) {
	before, err := snapshotFile(path)
	if err != nil {
		return nil, err
	}
	merged, err := mergeGlobalBlockData(path, managed, before.data)
	if err != nil {
		return nil, err
	}
	if merged == "" {
		if !before.absent && slices.Contains(p.old.Files, path) {
			p.guardRemoval(path, before)
		}
		return before, nil
	}
	if err := p.add(path, []byte(merged), 0o644); err != nil {
		return nil, err
	}
	w := &p.writes[p.seen[path]]
	w.keepMode = true
	if w.planned == nil {
		w.planned = before
	}
	return before, nil
}

func (p *globalPlan) planAgents(target string, g globalTarget) error {
	if g.agents == "" {
		return nil
	}
	b, next := p.bundle, &p.next
	dir := g.agentsPath(p.home)
	if err := adapters.ReportAgentWidening(target, b.Agents, p.onUnsupported); err != nil {
		return err
	}
	files, renderErr := adapters.RenderAgents(target, b.Agents, dir)
	if renderErr != nil {
		if err := p.agentErr(target, renderErr); err != nil {
			return err
		}
		// Keep what the last successful sync placed.
		next.Agents[target] = append([]string(nil), p.old.Agents[target]...)
	}
	for _, file := range files {
		if err := p.add(file.Path, []byte(file.Content), 0o644); err != nil {
			return err
		}
		next.Agents[target] = append(next.Agents[target], file.Path)
	}
	if g.agentEfforts == "" {
		return nil
	}
	return p.planAgentEfforts(target, g, renderErr == nil)
}

func (p *globalPlan) planAgentEfforts(target string, g globalTarget, rendered bool) error {
	efforts := p.old.AgentEfforts[target]
	if rendered {
		var err error
		if efforts, err = adapters.AgentEffortLevels(target, p.bundle.Agents); err != nil {
			return err
		}
	}
	// The settings file is the user's own, so ownership is per
	// key and the file never enters Files or removal.
	path := g.path(p.home, g.agentEfforts)
	doc, dropsComments, err := mergeGlobalAgentEfforts(path, efforts, p.old.AgentEfforts[target])
	if err != nil {
		return err
	}
	if doc != nil {
		placed, err := p.place(path, doc, 0o644)
		if err != nil {
			return err
		}
		if placed {
			p.writes[len(p.writes)-1].dropsComments = dropsComments
		}
	}
	if len(efforts) > 0 {
		p.next.AgentEfforts[target] = efforts
	}
	return nil
}

func (p *globalPlan) planInstructions(_ string, g globalTarget) error {
	if g.instructions == "" {
		return nil
	}
	path := g.pathForOS(p.home, g.instructions, p.platform)
	legacy := g.legacyInstructionsPath(p.home, p.platform)
	legacyOwned := legacy != "" && slices.Contains(p.old.Files, legacy)
	aliased := legacyOwned && aliasedGlobalInstructions(legacy, path)
	var legacyBefore *diskSnapshot
	if legacyOwned && !aliased {
		var err error
		legacyBefore, err = p.addInstructions(legacy, globalStart+"\n\n"+globalEnd)
		if err != nil {
			return err
		}
	}
	before, err := p.addInstructions(path, p.managed)
	if err != nil {
		return err
	}
	// A missing legacy file may appear through an alias when the active file is created.
	if aliased || legacyBefore != nil && legacyBefore.absent {
		if i, written := p.seen[path]; written {
			p.writes[i].keepAliases = append(p.writes[i].keepAliases, legacy)
		} else if legacyBefore != nil {
			p.guardRemoval(legacy, legacyBefore)
		} else {
			p.guardRemoval(legacy, before)
		}
	}
	return nil
}

func (p *globalPlan) planRules(_ string, g globalTarget) error {
	if g.rules == "" {
		return nil
	}
	dir := g.path(p.home, g.rules)
	for _, rule := range p.bundle.Rules {
		out := filepath.Join(dir, rule.Name+".md")
		if err := p.add(out, []byte(strings.TrimSpace(rule.Body)+"\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func (p *globalPlan) planSkills(target string, g globalTarget) error {
	if g.skills == "" {
		return nil
	}
	b, home := p.bundle, p.home
	skills, err := adapters.PrepareSkillCapabilities(target, b.Skills, p.onUnsupported)
	if err != nil {
		return err
	}
	dir := g.path(home, g.skills)
	adapters.NoteDroppedSkillFields(target, b.Skills, sharedGlobalSkillsDir(home, dir))
	if err := adapters.NoteManualOnlySkillDrops(target, b.Skills, sharedGlobalSkillsDir(home, dir)); err != nil {
		return err
	}
	if err := adapters.ReportGlobalSkillSyntax(target, b.Skills, p.onUnsupported, home, dir); err != nil {
		return err
	}
	addSkill := func(path string, data []byte, mode fs.FileMode) error {
		if err := p.add(path, data, mode); err != nil {
			return err
		}
		p.next.Skills[target] = append(p.next.Skills[target], path)
		return nil
	}
	for _, skill := range skills {
		if !skill.EmitsTo(target) {
			continue
		}
		overlays, err := globalSkillOverlays(home, dir, skill)
		if err != nil {
			return err
		}
		if err := addGlobalSkill(filepath.Join(dir, skill.Name), skill, target, sharedGlobalSkillsDir(home, dir), overlays, addSkill); err != nil {
			return err
		}
	}
	return nil
}

func (p *globalPlan) planHookFiles(target string, g globalTarget) error {
	if g.hooksDir == "" {
		return nil
	}
	hooks := adapters.TargetHooks(target, p.bundle.HooksFor(target))
	adapters.NotePortableHookGaps(target, p.bundle.Hooks)
	addHookFile := func(path string, data []byte, mode fs.FileMode) error {
		if err := p.add(path, data, mode); err != nil {
			return err
		}
		p.next.HookFiles[target] = append(p.next.HookFiles[target], path)
		return nil
	}
	return addGlobalHookFiles(p.home, p.source, target, g, hooks, p.onUnsupported, addHookFile)
}

func (p *globalPlan) planJSONHooks(target string, g globalTarget) error {
	if g.hooks == "" {
		return nil
	}
	b, old, next := p.bundle, p.old, &p.next
	next.Hooks[target] = map[string][]any{}
	path := g.path(p.home, g.hooks)
	hooks := adapters.TargetHooks(target, b.HooksFor(target))
	if target == "codebuddy" {
		if err := codebuddy.ValidateUserHooks(hooks); err != nil {
			return err
		}
	}
	adapters.NotePortableHookGaps(target, b.Hooks)
	if g.hooksFormat == "augment" {
		augment.NoteUserHookGaps(hooks)
	}
	scriptsDir := filepath.Join(filepath.Dir(path), "hooks")
	if err := p.planHookScripts(target, g, hooks, scriptsDir); err != nil {
		return err
	}
	if err := adapters.ReportHookProjectRoot(target, hooks, p.onUnsupported, true); err != nil {
		return err
	}
	hookTarget := globalHookTarget{name: target, mode: g.hookTarget, args: g.hookArgs, foldArgs: g.hookFoldArgs, timeout: g.hookTimeout, specHooks: len(hooks), scriptsDir: scriptsDir, goos: p.platform}
	if g.bridge && p.body != "" {
		bridge, command, script, mode := globalContextBridge(filepath.Dir(path), p.body, g.bridgeKey)
		if err := p.add(bridge, []byte(script), mode); err != nil {
			return err
		}
		hooks = append(append([]spec.Entry{}, hooks...), spec.Entry{Meta: map[string]any{"event": g.bridgeEvent, "command": command}})
	}
	if strings.HasSuffix(g.hooksFormat, "-toml") {
		return nil
	}
	merge, err := mergeGlobalHooks(path, g.hooksFormat, hookTarget, hooks, old.Hooks[target], next.Hooks[target], old.HookContainers[target], old.legacyHookContainers, p.warn)
	if next.HookContainers == nil {
		next.HookContainers = map[string][]string{}
	}
	next.HookContainers[target] = merge.created
	if len(merge.created) == 0 {
		delete(next.HookContainers, target)
	}
	doc := merge.data
	if errors.Is(err, errGlobalFileUnchanged) {
		if slices.Contains(old.Files, path) {
			next.Files = append(next.Files, path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if doc == nil {
		p.emptied[path] = true
		if merge.read != nil && !merge.read.absent {
			p.guardRemoval(path, merge.read)
		}
		return nil
	}
	if target == "codex" {
		codex.NoteHookTrust(path, doc)
	}
	// Ownership of a hooks file is per entry, so it carries no sum.
	placed, err := p.place(path, doc, 0o644)
	if err != nil {
		return err
	}
	if placed {
		next.Files = append(next.Files, path)
	}
	// The write stops if the file changed after the merge read it.
	if w := &p.writes[p.seen[path]]; w.planned == nil {
		w.planned = merge.read
	}
	return nil
}

// planHookScripts places the portable wrapper and each neutral script the
// target's hooks run from scriptsDir.
func (p *globalPlan) planHookScripts(target string, g globalTarget, hooks []spec.Entry, scriptsDir string) error {
	if wrapper, ok := adapters.PortableHookWrapperScript(hooks, target, scriptsDir); ok {
		if err := p.add(wrapper.Path, wrapper.Body, wrapper.Mode); err != nil {
			return err
		}
	}
	for _, hook := range hooks {
		if target == "copilot" || strings.HasSuffix(g.hooksFormat, "-toml") {
			hook.Meta = adapters.ResolveMeta(hook.Meta, target)
		}
		if event, _ := hook.Meta["event"].(string); event == "" {
			continue
		}
		type sourceCommand struct {
			value   string
			literal bool
		}
		var commands []sourceCommand
		literal := len(stringSliceFromAny(hook.Meta["args"])) > 0
		for _, command := range globalHookCommands(hook.Meta["command"]) {
			commands = append(commands, sourceCommand{command, literal})
		}
		if target == "codex" || target == "copilot" || target == "vibe" || target == "kimi" {
			if windows, _ := hook.Meta["commandWindows"].(string); windows != "" {
				windowsLiteral := false
				if target == "vibe" || target == "kimi" {
					windowsLiteral = literal
				}
				commands = append(commands, sourceCommand{windows, windowsLiteral})
			}
		}
		for _, command := range commands {
			scripts, err := adapters.NeutralHookScriptsFor(hook, command.value, target, filepath.Join(p.source, "scripts"), scriptsDir, command.literal)
			if err != nil {
				return err
			}
			for _, script := range scripts {
				if err := p.add(script.Path, script.Body, script.Mode); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// snapshotTOML records each path as it is before any merge edits it.
func (p *globalPlan) snapshotTOML(target string, paths []string) error {
	g := globalTargets[target]
	old := p.old
	for _, path := range paths {
		if path == "" || p.initialTOML[path] != nil {
			continue
		}
		snapshot, err := snapshotFile(path)
		if err != nil {
			return err
		}
		p.initialTOML[path] = snapshot
		if target == "vibe" && (path == g.mcpPath(p.home) || path == old.MCPPaths[target] || path == g.path(p.home, g.settings.path) || path == old.SettingsPaths[target]) {
			if err := validateVibeMCPDocument(path, snapshot.data); err != nil {
				return err
			}
		}
	}
	return nil
}

// snapshotTargetTOML snapshots every TOML file a target with TOML arrays
// may edit, at its current and its recorded paths.
func (p *globalPlan) snapshotTargetTOML(target string, g globalTarget) error {
	if g.mcp.format != "toml-array" && !strings.HasSuffix(g.hooksFormat, "-toml") {
		return nil
	}
	paths := []string{p.old.SettingsPaths[target], p.old.MCPPaths[target], p.old.HookPaths[target]}
	if g.settings.path != "" {
		paths = append(paths, g.path(p.home, g.settings.path))
	}
	if g.mcp.path != "" {
		paths = append(paths, g.mcpPath(p.home))
	}
	if g.hooks != "" {
		paths = append(paths, g.path(p.home, g.hooks))
	}
	return p.snapshotTOML(target, paths)
}

// placeEdit plans a key-level edit of a user file another program may
// also write.
func (p *globalPlan) placeEdit(path string, m globalSettingsMerge, mode fs.FileMode) error {
	if m.data == nil && len(m.adopted) == 0 {
		return nil
	}
	next := &p.next
	i, planned := p.seen[path]
	if !planned {
		data := m.data
		if data == nil {
			var err error
			if data, err = os.ReadFile(path); err != nil {
				return fmt.Errorf("read %s: %w", path, err)
			}
		}
		p.seen[path] = len(p.writes)
		p.writes = append(p.writes, globalWrite{path: path, data: data, mode: mode})
		i = len(p.writes) - 1
	} else if m.data != nil {
		p.writes[i].data = m.data
	}
	w := &p.writes[i]
	if w.planned == nil {
		snapshot := p.initialTOML[path]
		if snapshot == nil {
			var err error
			snapshot, err = snapshotFile(path)
			if err != nil {
				return err
			}
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
	w.addTarget(p.current)
	w.changes = append(w.changes, m.changes...)
	w.adopted = append(w.adopted, m.adopted...)
	w.conflicts = append(w.conflicts, m.conflicts...)
	// A file a removal would sweep, such as a hooks file with no
	// hooks left, stays while it holds managed keys.
	if slices.Contains(p.old.Files, path) {
		next.Files = append(next.Files, path)
	}
	return nil
}

// baseFor returns the content a merge into path starts from: what this
// run planned there, or nil to read the disk.
func (p *globalPlan) baseFor(path string) []byte {
	if i, planned := p.seen[path]; planned {
		return p.writes[i].data
	}
	if p.emptied[path] {
		return []byte("{}\n")
	}
	if initial, ok := p.initialTOML[path]; ok {
		if initial.data == nil {
			return []byte{}
		}
		return initial.data
	}
	return nil
}

func (p *globalPlan) planSettings(target string, g globalTarget) error {
	b, old, next, home, warn := p.bundle, p.old, &p.next, p.home, p.warn
	want := globalSettingsFor(target, g, b.Settings)
	var wantRules map[string][]string
	if g.settings.permissionLists {
		var err error
		wantRules, err = globalPermissionRules(target, b.Settings, p.onUnsupported)
		if err != nil {
			return err
		}
	}
	if g.settings.path == "" {
		return nil
	}
	path := g.path(home, g.settings.path)
	previous := old.Settings[target]
	if g.settings.toolOwned && len(want) == 0 && len(previous) == 0 && old.SettingsPaths[target] == "" {
		if mode, _ := homeToolMode(p.approvals, target); mode != config.ApprovalsReview {
			return nil
		}
	}
	previousRules := old.PermissionRules[target]
	originals := old.SettingsOriginals[target]
	parents := jsonParents{created: old.SettingsParents[target]}
	// Keys recorded in a file the root no longer resolves to go
	// from that file, and the new one starts unowned.
	if moved := old.SettingsPaths[target]; moved != "" && moved != path {
		m, _, err := mergeGlobalPersonal(moved, g.settings.format, p.baseFor(moved), nil, previous, originals, parents, nil, target, warn)
		if err != nil {
			return err
		}
		if m, _, err = withGlobalPermissionRules(moved, m, p.baseFor(moved), nil, previousRules, parents); err != nil {
			return err
		}
		if err := p.placeEdit(moved, m, g.settings.fileMode()); err != nil {
			return err
		}
		previous = nil
		previousRules = nil
		originals = nil
		parents = jsonParents{}
	}
	want, previous, err := p.approvalsReviewSetting(target, path, g.settings.format, want, previous)
	if err != nil {
		return err
	}
	m, originals, err := mergeGlobalPersonal(path, g.settings.format, p.baseFor(path), want, previous, originals, parents, personalNativeKeys(target, g.settings, b.Settings), target, warn)
	if err != nil {
		return err
	}
	if originals != nil {
		next.SettingsOriginals[target] = originals
	}
	var rules map[string][]string
	if g.settings.permissionLists {
		if m, rules, err = withGlobalPermissionRules(path, m, p.baseFor(path), wantRules, previousRules, parents); err != nil {
			return err
		}
	}
	if len(rules) > 0 {
		next.PermissionRules[target] = rules
		next.SettingsPaths[target] = path
	}
	if len(m.owned) > 0 {
		next.Settings[target] = m.owned
		next.SettingsPaths[target] = path
	}
	if len(m.parents) > 0 {
		next.SettingsParents[target] = m.parents
	}
	mode := g.settings.fileMode()
	if g.mcp.private && path == g.mcpPath(home) {
		mode = 0o600
	}
	return p.placeEdit(path, m, mode)
}

func (p *globalPlan) planTOMLArrays(target string, g globalTarget) error {
	if g.mcp.format == "toml-array" {
		if err := p.planMCPArray(target, g); err != nil {
			return err
		}
	}
	if strings.HasSuffix(g.hooksFormat, "-toml") {
		return p.planHookArray(target, g)
	}
	return nil
}

func (p *globalPlan) planMCPArray(target string, g globalTarget) error {
	path := g.mcpPath(p.home)
	previous := p.old.MCP[target]
	if moved := p.old.MCPPaths[target]; moved != "" && moved != path {
		if _, err := p.planArray(target, moved, g.mcp.key, nil, previous); err != nil {
			return err
		}
		previous = nil
	}
	records, err := adapters.UserMCPArrayRecords(target, p.bundle.MCPs)
	if err != nil {
		return err
	}
	want, err := namedTOMLRecords(path, records)
	if err != nil {
		return err
	}
	m, err := p.planArray(target, path, g.mcp.key, want, previous)
	if err != nil {
		return err
	}
	if len(m.owned) > 0 {
		p.next.MCP[target] = m.owned
		p.next.MCPPaths[target] = path
	}
	return nil
}

func (p *globalPlan) planHookArray(target string, g globalTarget) error {
	old, next := p.old, &p.next
	path := g.path(p.home, g.hooks)
	previous, err := namedTOMLAnyRecords(path, old.Hooks[target]["records"])
	if target == "kimi" {
		previous, err = kimiHookPrevious(path, old.Hooks[target]["records"])
	}
	if err != nil {
		return err
	}
	if moved := old.HookPaths[target]; moved != "" && moved != path {
		if _, err := p.planArray(target, moved, "hooks", nil, previous); err != nil {
			return err
		}
		previous = nil
	}
	records, err := adapters.UserHookArrayRecordsForPlatform(target, p.bundle.HooksFor(target), filepath.Join(filepath.Dir(path), "hooks"), p.platform)
	if err != nil {
		return err
	}
	want, err := namedTOMLRecords(path, records)
	if target == "kimi" {
		want, err = kimiHookRecordsByID(records), nil
	}
	if err != nil {
		return err
	}
	m, err := p.planArray(target, path, "hooks", want, previous)
	if err != nil {
		return err
	}
	next.Hooks[target] = map[string][]any{}
	delete(next.HookPaths, target)
	for _, name := range slices.Sorted(maps.Keys(m.owned)) {
		next.Hooks[target]["records"] = append(next.Hooks[target]["records"], m.owned[name])
	}
	if len(m.owned) > 0 {
		next.HookPaths[target] = path
	}
	return nil
}

// planArray merges the records sync owns into the TOML array of tables
// at parent in path, keeping each record's recorded layout.
func (p *globalPlan) planArray(target, path, parent string, want map[string]map[string]any, previous map[string]any) (globalSettingsMerge, error) {
	next := &p.next
	var snapshot *diskSnapshot
	base := p.baseFor(path)
	if i, ok := p.seen[path]; ok {
		snapshot = p.writes[i].planned
	}
	if snapshot == nil {
		snapshot = p.initialTOML[path]
	}
	if snapshot == nil {
		var err error
		snapshot, err = snapshotFile(path)
		if err != nil {
			return globalSettingsMerge{}, err
		}
	}
	if base == nil {
		base = snapshot.data
	}
	key := path + "\x00" + parent
	unnamed := target == "kimi" && parent == "hooks"
	if unnamed {
		doc, err := decodeTOMLDocument(path, base)
		if err != nil {
			return globalSettingsMerge{}, err
		}
		records, err := nativeTOMLRecords(path, parent, doc[parent])
		if err != nil {
			return globalSettingsMerge{}, err
		}
		for id, desired := range want {
			for _, have := range records {
				if sameSetting(adapters.CanonicalUserArrayRecord(target, parent, have), adapters.CanonicalUserArrayRecord(target, parent, desired)) {
					want[id] = have
					break
				}
			}
		}
	}
	existing, _, err := scanTOMLArrayWithIdentity(path, base, parent, want, previous, unnamed)
	if err != nil {
		return globalSettingsMerge{}, err
	}
	if target == "vibe" && parent == "mcp_servers" {
		want, err = reconcileVibeMCPNames(path, want, existing, previous)
		if err != nil {
			return globalSettingsMerge{}, err
		}
	}
	for id, desired := range want {
		if have, ok := existing[id]; ok && sameSetting(adapters.CanonicalUserArrayRecord(target, parent, have.value), adapters.CanonicalUserArrayRecord(target, parent, desired)) {
			want[id] = have.value
		}
	}
	rebased, err := rebaseTOMLArrayLayouts(path, parent, snapshot.data, base, next.TOMLArrays[key], want, previous, unnamed)
	if err != nil {
		return globalSettingsMerge{}, err
	}
	m, layouts, err := mergeTOMLArrayRecords(path, base, parent, want, previous, rebased, unnamed)
	if err != nil {
		return m, err
	}
	if target == "vibe" && parent == "mcp_servers" {
		final := m.data
		if final == nil {
			final = base
		}
		if err := validateVibeMCPDocument(path, final); err != nil {
			return m, err
		}
	}
	if len(m.owned) > 0 && slices.Contains(p.old.Files, path) {
		next.Files = append(next.Files, path)
	}
	if len(layouts) > 0 {
		next.TOMLArrays[key] = layouts
	} else {
		delete(next.TOMLArrays, key)
	}
	if err := p.placeEdit(path, m, 0o600); err != nil {
		return m, err
	}
	if i, ok := p.seen[path]; ok {
		p.writes[i].planned = snapshot
	}
	return m, nil
}

func (p *globalPlan) planMCP(target string, g globalTarget) error {
	old := p.old
	if g.mcp.path == "" || g.mcp.format == "toml-array" || (g.mcp.retired != "" && len(old.MCP[target]) == 0) {
		return nil
	}
	path := g.mcpPath(p.home)
	previous := old.MCP[target]
	mcps := p.bundle.MCPs
	if g.mcp.retired != "" {
		mcps = nil
	}
	if moved := old.MCPPaths[target]; moved != "" && moved != path {
		m, err := mergeGlobalMCP(moved, g.mcp, p.baseFor(moved), target, nil, previous)
		if err != nil {
			return err
		}
		if err := p.placeEdit(moved, m, g.mcp.fileMode()); err != nil {
			return err
		}
		previous = nil
	}
	m, err := mergeGlobalMCP(path, g.mcp, p.baseFor(path), target, mcps, previous)
	if err != nil {
		return err
	}
	if len(m.owned) > 0 {
		p.next.MCP[target] = m.owned
		p.next.MCPPaths[target] = path
	}
	return p.placeEdit(path, m, g.mcp.fileMode())
}

// finalize drops user files left empty, then settles the owned file
// list, the created list, and the sums.
func (p *globalPlan) finalize() {
	old, next := p.old, &p.next
	// A user file sync created goes once nothing is left in it, rather
	// than staying behind as an empty document.
	for i := 0; i < len(p.writes); i++ {
		w := p.writes[i]
		if w.owned || !emptyUserDocument(w.data) || !slices.Contains(next.Created, w.path) {
			continue
		}
		p.writes = slices.Delete(p.writes, i, i+1)
		i--
		next.Files = removePaths(next.Files, []string{w.path})
		next.Created = removePaths(next.Created, []string{w.path})
		if w.planned != nil && !w.planned.absent {
			p.guardRemoval(w.path, w.planned)
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
	next.Created = slices.DeleteFunc(next.Created, func(path string) bool { return !slices.Contains(next.Files, path) })
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
	for _, w := range p.writes {
		delete(next.Sums, w.path)
		if w.owned {
			next.Sums[w.path] = globalSum(w.data)
		}
	}
}
