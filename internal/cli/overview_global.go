package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// newGlobalOverviewHandler serves the overview of the
// user-level configuration sync --global reads, built afresh on every
// request. Unlike the project handler it runs from any working
// directory: every path comes from $HOME and the global source root.
func newGlobalOverviewHandler() http.Handler {
	return overview.NewHandler(globalOverviewInputs)
}

// globalOverviewInputs loads the global and global-local layers as
// lint --global does, runs the read-only checks that apply to them, and
// builds the snapshot the overview names. The snapshot uses the options
// the served Authority view uses for --global, so both report one
// revision for the same sources.
func globalOverviewInputs() (overview.Inputs, error) {
	scope, err := loadCheckScope(true)
	if err != nil {
		return overview.Inputs{}, err
	}
	builtins, err := loadGlobalBuiltins(scope.source, nil)
	if err != nil {
		return overview.Inputs{}, err
	}
	cfg := &config.Config{Targets: scope.targets, Builtins: builtins, Models: scope.models, Requires: globalRequires(scope.source)}
	checks := runGlobalOverviewChecks(scope)
	src := &authoritySource{root: scope.source, global: true}
	opts := authority.Options{Root: scope.source, ConfigFiles: src.configFiles(), Global: true}
	snapshot := authority.NewSnapshot(cfg, scope.bundle, opts)
	snapshot.Evidence.Installed = authority.InstalledNotChecked
	if len(checks.compared) > 0 {
		snapshot.Evidence.Installed = "checked"
	}
	snapshot.Evidence.Note = globalOverviewEvidenceNote(checks)
	instructions := instructionDocuments([]instructionFile{
		{path: filepath.Join(scope.source, "AGNOSTIC_AI.md"), layer: "global"},
		{path: filepath.Join(scope.source, "local", "AGNOSTIC_AI.md"), layer: "global-local"},
	}, scope.source)
	return overview.Inputs{
		Cfg:            cfg,
		Bundle:         scope.bundle,
		Snapshot:       snapshot,
		Instructions:   instructions,
		Findings:       checks.findings,
		Compared:       checks.compared,
		MCPTools:       mcpprobe.LoadCache(scope.source),
		RefreshCommand: "agnostic-ai mcp tools --global",
	}, nil
}

// globalRequires is the requires value of the home config, where
// local/agnostic-ai.yaml replaces the shared one. A config that cannot
// be read sets none; lint --global reports it.
func globalRequires(source string) string {
	var requires string
	for _, path := range globalConfigPaths(source) {
		doc, err := readGlobalConfig(path)
		if err != nil {
			continue
		}
		if node, ok := doc["requires"]; ok {
			requires = ""
			_ = node.Decode(&requires)
		}
	}
	return requires
}

// runGlobalOverviewChecks runs the read-only checks the global overview
// reads: every finding lint --global reports, and drift between the
// files sync --global would write and the files on disk. The doctor
// checks read a project's generated files and do not run.
func runGlobalOverviewChecks(scope checkScope) overviewChecks {
	overviewChecksMu.Lock()
	defer overviewChecksMu.Unlock()
	// Planning the global writes prints capability warnings and buffers
	// coverage notes; a read-only view reports neither.
	defer adapters.SetAsideNotes()()
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	display := func(path string) string { return globalOverviewPath(scope.source, path) }
	var out overviewChecks
	out.findings = append(out.findings, overviewScopeLintFindings(scope, globalConfigPaths(scope.source)[0], display)...)
	drift, compared, skipped := globalOverviewDriftFindings(scope)
	out.findings = append(out.findings, drift...)
	out.compared, out.skipped = compared, skipped
	return out
}

// globalOverviewPath shows a path under the global source relative to
// it, as the Authority view lists the specs, and any other path, such
// as a generated file under $HOME, as it is.
func globalOverviewPath(source, path string) string {
	rel, err := filepath.Rel(source, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// globalOverviewDriftFindings compares the files sync --global would
// write for each target with the files on disk, as sync --global --check
// does, and credits each file to the targets whose surfaces it carries.
// A missing file is "not generated" and a differing one is "stale". The
// sync ledger is left out, because no tool reads it. A target global
// sync does not support, or whose configuration root cannot be
// resolved, is skipped, as a default sync skips it. Nothing is written:
// the writes are planned in memory and compared.
func globalOverviewDriftFindings(scope checkScope) (findings []overview.Finding, compared, skipped []string) {
	home, err := globalUserHome()
	if err != nil {
		return nil, nil, scope.targets
	}
	var targets []string
	for _, target := range scope.targets {
		if unsupportedGlobalTarget("--global", target) != nil || globalTargets[target].rootError(target) != nil {
			skipped = append(skipped, target)
			continue
		}
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, nil, skipped
	}
	writes, err := planGlobalWrites(home, scope, targets)
	if err != nil {
		return nil, nil, append(skipped, targets...)
	}
	seen := map[string]bool{}
	for _, w := range writes {
		state := onDiskState(w.path, string(w.data))
		if state == "" {
			continue
		}
		path := filepath.ToSlash(w.path)
		for _, target := range w.targets {
			if key := target + "\x00" + path; !seen[key] {
				seen[key] = true
				findings = append(findings, overview.Finding{Source: overview.SourceDrift, Target: target, Path: path, Message: state})
			}
		}
	}
	return findings, targets, skipped
}

// planGlobalWrites builds the files a default sync --global would write
// for targets, as runGlobalSync does before it writes, without the sync
// ledger. It reads the user files sync would edit and writes none.
func planGlobalWrites(home string, scope checkScope, targets []string) ([]globalWrite, error) {
	instructions, err := globalInstructions(scope.source, scope.bundle.Rules)
	if err != nil {
		return nil, err
	}
	old, err := loadGlobalState(filepath.Join(scope.source, "state", "global.json"))
	if err != nil {
		return nil, err
	}
	onUnsupported, err := loadGlobalOnUnsupported(scope.source)
	if err != nil {
		return nil, err
	}
	writes, _, err := buildGlobalWrites(home, scope.source, targets, instructions, scope.bundle, old, agentFailure(false, io.Discard), io.Discard, onUnsupported)
	return writes, err
}

// globalOverviewEvidenceNote states which checks ran for the global
// configuration and which did not.
func globalOverviewEvidenceNote(checks overviewChecks) string {
	var parts []string
	if len(checks.compared) > 0 {
		parts = append(parts, "The files sync --global would write were compared with disk for "+strings.Join(checks.compared, ", ")+".")
	}
	if len(checks.skipped) > 0 {
		parts = append(parts, "Not compared, because global sync does not support the tool, its configuration root could not be resolved, or the plan failed: "+strings.Join(checks.skipped, ", ")+".")
	}
	parts = append(parts, "The doctor checks read a project and did not run.", "Runtime enforcement is not observed.")
	return strings.Join(parts, " ")
}
