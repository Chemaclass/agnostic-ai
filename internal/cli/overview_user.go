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

// newUserOverviewHandler serves the overview of the
// user-level configuration sync --scope user reads, built afresh on every
// request. Unlike the project handler it runs from any working
// directory: every path comes from $HOME and the user source root.
func newUserOverviewHandler(compareInstalled bool) http.Handler {
	return overview.NewHandler(func() (overview.Inputs, error) {
		return userOverviewInputsWith(compareInstalled)
	})
}

// userOverviewInputs loads the user and user-local layers as
// lint --scope user does, runs the read-only checks that apply to them, and
// builds the snapshot the overview names. The snapshot uses the options
// the served Authority view uses for --scope user, so both report one
// revision for the same sources.
func userOverviewInputs() (overview.Inputs, error) {
	return userOverviewInputsWith(true)
}

// userOverviewInputsWith is userOverviewInputs that skips comparing
// the files sync --scope user would write with disk when compareInstalled is
// false.
func userOverviewInputsWith(compareInstalled bool) (overview.Inputs, error) {
	scope, err := loadCheckScope(true)
	if err != nil {
		return overview.Inputs{}, err
	}
	builtins, err := loadUserBuiltins(scope.source, nil)
	if err != nil {
		return overview.Inputs{}, err
	}
	cfg := &config.Config{Targets: scope.targets, Builtins: builtins, Models: scope.models, Requires: userRequires(scope.source)}
	checks := runUserOverviewChecks(scope, compareInstalled)
	src := &authoritySource{root: scope.source, userLevel: true}
	opts := authority.Options{Root: scope.source, ConfigFiles: src.configFiles(), UserLevel: true}
	snapshot := authority.NewSnapshot(cfg, scope.bundle, opts)
	snapshot.Evidence.Installed = authority.InstalledNotChecked
	if len(checks.compared) > 0 {
		snapshot.Evidence.Installed = "checked"
	}
	snapshot.Evidence.Note = userOverviewEvidenceNote(checks)
	team, local := overviewBuiltins(scope.source, true)
	return overview.Inputs{
		Cfg:            cfg,
		UserLevel:      true,
		Bundle:         scope.bundle,
		Snapshot:       snapshot,
		Instructions:   userInstructionDocuments(scope.source),
		Findings:       checks.findings,
		Compared:       checks.compared,
		FieldNotes:     checks.fieldNotes,
		MCPTools:       mcpprobe.LoadCache(scope.source),
		RefreshCommand: "agnostic-ai mcp tools --scope user",
		BuiltinsTeam:   team,
		BuiltinsLocal:  local,
		Tools:          overviewTools(scope.source, true),
		NativeMCPs:     userNativeMCPServers(scope.source, cfg),
	}, nil
}

// userNativeMCPServers lists the MCP servers the targets hold in their
// own user files. Without a home it lists none.
func userNativeMCPServers(source string, cfg *config.Config) []overview.NativeMCP {
	home, err := userHomeDir()
	if err != nil {
		return nil
	}
	return nativeMCPServers(home, ".", source, cfg, true)
}

// userRequires is the requires value of the home config, where
// local/agnostic-ai.yaml replaces the shared one. A config that cannot
// be read sets none; lint --scope user reports it.
func userRequires(source string) string {
	var requires string
	for _, path := range userConfigPaths(source) {
		doc, err := readUserConfig(path)
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

// runUserOverviewChecks runs the read-only checks the user overview
// reads: every finding lint --scope user reports, and drift between the
// files sync --scope user would write and the files on disk. The doctor
// checks read a project's generated files and do not run.
func runUserOverviewChecks(scope checkScope, compareInstalled bool) overviewChecks {
	overviewChecksMu.Lock()
	defer overviewChecksMu.Unlock()
	// Planning the user writes prints capability warnings and buffers
	// coverage notes; a read-only view reports neither.
	defer adapters.SetAsideNotes()()
	adapters.SetWarner(io.Discard)
	defer adapters.SetWarner(os.Stderr)
	display := func(path string) string { return userOverviewPath(scope.source, path) }
	var out overviewChecks
	out.findings = append(out.findings, overviewScopeLintFindings(scope, userConfigPaths(scope.source)[0], display)...)
	setFindingOpenPaths(out.findings, scope.source)
	adapters.DrainNotes()
	if compareInstalled {
		drift, compared, skipped := userOverviewDriftFindings(scope)
		out.fieldNotes = settingsFieldNotes(adapters.DrainNotes())
		out.findings = append(out.findings, drift...)
		out.compared, out.skipped = compared, skipped
	} else {
		out.fieldNotes = userSettingsFieldNotes(scope)
	}
	return out
}

// userSettingsFieldNotes resolves the user settings sync --scope user
// would write for each target it syncs, without planning any file, and
// returns the model and effort notes that raises.
func userSettingsFieldNotes(scope checkScope) []overview.FieldNote {
	targets, _ := userSyncedTargets(scope.targets)
	for _, target := range targets {
		userSettingsFor(target, userTargets[target], scope.bundle.Settings)
	}
	return settingsFieldNotes(adapters.DrainNotes())
}

// userSyncedTargets splits targets into the ones a default sync
// --scope user writes and the ones it skips: unsupported, or with a
// configuration root that cannot be resolved.
func userSyncedTargets(all []string) (targets, skipped []string) {
	for _, target := range all {
		if unsupportedUserTarget("--scope user", target) != nil || userTargets[target].rootError(target) != nil {
			skipped = append(skipped, target)
			continue
		}
		targets = append(targets, target)
	}
	return targets, skipped
}

// userOverviewPath shows a path under the user source relative to
// it, as the Authority view lists the specs, and any other path, such
// as a generated file under $HOME, as it is.
func userOverviewPath(source, path string) string {
	rel, err := filepath.Rel(source, path)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

// userOverviewDriftFindings compares the files sync --scope user would
// write for each target with the files on disk, as sync --scope user --check
// does, and credits each file to the targets whose surfaces it carries.
// A missing file is "not generated" and a differing one is "stale". The
// sync ledger is left out, because no tool reads it. A target user
// sync does not support, or whose configuration root cannot be
// resolved, is skipped, as a default sync skips it. Nothing is written:
// the writes are planned in memory and compared.
func userOverviewDriftFindings(scope checkScope) (findings []overview.Finding, compared, skipped []string) {
	home, err := userHomeDir()
	if err != nil {
		return nil, nil, scope.targets
	}
	targets, skipped := userSyncedTargets(scope.targets)
	if len(targets) == 0 {
		return nil, nil, skipped
	}
	writes, err := planUserWrites(home, scope, targets)
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

// planUserWrites builds the files a default sync --scope user would write
// for targets, as runUserSync does before it writes, without the sync
// ledger. It reads the user files sync would edit and writes none.
func planUserWrites(home string, scope checkScope, targets []string) ([]userWrite, error) {
	instructions, err := userInstructions(scope.source, scope.models, scope.bundle.Rules)
	if err != nil {
		return nil, err
	}
	old, err := loadUserState(filepath.Join(scope.source, "state", "global.json"))
	if err != nil {
		return nil, err
	}
	onUnsupported, err := loadUserOnUnsupported(scope.source)
	if err != nil {
		return nil, err
	}
	writes, _, err := buildUserWrites(home, scope.source, targets, instructions, scope.bundle, old, agentFailure(false, io.Discard), io.Discard, onUnsupported)
	return writes, err
}

// userOverviewEvidenceNote states which checks ran for the user
// configuration and which did not.
func userOverviewEvidenceNote(checks overviewChecks) string {
	var parts []string
	if len(checks.compared) == 0 && len(checks.skipped) == 0 {
		parts = append(parts, "The files sync --scope user would write were not compared with disk.")
	}
	if len(checks.compared) > 0 {
		parts = append(parts, "The files sync --scope user would write were compared with disk for "+strings.Join(checks.compared, ", ")+".")
	}
	if len(checks.skipped) > 0 {
		parts = append(parts, "Not compared, because user sync does not support the tool, its configuration root could not be resolved, or the plan failed: "+strings.Join(checks.skipped, ", ")+".")
	}
	parts = append(parts, "The doctor checks read a project and did not run.", "Runtime enforcement is not observed.")
	return strings.Join(parts, " ")
}
