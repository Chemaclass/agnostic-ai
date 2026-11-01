package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/claude"
	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/preview"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// overviewChecksMu runs one overview's checks at a time. They silence
// and drop the adapters' buffered warnings and notes, which are package
// state shared by every capture.
var overviewChecksMu sync.Mutex

// Drift states, carried as a drift finding's message.
const (
	driftNotGenerated = "not generated"
	driftStale        = "stale"
)

// overviewChecks is what the overview's read-only checks found, and
// which targets' generated files the drift check compared with disk.
type overviewChecks struct {
	findings []overview.Finding
	compared []string
	skipped  []string
	// fieldNotes are the settings model and effort notes the drift
	// render raised.
	fieldNotes []overview.FieldNote
}

// settingsFieldNotes keeps the model and effort notes on settings specs
// from the notes a render buffered.
func settingsFieldNotes(notes []adapters.Note) []overview.FieldNote {
	var out []overview.FieldNote
	for _, n := range notes {
		if n.Shape == adapters.NoteField && n.Kind == spec.KindSettings && (n.Field == "model" || n.Field == "effort") {
			out = append(out, overview.FieldNote{Target: n.Target, Field: n.Field, Reason: n.Reason})
		}
	}
	return out
}

// runOverviewChecks runs the read-only checks the overview shows on the
// items they concern: every finding `lint` reports, drift between the
// files sync would write and the files on disk, and the doctor checks
// that only read a few local files. It writes nothing and reaches no
// network. root must be the working directory, because the checks
// resolve generated paths against it as every command does.
func runOverviewChecks(root string, cfg *config.Config, b spec.Bundle) overviewChecks {
	return runOverviewChecksWith(root, cfg, b, true)
}

// runOverviewChecksWith is runOverviewChecks that skips the drift check,
// and with it the rendering and reading of generated files, when
// compareInstalled is false.
func runOverviewChecksWith(root string, cfg *config.Config, b spec.Bundle, compareInstalled bool) overviewChecks {
	overviewChecksMu.Lock()
	defer overviewChecksMu.Unlock()
	// Captures buffer coverage notes and print capability warnings; a
	// read-only view reports neither and leaves no trace of them.
	defer adapters.SetAsideNotes()()
	// Every capture asks the sync ledger what the last sync wrote; read
	// it once for all of them.
	defer holdCaptureState(readStateFile("."))()
	var out overviewChecks
	out.findings = append(out.findings, overviewLintFindings(cfg, b)...)
	adapters.DrainNotes()
	if compareInstalled {
		drift, compared, skipped := overviewDriftFindings(cfg, b)
		out.fieldNotes = settingsFieldNotes(adapters.DrainNotes())
		out.findings = append(out.findings, drift...)
		out.compared, out.skipped = compared, skipped
	}
	out.findings = append(out.findings, overviewDoctorFindings(root, cfg, b)...)
	for i := range out.findings {
		out.findings[i].Message = servedFindingMessage(out.findings[i].Message)
	}
	setFindingOpenPaths(out.findings, root)
	return out
}

// setFindingOpenPaths gives each lint and doctor finding the absolute
// path of the file it names under base, the project root or global
// source. Drift findings name files the overview lists elsewhere.
func setFindingOpenPaths(findings []overview.Finding, base string) {
	for i := range findings {
		if findings[i].Source != overview.SourceDrift {
			findings[i].OpenPath = findingOpenPath(base, findings[i].Path)
		}
	}
}

// findingOpenPath is the absolute, slash-separated path of a regular file
// at path, resolved against base when relative, or "" when no such file
// exists under base. A built-in spec's file lives in a cache outside
// base, where an edit changes nothing, so it gets no path, as its source
// row gets none.
func findingOpenPath(base, path string) string {
	if path == "" {
		return ""
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return ""
	}
	full := filepath.FromSlash(path)
	if !filepath.IsAbs(full) {
		full = filepath.Join(absBase, full)
	}
	rel, err := filepath.Rel(absBase, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	if info, err := os.Stat(full); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return filepath.ToSlash(full)
}

// overviewLintFindings is what `lint` reports for the project, each with
// the cause and fix `explain` gives for its code.
func overviewLintFindings(cfg *config.Config, b spec.Bundle) []overview.Finding {
	return overviewScopeLintFindings(loadedProjectCheckScope(cfg, b), overviewConfigPath(), filepath.ToSlash)
}

// overviewScopeLintFindings is what `lint` reports for scope, each with
// the cause and fix `explain` gives for its code and its path shown through
// display. A lint run that stops early becomes one finding on
// configPath, so the overview shows the gap instead of hiding it.
func overviewScopeLintFindings(scope checkScope, configPath string, display func(string) string) []overview.Finding {
	findings, err := lintScopeFindings(scope)
	if err != nil {
		return []overview.Finding{{
			Source:      overview.SourceLint,
			Code:        overview.CodeLintIncomplete,
			Severity:    lintError.String(),
			Path:        display(configPath),
			Message:     servedFindingMessage("Lint stopped before it finished: " + err.Error()),
			Explanation: "The overview shows no lint findings until lint runs to the end.",
		}}
	}
	out := make([]overview.Finding, 0, len(findings))
	for _, f := range findings {
		finding := overview.Finding{
			Source:   overview.SourceLint,
			Code:     f.Code,
			Severity: f.Severity.String(),
			Message:  servedFindingMessage(f.Message),
			Path:     display(f.Path),
		}
		if code, ok := lintCodes[f.Code]; ok {
			finding.Explanation = code.Cause
			finding.Fix = servedFindingMessage(code.Fix)
		}
		out = append(out, finding)
	}
	return out
}

// overviewDriftFindings compares what sync would write for each
// configured target with the files on disk: the adapter's own output,
// captured in memory, and the entry points that carry the instructions
// and inlined rules, credited to each target that reads them. A missing
// file is "not generated" and a differing one is "stale". An external
// adapter is skipped, because rendering it runs a program outside
// agnostic-ai.
func overviewDriftFindings(cfg *config.Config, b spec.Bundle) (findings []overview.Finding, compared, skipped []string) {
	seen := map[string]bool{}
	add := func(target, path, state string) {
		path = filepath.ToSlash(path)
		if key := target + "\x00" + path; !seen[key] {
			seen[key] = true
			findings = append(findings, overview.Finding{Source: overview.SourceDrift, Target: target, Path: path, Message: state})
		}
	}
	for _, target := range cfg.Targets {
		adapter, ok := adapters.Get(target)
		if !ok {
			skipped = append(skipped, target)
			continue
		}
		files, err := captureEmit(adapter, b, cfg)
		if err != nil {
			skipped = append(skipped, target)
			continue
		}
		compared = append(compared, target)
		for _, f := range files {
			if state := onDiskState(f.Path, f.Content); state != "" {
				add(target, f.Path, state)
			}
		}
	}
	for _, f := range overviewEntryPoints(cfg, b) {
		state := onDiskState(f.Path, f.Content)
		if state == "" {
			continue
		}
		for _, reader := range f.Readers {
			if slices.Contains(compared, reader) {
				add(reader, f.Path, state)
			}
		}
	}
	return findings, compared, skipped
}

// overviewEntryPoints renders the entry points sync writes, from
// AGNOSTIC_AI.md or the template when it is missing, as `sync --check`
// does. A render that fails yields none; lint reports why.
func overviewEntryPoints(cfg *config.Config, b spec.Bundle) []entryPointFile {
	body := adapters.EntryPointBody()
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	switch {
	case err == nil:
		body = header.Strip(string(data))
	case !errors.Is(err, fs.ErrNotExist):
		return nil
	}
	files, err := renderEntryPointFiles(cfg, b, cfg.Targets, body)
	if err != nil {
		return nil
	}
	return slices.DeleteFunc(files, func(f entryPointFile) bool { return cfg.IsUnmanaged(f.Path) })
}

// onDiskState compares want with the file at path: "" when they match or
// the file cannot be read, else a drift state.
func onDiskState(path, want string) string {
	disk, err := os.ReadFile(path)
	switch {
	case err == nil && string(disk) == want:
		return ""
	case err == nil:
		return driftStale
	case notOnDisk(err):
		return driftNotGenerated
	}
	return ""
}

// overviewDoctorFindings runs the doctor checks that only read a few
// local files: a nested CLAUDE.md a rule already holds, a Claude hook
// spec an older import merged, a hook the project gives Codex that
// Codex has not trusted, and hook scripts that differ between tools.
// Checks that walk the tree, run git, or reach the network stay in
// `doctor`.
func overviewDoctorFindings(root string, cfg *config.Config, b spec.Bundle) []overview.Finding {
	var out []overview.Finding
	for _, c := range nestedClaudeCopies(cfg, b) {
		out = append(out, overview.Finding{
			Source:      overview.SourceDoctor,
			Code:        "nested-claude-copy",
			Severity:    lintError.String(),
			Message:     fmt.Sprintf("Nested %s repeats rule %s, which sync already writes for Claude Code", claudeMainFile, c.rule),
			Path:        filepath.ToSlash(c.path),
			Fix:         "agnostic-ai doctor --fix",
			Explanation: "Claude Code loads the same text twice, once from this file and once from the rule.",
			Section:     overview.SectionContext,
		})
	}
	if paths := mergedClaudeHookSpecs(cfg, b); len(paths) > 0 {
		settings := filepath.ToSlash(claude.SettingsFilePath(cfg))
		for _, p := range paths {
			out = append(out, overview.Finding{
				Source:      overview.SourceDoctor,
				Code:        "merged-claude-hooks",
				Severity:    lintWarn.String(),
				Message:     fmt.Sprintf("Hook spec merges commands that %s runs with different settings; delete the spec, run agnostic-ai sync, then agnostic-ai import claude", settings),
				Path:        filepath.ToSlash(p),
				Explanation: "Sync adds a second hook group for these commands, so Claude Code runs each of them twice.",
				Section:     overview.SectionLifecycle,
			})
		}
	}
	out = append(out, overviewCodexTrustFindings(cfg)...)
	out = append(out, overviewHookScriptFindings(root)...)
	return out
}

// overviewCodexTrustFindings reports the hooks in the project's Codex
// hooks file that Codex has not trusted or has disabled. Findings on the
// user's own hooks file are left out: they belong to the machine, not to
// the project.
func overviewCodexTrustFindings(cfg *config.Config) []overview.Finding {
	project := codex.HooksFilePath(cfg)
	projectAbs, err := filepath.Abs(project)
	if err != nil {
		return nil
	}
	var out []overview.Finding
	for _, f := range collectCodexHookTrust(cfg, cfg.Targets) {
		if path := filepath.Clean(f.Path); f.Problem != "" || (path != projectAbs && path != filepath.Clean(project)) {
			continue
		}
		action, severity := "review and trust", lintError.String()
		explanation := "Codex treats this hook as inactive until it is reviewed in /hooks."
		if f.Status == "disabled" {
			action, severity = "review and enable", lintWarn.String()
			explanation = "Codex skips this hook because it is disabled in /hooks."
		}
		out = append(out, overview.Finding{
			Source:      overview.SourceDoctor,
			Code:        "codex-hook-trust",
			Severity:    severity,
			Message:     fmt.Sprintf("Codex %s hook %s is %s; open /hooks in Codex to %s it", f.Event, strconv.Quote(f.Hook), f.Status, action),
			Path:        filepath.ToSlash(project),
			Explanation: explanation,
			Section:     overview.SectionLifecycle,
		})
	}
	return out
}

// overviewHookScriptFindings reports each hook script captured for two
// or more tools under .agnostic-ai/scripts with different bodies.
func overviewHookScriptFindings(root string) []overview.Finding {
	divergent, _, err := collectDivergentHookScripts(root)
	if err != nil {
		return nil
	}
	var out []overview.Finding
	for _, d := range divergent {
		path := d.variants[0].path
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
		out = append(out, overview.Finding{
			Source:      overview.SourceDoctor,
			Code:        "divergent-hook-script",
			Severity:    lintError.String(),
			Message:     fmt.Sprintf("Hook script %s differs between %s; move the agreed body to %s/%s and delete the per-tool copies", d.Script, strings.Join(d.Tools, ","), agnosticScriptsDir, d.Script),
			Path:        filepath.ToSlash(path),
			Explanation: "The tools hold different bodies for one hook script, so the hook may not behave the same in every tool.",
			Section:     overview.SectionLifecycle,
		})
	}
	return out
}

// overviewInputs loads the project, runs the read-only checks, and
// builds the snapshot the overview names. root must be the working
// directory, because the checks compare generated paths relative to it;
// a server changes into the project once, before it serves, and passes
// ".".
func overviewInputs(root string) (overview.Inputs, error) {
	return overviewInputsWith(root, true)
}

// overviewInputsWith is overviewInputs that skips comparing generated
// files with disk when compareInstalled is false.
func overviewInputsWith(root string, compareInstalled bool) (overview.Inputs, error) {
	if err := requireOverviewRoot(root); err != nil {
		return overview.Inputs{}, err
	}
	cfg, b, err := loadProject(".")
	if err != nil {
		return overview.Inputs{}, err
	}
	checks := runOverviewChecksWith(".", cfg, b, compareInstalled)
	snapshot, err := overviewSnapshot(cfg, b, checks)
	if err != nil {
		return overview.Inputs{}, err
	}
	team, local := overviewBuiltins(".", false)
	return overview.Inputs{
		Cfg:            cfg,
		Bundle:         b,
		Snapshot:       snapshot,
		Instructions:   instructionDocuments([]instructionFile{{path: adapters.AgnosticEntryPointPath, layer: "project"}}, "."),
		Findings:       checks.findings,
		Compared:       checks.compared,
		FieldNotes:     checks.fieldNotes,
		MCPTools:       mcpprobe.LoadCache("."),
		RefreshCommand: "agnostic-ai mcp tools",
		BuiltinsTeam:   team,
		BuiltinsLocal:  local,
		Tools:          overviewTools(".", false),
		NativeMCPs:     projectNativeMCPServers(cfg),
	}, nil
}

// projectNativeMCPServers lists the MCP servers the project's targets
// hold in their own project and user files. Without a home it lists
// none.
func projectNativeMCPServers(cfg *config.Config) []overview.NativeMCP {
	home, err := globalUserHome()
	if err != nil {
		return nil
	}
	source, _ := globalSourceRoot()
	return nativeMCPServers(home, ".", source, cfg, false)
}

// instructionFile is one instructions file a configuration loads.
type instructionFile struct {
	path, layer string
}

// instructionDocuments describes each instructions file that exists, with
// its path relative to root and its word count without the generated
// header.
func instructionDocuments(sources []instructionFile, root string) []overview.Document {
	var docs []overview.Document
	for _, src := range sources {
		data, err := os.ReadFile(src.path)
		if err != nil {
			continue
		}
		abs, err := filepath.Abs(src.path)
		if err != nil {
			abs = src.path
		}
		docs = append(docs, overview.Document{
			Path:    globalOverviewPath(root, src.path),
			AbsPath: abs,
			Layer:   src.layer,
			Words:   len(strings.Fields(header.Strip(string(data)))),
		})
	}
	return docs
}

// requireOverviewRoot refuses a root other than the working directory.
func requireOverviewRoot(root string) error {
	if root == "" || filepath.Clean(root) == "." {
		return nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("overview: working directory: %w", err)
	}
	want, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("overview: %w", err)
	}
	have, err := os.Stat(wd)
	if err != nil {
		return fmt.Errorf("overview: %w", err)
	}
	if !os.SameFile(want, have) {
		return fmt.Errorf("overview: %s is not the working directory %s; the checks compare generated files relative to the working directory, so change into the project first", root, wd)
	}
	return nil
}

// overviewSnapshot names the project the inputs came from: its root,
// targets, config files, and specs, with a revision that changes when
// any of them does.
func overviewSnapshot(cfg *config.Config, b spec.Bundle, checks overviewChecks) (authority.Snapshot, error) {
	if _, err := os.Getwd(); err != nil {
		return authority.Snapshot{}, fmt.Errorf("overview: working directory: %w", err)
	}
	// The Authority view builds the snapshot, so both views report the
	// same revision for the same sources.
	snapshot := authority.NewSnapshot(cfg, b, authority.Options{Root: ".", ConfigFiles: overviewConfigFiles()})
	snapshot.Evidence.Installed = "not-checked"
	if len(checks.compared) > 0 {
		snapshot.Evidence.Installed = "checked"
	}
	snapshot.Evidence.Note = overviewEvidenceNote(checks)
	return snapshot, nil
}

// overviewConfigFiles lists the config files the project loads, relative
// to its root.
func overviewConfigFiles() []string {
	files := []string{}
	if path, _, err := config.ResolveConfigPath("."); err == nil {
		files = append(files, filepath.ToSlash(path))
	}
	if info, err := os.Stat(config.LocalOverrideFileName); err == nil && info.Mode().IsRegular() {
		files = append(files, config.LocalOverrideFileName)
	}
	return files
}

func overviewConfigPath() string {
	if path, _, err := config.ResolveConfigPath("."); err == nil {
		return filepath.ToSlash(path)
	}
	return config.ConfigFileName
}

func overviewEvidenceNote(checks overviewChecks) string {
	var parts []string
	if len(checks.compared) == 0 && len(checks.skipped) == 0 {
		parts = append(parts, "Generated files were not compared with disk.")
	}
	if len(checks.compared) > 0 {
		parts = append(parts, "Generated files were compared with disk for "+strings.Join(checks.compared, ", ")+".")
	}
	if len(checks.skipped) > 0 {
		parts = append(parts, "Not compared, because the adapter is external or failed to render: "+strings.Join(checks.skipped, ", ")+".")
	}
	return strings.Join(append(parts, "Runtime enforcement is not observed."), " ")
}

// newOverviewHandler serves the overview of the project in
// the working directory, built afresh on every request. root must be
// the working directory; see overviewInputs.
func newOverviewHandler(root string, compareInstalled bool) http.Handler {
	return overview.NewHandler(func() (overview.Inputs, error) {
		return overviewInputsWith(root, compareInstalled)
	})
}

func servedFindingMessage(message string) string { return preview.RedactText(message) }
