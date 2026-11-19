package cli

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func doctorPreviewDrift(reports []driftReport, cfg *config.Config) ([]driftReport, error) {
	display := slices.Clone(reports)
	for i := range display {
		display[i].Stale = slices.Clone(display[i].Stale)
		display[i].Orphaned = slices.Clone(display[i].Orphaned)
	}
	display, err := previewMergedLeftovers(display, readStateFile("."), cfg.Sync.Unmanaged, false)
	if err != nil {
		return nil, err
	}
	for i := range display {
		for _, f := range display[i].Stale {
			if slices.Contains(reports[i].Leftover, f.Path) {
				if display[i].releasedMerged == nil {
					display[i].releasedMerged = map[string]bool{}
				}
				display[i].releasedMerged[f.Path] = true
			}
		}
	}
	return display, nil
}

const doctorLong = "Start troubleshooting with doctor. It is read-only by default and suggests a next step.\n" +
	"Review the findings before choosing a repair. Pass --fix to change files.\n\n" +
	"doctor runs these checks:\n" +
	"  1. Detect installed AI CLIs on PATH.\n" +
	"  2. Validate agnostic-ai.yaml config.\n" +
	"  3. Report unsupported spec kinds per target, then spec health: the\n" +
	"     findings `agnostic-ai lint` reports.\n" +
	"  4. Report agentic config on disk not single-sourced from .agnostic-ai/,\n" +
	"     project skills or agents that share a name with a global one, and\n" +
	"     synced skills a Claude settings file turns off (skillOverrides).\n" +
	"  5. Compare what sync would emit against files on disk (drift), and flag\n" +
	"     a generated file still tracked despite being ignored.\n" +
	"     --check-globs and --check-references add opt-in checks here.\n" +
	"  6. Check MCP server command binaries.\n" +
	"  7. Check persisted Codex hook trust, and Claude hook specs an older\n" +
	"     import merged so each hook runs twice.\n" +
	"  8. Check existing packaging ignore files against generated paths.\n" +
	"  9. Suggest a concrete next step.\n\n" +
	"Exits non-zero on any drift, lint error, inactive Codex hook, or unknown -t target; lint warnings show without\n" +
	"failing. Subcommands run individual checks.\n\n" +
	"--scope project skips local Codex hook trust and reports it as skipped, " +
	"and reads only the shared Claude settings file for skillOverrides. " +
	"All project checks stay enabled. With --fix, only project drift is repaired; " +
	"trust is never approved. JSON output stays read-only, so --json rejects --fix."

const doctorExample = `  # Project and local runtime diagnostic
  agnostic-ai doctor

  # Validate project files on a fresh CI runner
  agnostic-ai doctor --scope project --check-references

  # Reconcile drift in place, keeping a .bak of every file it overwrites
  agnostic-ai doctor --fix --backup

  # Machine-readable drift report for CI dashboards
  agnostic-ai doctor --json

  # Fail when a generated skill links to a missing local file
  agnostic-ai doctor --check-references

  # Check only MCP command resolution
  agnostic-ai doctor mcp

  # Check only installed AI CLIs
  agnostic-ai doctor install`

// doctorOptions holds the doctor flags.
type doctorOptions struct {
	targets    []string
	scope      string
	fix        bool
	backup     bool
	jsonOut    bool
	checkGlobs bool
	checkRefs  bool
}

func newDoctorCmd() *cobra.Command {
	var opts doctorOptions
	cmd := &cobra.Command{
		Use:     "doctor",
		Short:   "Diagnose project and local setup problems, then suggest a next step.",
		Long:    doctorLong,
		Example: doctorExample,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDoctorScope(opts.scope); err != nil {
				return err
			}
			if opts.jsonOut && opts.fix {
				return errs.Coded(errs.CodeFlagConflict, "--json is read-only and cannot be combined with --fix")
			}
			if opts.fix {
				if err := readLedger(".").refuseNewer(); err != nil {
					return err
				}
			} else {
				readLedger(".").warnNewer()
			}
			if opts.jsonOut {
				return runDoctorJSON(cmd, opts)
			}
			return runDoctorText(cmd, opts)
		},
	}
	cmd.Flags().StringSliceVarP(&opts.targets, "target", "t", nil, "Targets to check (default: all in config)")
	cmd.Flags().StringVar(&opts.scope, "scope", "all", "Checks to run: all includes local hook trust and personal Claude settings; project skips them")
	cmd.Flags().BoolVar(&opts.fix, "fix", false, "Reconcile drift, and in a terminal offer removal of kept orphans and of skillOverrides entries for synced skills")
	cmd.Flags().BoolVar(&opts.backup, "backup", false, "With --fix, copy each existing file to <path>.bak before overwriting, confirmed orphan removal, or confirmed skillOverrides removal")
	cmd.Flags().BoolVar(&opts.jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&opts.checkGlobs, "check-globs", false, "Flag rules whose `globs:` pattern matches no files in the working tree")
	cmd.Flags().BoolVar(&opts.checkRefs, "check-references", false, "Flag relative Markdown links in emitted skills whose file is missing on disk")
	registerTargetCompletion(cmd)
	cmd.AddCommand(newDoctorMCPCmd())
	cmd.AddCommand(newDoctorInstallCmd())
	cmd.AddCommand(newDoctorConfigCmd())
	cmd.AddCommand(newDoctorRTKCmd())
	return cmd
}

// runDoctorJSON emits only the drift report, skipping the human-readable sections.
func runDoctorJSON(cmd *cobra.Command, opts doctorOptions) error {
	targets := opts.targets
	if err := resolveRequestedTargets(targets); err != nil {
		return err
	}
	reports, err := collectDrift(targets)
	if err != nil {
		return err
	}
	var refs []referenceFinding
	if opts.checkRefs {
		if refs, _, err = collectReferenceFindings(targets); err != nil {
			return err
		}
	}
	scope, err := loadCheckScope(false)
	if err != nil {
		return err
	}
	displayReports, err := doctorPreviewDrift(reports, scope.cfg)
	if err != nil {
		return err
	}
	lint, accepted, err := lintScopeReport(scope)
	if err != nil {
		return err
	}
	gate := doctorGateFindings{nestedCopies: nestedClaudeCopies(scope.cfg, scope.bundle)}
	if opts.checkGlobs {
		globs, err := collectUnmatchedGlobs(".", scope.bundle)
		if err != nil {
			return err
		}
		gate.globs = &globs
	}
	if gate.hookScripts, _, err = collectDivergentHookScripts("."); err != nil {
		return err
	}
	hookTrust, trustCheck := doctorTrustForScope(scope.cfg, targets, opts.scope)
	var overrides []*skillOverrideReport
	if claudeSkillOverridesSelected(scope.cfg, targets) {
		overrides = collectClaudeSkillOverrides(scope.cfg, scope.bundle, opts.scope == "project")
	}
	return printDoctorJSON(cmd, displayReports, gate, refs, opts.checkRefs, lint, accepted, hookTrust, collectPackagingIgnoreFindings(reports), trustCheck, skillOverrideRecords(overrides))
}

// doctorTextFindings carries what the text checks found into the final gate.
type doctorTextFindings struct {
	drift           doctorDrift
	scriptDrift     bool
	lint            []lintFinding
	hookTrust       []codex.HookTrustFinding
	unmanaged       int
	packaging       []packagingIgnoreFinding
	mergedHooks     int
	unloadableRules int
	brokenRefs      int
}

// runDoctorText prints the numbered checks, suggests a next step, and with
// --fix reconciles drift.
func runDoctorText(cmd *cobra.Command, opts doctorOptions) error {
	configOK := doctorConfigOK()

	// 1. Installed CLIs
	reportInstalledCLIs(cmd)

	// 2. Config check
	cmd.Println()
	cmd.Println("Config:")
	if !configOK {
		cmd.Println("  ✗ agnostic-ai.yaml not found. Run: agnostic-ai init")
		doctorNextStep(cmd, false, false, false, nil, 0, 0, 0, errDoctorNoConfig, 0, 0)
		return errDoctorNoConfig
	}
	if err := resolveRequestedTargets(opts.targets); err != nil {
		cmd.Printf("  ✗ %v\n", err)
		return err
	}
	scope, err := loadCheckScope(false)
	if err != nil {
		cmd.Printf("  ✗ %v\n", err)
		doctorNextStep(cmd, false, false, false, nil, 0, 0, 0, err, 0, 0)
		return err
	}
	cfg := scope.cfg
	if noTargetsKept(cfg) {
		warnEmptyTargets(cmd.OutOrStdout())
		opts.targets = nil
	}
	targets := opts.targets
	// These two read files only and take a large share of a run on
	// a big project, so they run beside the checks before them.
	var (
		unmanagedFound []unmanagedFinding
		unmanagedErr   error
		migrationHint  string
		background     sync.WaitGroup
	)
	background.Add(2)
	go func() {
		defer background.Done()
		unmanagedFound, unmanagedErr = findUnmanagedConfig(".", cfg)
	}()
	go func() {
		defer background.Done()
		migrationHint = pendingMigrationHint(scope.migrationScope())
	}()
	configPath, _, _ := config.ResolveConfigPath(".")
	cmd.Printf("  ✓ %s valid (version %d, %d target(s))\n", filepath.Base(configPath), cfg.Version, len(cfg.Targets))

	// 3. Unsupported kinds, then what `lint` reports.
	reportUnsupportedKinds(cmd, cfg, scope.bundle)
	lint, err := reportSpecHealth(cmd, scope)
	background.Wait()
	if err != nil {
		return err
	}

	// 3b. Config present on disk but not single-sourced, then
	// the paths the user owns through sync.unmanaged.
	unmanagedFound, leftOut := withoutLeftOutClaudeAgents(unmanagedFound, scope.bundle, cfg)
	unmanaged := printUnmanagedConfig(cmd, cfg, unmanagedFound, unmanagedErr)
	for _, line := range leftOut {
		cmd.Printf("  ! %s\n", line)
	}
	reportUserOwned(cmd, cfg)
	reportLegacyDefaultInstructions(cmd)
	reportGlobalNameClashes(cmd, scope.bundle, cfg)
	if claudeSkillOverridesSelected(cfg, targets) {
		overrides := collectClaudeSkillOverrides(cfg, scope.bundle, opts.scope == "project")
		reportClaudeSkillOverrides(cmd, overrides)
		if opts.fix {
			if err := fixClaudeSkillOverrides(cmd, localSkillOverrides(overrides), opts.backup); err != nil {
				return err
			}
		}
	}
	if migrationHint != "" {
		cmd.Println()
		cmd.Println("Spec migrations:")
		cmd.Printf("  ! %s\n", migrationHint)
	}

	// 4. Drift
	drift, err := reportDoctorDrift(cmd, opts, cfg, scope.bundle)
	if err != nil {
		return err
	}
	bundle := drift.bundle

	// 4b. Optional: globs that match nothing in the working tree.
	unloadableRules := 0
	if opts.checkGlobs {
		cmd.Println()
		cmd.Println("Glob coverage:")
		n, err := reportUnmatchedGlobs(cmd, ".")
		if err != nil {
			return err
		}
		unloadableRules = n
	}

	// 4c. Optional: relative links in emitted skill documents
	// whose destination is missing on disk.
	brokenRefs := 0
	if opts.checkRefs {
		n, err := reportBrokenReferences(cmd, targets)
		if err != nil {
			return err
		}
		brokenRefs = n
	}

	// 4d. Generated files git tracks despite ignoring.
	reportTrackedIgnored(cmd, drift.reports)

	// 5. MCP resolution
	reportMCPCommandResolution(cmd, bundle)
	reportMCPUnsetEnvRefs(cmd, cfg, bundle, targets)

	// 5b. Hook script body divergence across per-tool stashes.
	scriptDrift, err := reportDivergentHookScripts(cmd, ".")
	if err != nil {
		return err
	}
	drift.copiesOnly = drift.copiesOnly && !scriptDrift
	drift.hasDrift = drift.hasDrift || scriptDrift || len(drift.copies) > 0

	packaging := collectPackagingIgnoreFindings(drift.reports)
	reportPackagingIgnoreFindings(cmd, packaging)
	hookTrust, trustCheck := doctorTrustForScope(cfg, targets, opts.scope)
	if trustCheck != nil {
		cmd.Printf("\nCodex hook trust:\n  SKIPPED: %s\n", trustCheck.Reason)
	}
	reportCodexHookTrust(cmd, hookTrust)
	mergedHooks := mergedClaudeHookSpecs(cfg, bundle)
	reportMergedClaudeHookSpecs(cmd, cfg, mergedHooks)

	return finishDoctorText(cmd, opts, cfg, doctorTextFindings{
		drift:           drift,
		scriptDrift:     scriptDrift,
		lint:            lint,
		hookTrust:       hookTrust,
		unmanaged:       unmanaged,
		packaging:       packaging,
		mergedHooks:     len(mergedHooks),
		unloadableRules: unloadableRules,
		brokenRefs:      brokenRefs,
	})
}

// doctorDrift is what the drift step found, and the bundle the later
// steps read.
type doctorDrift struct {
	bundle        spec.Bundle
	copies        []nestedClaudeCopy
	removedCopies int
	reports       []driftReport
	hasDrift      bool
	copiesOnly    bool
}

// reportDoctorDrift prints the drift section. With --fix it first removes
// nested CLAUDE.md copies, which would stop the drift check.
func reportDoctorDrift(cmd *cobra.Command, opts doctorOptions, cfg *config.Config, bundle spec.Bundle) (doctorDrift, error) {
	cmd.Println()
	cmd.Println("Sync drift:")
	var copies []nestedClaudeCopy
	if !noTargetsKept(cfg) {
		copies = nestedClaudeCopies(cfg, bundle)
	}
	reportNestedClaudeCopies(cmd, copies)
	removedCopies := 0
	var err error
	if opts.fix {
		if removedCopies, err = removeNestedClaudeCopies(copies, opts.backup); err != nil {
			return doctorDrift{}, err
		}
	}
	// A removed copy can itself be a spec, when rules are sourced
	// from the project tree, so load the project again after one.
	if removedCopies > 0 {
		if _, bundle, err = loadProject("."); err != nil {
			return doctorDrift{}, err
		}
	}
	reports, err := collectLoadedDrift(cfg, bundle, opts.targets, "")
	if err != nil {
		return doctorDrift{}, err
	}
	displayReports, err := doctorPreviewDrift(reports, cfg)
	if err != nil {
		return doctorDrift{}, err
	}
	hasDrift := printDrift(displayReports)
	// A target that did not resolve was warned about and skipped,
	// so its files were never compared.
	if !hasDrift && len(copies) == 0 && allTargetsResolve(opts.targets, cfg.Targets) {
		cmd.Println("  ✓ generated files in sync")
	}
	return doctorDrift{
		bundle:        bundle,
		copies:        copies,
		removedCopies: removedCopies,
		reports:       reports,
		hasDrift:      hasDrift,
		copiesOnly:    !hasDrift && len(copies) > 0,
	}, nil
}

// finishDoctorText suggests the next step, then decides the exit: the
// opt-in gates first, then drift (reconciled with --fix), hook trust, lint.
func finishDoctorText(cmd *cobra.Command, opts doctorOptions, cfg *config.Config, f doctorTextFindings) error {
	// 6. Next step
	reports := f.drift.reports
	manualFiles, manualOnly := manualOnlyDrift(reports)
	// Hook script divergence is drift a scope document does not explain.
	manualOnly = manualOnly && !f.scriptDrift && len(f.drift.copies) == 0
	doctorNextStep(cmd, f.drift.hasDrift, manualOnly, f.drift.copiesOnly, manualFiles, len(f.lint), len(f.hookTrust), f.unmanaged, nil, len(f.packaging), f.mergedHooks)

	// A rule whose globs match nothing never loads, so it
	// silently does not exist. Reported before, but exit 0 meant
	// no CI step could gate on it (#617).
	if f.unloadableRules > 0 {
		return fmt.Errorf("%d rule(s) have a glob matching no files and will never load", f.unloadableRules)
	}
	if f.brokenRefs > 0 {
		return fmt.Errorf("%d broken skill reference(s): a relative link points to a file missing from the emitted skill", f.brokenRefs)
	}
	hookTrust := f.hookTrust
	if f.drift.hasDrift {
		if !opts.fix {
			if manualOnly {
				return fmt.Errorf("drift detected. delete %s", manualRemovalAdvice(manualFiles))
			}
			if f.drift.copiesOnly {
				return fmt.Errorf("drift detected. run `agnostic-ai doctor --fix`")
			}
			return fmt.Errorf("drift detected. run `agnostic-ai sync` to reconcile, or `agnostic-ai doctor --fix`")
		}
		fixed, err := fixDrift(reports, opts.backup)
		if err != nil {
			return err
		}
		removedOrphans, err := offerOrphanRemoval(cfg, reports, opts.backup, orphanRemovalPrompt(cmd))
		if err != nil {
			return err
		}
		summaryf("→ reconciled %d file(s)\n", fixed+f.drift.removedCopies+removedOrphans)
		hookTrust, _ = doctorTrustForScope(cfg, opts.targets, opts.scope)
		reportCodexHookTrust(cmd, hookTrust)
		if n := orphanedCount(reports); n > 0 {
			return fmt.Errorf("%d orphaned file(s) need manual removal", n)
		}
	}
	if err := codexHookTrustErr(hookTrust); err != nil {
		return err
	}
	return lintErrorsErr(f.lint)
}

// doctorGateFindings are the checks besides drift that fail text mode.
// A nil globs means --check-globs was not passed.
type doctorGateFindings struct {
	globs        *[]unmatchedGlob
	nestedCopies []nestedClaudeCopy
	hookScripts  []divergentHookScript
}

// nestedCopyRecord is a nested CLAUDE.md a rule holds, as doctor --json lists it.
type nestedCopyRecord struct {
	Path string `json:"path"`
	Rule string `json:"rule"`
}

func nestedCopyRecords(copies []nestedClaudeCopy) []nestedCopyRecord {
	out := make([]nestedCopyRecord, 0, len(copies))
	for _, c := range copies {
		out = append(out, nestedCopyRecord{Path: filepath.ToSlash(c.path), Rule: c.rule})
	}
	return out
}

// doctorJSONOutput adds lint, hook trust, packaging warnings, skill overrides, the checks text mode fails on, and opt-in references and globs.
type doctorJSONOutput struct {
	jsonOutput
	Lint            []lintFinding            `json:"lint"`
	References      *[]referenceFinding      `json:"references,omitempty"`
	Globs           *[]unmatchedGlob         `json:"globs,omitempty"`
	NestedCopies    []nestedCopyRecord       `json:"nested_copies"`
	HookScriptDrift []divergentHookScript    `json:"hook_script_drift"`
	HookTrust       []codex.HookTrustFinding `json:"hook_trust"`
	HookTrustCheck  *doctorTrustCheck        `json:"hook_trust_check,omitempty"`
	PackagingIgnore []packagingIgnoreFinding `json:"packaging_ignore"`
	// CoverageAccepted counts the coverage notes coverage.accept matches.
	CoverageAccepted int                   `json:"coverage_accepted"`
	SkillOverrides   []skillOverrideRecord `json:"skill_overrides"`
}

// printDoctorJSON emits a JSON drift report for `doctor`. Mirrors the schema
// used by `sync --check --json`: missing, stale, and orphaned files appear
// in writes. Lint, hook trust, packaging, nested copy, and hook script findings have their own lists. With checkRefs, broken skill
// references appear in references, and with --check-globs, rules that never load appear in globs.
func printDoctorJSON(cmd *cobra.Command, reports []driftReport, gate doctorGateFindings, refs []referenceFinding, checkRefs bool, lint []lintFinding, coverageAccepted int, hookTrust []codex.HookTrustFinding, packaging []packagingIgnoreFinding, trustCheck *doctorTrustCheck, skillOverrides []skillOverrideRecord) error {
	if lint == nil {
		lint = []lintFinding{}
	}
	out := doctorJSONOutput{jsonOutput: jsonOutput{Version: "1", Command: "doctor", Writes: driftRecords(reports)}.forOutput(), Lint: slashLintPaths(lint), HookTrust: slashHookTrustPaths(hookTrust), PackagingIgnore: slashPackagingPaths(packaging), CoverageAccepted: coverageAccepted, HookTrustCheck: trustCheck, SkillOverrides: skillOverrides,
		Globs: gate.globs, NestedCopies: nestedCopyRecords(gate.nestedCopies), HookScriptDrift: gate.hookScripts}
	if out.HookScriptDrift == nil {
		out.HookScriptDrift = []divergentHookScript{}
	}
	if out.HookTrust == nil {
		out.HookTrust = []codex.HookTrustFinding{}
	}
	if out.PackagingIgnore == nil {
		out.PackagingIgnore = []packagingIgnoreFinding{}
	}
	if checkRefs {
		if refs == nil {
			refs = []referenceFinding{}
		}
		slashed := slashReferencePaths(refs)
		out.References = &slashed
	}
	if err := writeIndentedJSON(cmd, out); err != nil {
		return err
	}
	if gate.globs != nil && len(*gate.globs) > 0 {
		return fmt.Errorf("%d rule(s) have a glob matching no files and will never load", len(*gate.globs))
	}
	if len(out.Writes) > 0 || len(out.NestedCopies) > 0 || len(out.HookScriptDrift) > 0 {
		return fmt.Errorf("drift detected")
	}
	if len(refs) > 0 {
		return fmt.Errorf("%d broken skill reference(s)", len(refs))
	}
	if err := codexHookTrustErr(hookTrust); err != nil {
		return err
	}
	return lintErrorsErr(lint)
}
