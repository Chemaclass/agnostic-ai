package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func newSyncCmd() *cobra.Command {
	var f syncFlags

	cmd := &cobra.Command{
		Use:   "sync",
		Args:  cobra.NoArgs,
		Short: "Emit per-target configs from agnostic specs.",
		Example: `  # Emit every target listed in agnostic-ai.yaml
  agnostic-ai sync

  # Emit only Claude and Cursor
  agnostic-ai sync --only claude,cursor

  # Emit everything except Codex
  agnostic-ai sync --except codex

  # Show changed targets and a short report
  agnostic-ai sync -v

  # Run sync again and show every target, path, and coverage note
  agnostic-ai sync -vv

  # Preview without writing
  agnostic-ai sync --dry-run

  # Preview only the changed lines, without writing
  agnostic-ai sync --dry-run --diff

  # CI gate: non-zero exit when output drifts from specs
  agnostic-ai sync --check

  # Show a unified diff of every drifted file
  agnostic-ai sync --check --diff

  # Emit GitHub Actions annotations so drift surfaces inline on the PR
  agnostic-ai sync --check --format=github

  # Back up each existing file to <path>.bak before overwriting
  agnostic-ai sync --backup

  # From a post-checkout hook: leave hand-edited outputs in place and name them
  agnostic-ai sync --keep-edits

  # Structured per-target diff (added/changed counts) without writing
  agnostic-ai sync --plan

  # Per-target counts, then a unified diff of each changed file
  agnostic-ai sync --plan --diff

  # Preview user-level changes and fail on drift
  agnostic-ai sync --global --plan --check

  # Machine-readable output for CI dashboards and editor extensions
  agnostic-ai sync --json

  # After moving generated files into the managed .gitignore block, stop tracking them
  agnostic-ai sync --untrack`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncCommand(cmd, f)
		},
	}
	cmd.Flags().StringSliceVarP(&f.targets, "target", "t", nil, "Targets to emit (default: all in config)")
	cmd.Flags().StringSliceVar(&f.only, "only", nil, "Emit only these targets (comma-separated); mutually exclusive with --except")
	cmd.Flags().StringSliceVar(&f.except, "except", nil, "Emit all configured targets except these (comma-separated); mutually exclusive with --only")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "Print outputs instead of writing")
	cmd.Flags().BoolVar(&f.check, "check", false, "Compare emitted output to disk; non-zero exit on drift")
	cmd.Flags().BoolVar(&f.diff, "diff", false, "Requires --check, --plan, or --dry-run; prints a unified diff per drifted file (default: counts only, so CI logs stay lean)")
	cmd.Flags().StringVar(&f.format, "format", checkFormatHuman, "With --check, drift report format: 'human' or 'github' (GitHub Actions ::error annotations)")
	cmd.Flags().StringVar(&f.against, "against", "", "With --check, compare what Git holds instead of the working tree: 'index' (staged, for pre-commit hooks) or 'HEAD' (the last commit, for CI). Only outputs Git tracks are compared.")
	cmd.Flags().BoolVar(&f.plan, "plan", false, "Show per-target added/changed counts without writing; with --diff, then a unified diff per changed file")
	cmd.Flags().BoolVar(&f.backup, "backup", false, "Copy each existing target file to <path>.bak before overwriting (consumed by `agnostic-ai revert`; clear leftover .bak with `agnostic-ai cleanup --backups`)")
	cmd.Flags().BoolVar(&f.keepEdits, "keep-edits", false, "Leave each output edited since the last sync in place and list it, writing the rest (for post-checkout and post-merge hooks)")
	cmd.Flags().BoolVar(&f.untrack, "untrack", false, "Remove a generated path from git's index when it is also gitignored (git rm --cached, working tree untouched). Not with --check, --plan, --dry-run, --watch, or --global.")
	cmd.Flags().BoolVar(&f.allowEmptyTargets, "allow-empty-targets", false, "With an empty targets list, remove every file sync generated instead of keeping them; with --dry-run, name each file it would remove. Not with --check, --plan, --watch, or --global.")
	cmd.Flags().StringVar(&f.gitignoreFlag, "gitignore", "", "Override config: 'on' or 'off' to manage the .gitignore block this run.")
	cmd.Flags().BoolVar(&f.watch, "watch", false, "Re-emit on spec changes (Ctrl+C to exit)")
	cmd.Flags().BoolVar(&f.watchPoll, "watch-poll", false, "Force polling instead of fsnotify (use on filesystems where fsnotify is unreliable, e.g. some network mounts)")
	cmd.Flags().BoolVar(&f.jsonOut, "json", false, "Output as JSON for machine consumption")
	cmd.Flags().BoolVar(&f.allTargets, "all", false, "Sync every configured target without prompting (skip the first-sync target picker)")
	cmd.Flags().IntVar(&f.jobs, "jobs", 0, "Number of targets to emit in parallel (0 = one per CPU; 1 = serial). Output is identical regardless.")
	cmd.Flags().BoolVar(&f.global, "global", false, "Sync user-level instructions, rules, hooks, skills, and native agents")
	registerTargetCompletion(cmd)
	return cmd
}

// previewOptions are what the sync previews do differently. tree is set
// under --against, which compares what Git holds, not these files.
// removals adds the files an empty targets list removes. keepEdits leaves
// edited merged files out of the leftovers, as the sync would.
type previewOptions struct {
	tree      *againstTree
	removals  func([]driftReport) ([]driftReport, error)
	keepEdits bool
}

// previewDrift runs the steps every sync preview shares, in order: reset
// the drops, check hand-written instructions, capture the drift, add the
// merged leftovers, and read the coverage notes of this capture.
func previewDrift(cfg *config.Config, bundle spec.Bundle, targets []string, backup bool, gitignoreFlag string, ledger ledgerSnapshot, opts previewOptions) ([]driftReport, syncDrops, error, error) {
	resetDrops()
	if opts.tree == nil {
		readers := targets
		if len(readers) == 0 {
			readers = cfg.Targets
		}
		if err := keepHandWrittenInstructions(cfg, bundle, readers, ledger.state.Outputs, backup, false); err != nil {
			return nil, syncDrops{}, nil, err
		}
	}
	reports, err := collectLoadedDriftWith(ledger, cfg, bundle, targets, nil, gitignoreFlag)
	if err != nil {
		return nil, syncDrops{}, nil, err
	}
	if opts.removals != nil {
		if reports, err = opts.removals(reports); err != nil {
			return nil, syncDrops{}, nil, err
		}
	}
	if opts.tree == nil {
		if reports, err = previewMergedLeftovers(reports, ledger.state, cfg.Sync.Unmanaged, opts.keepEdits); err != nil {
			return nil, syncDrops{}, nil, err
		}
	} else if reports, err = opts.tree.recollect(reports, targets, gitignoreFlag); err != nil {
		return nil, syncDrops{}, nil, err
	}
	drops, notesErr := checkCoverageNotes(cfg, targets)
	return reports, drops, notesErr, nil
}

// syncFlags are the flags of `agnostic-ai sync`.
type syncFlags struct {
	targets, only, except                               []string
	dryRun, check, plan, backup, keepEdits, untrack     bool
	watch, watchPoll, jsonOut, allTargets, diff, global bool
	allowEmptyTargets                                   bool
	gitignoreFlag, format, against                      string
	jobs                                                int
}

// writes reports whether the run may write files, so it takes the lock.
func (f syncFlags) writes() bool {
	return !f.check && !f.plan && !f.dryRun
}

// filtered reports whether the command line narrowed the targets.
func (f syncFlags) filtered() bool {
	return len(f.targets) > 0 || len(f.only) > 0 || len(f.except) > 0
}

func runSyncCommand(cmd *cobra.Command, f syncFlags) error {
	if f.jsonOut {
		previousOut := logOut
		logOut = io.Discard
		defer func() { logOut = previousOut }()
	}
	if err := validateSyncFlags(f); err != nil {
		return err
	}
	if f.global {
		return runGlobalSync(cmd, globalSyncOptions{targets: f.targets, only: f.only, except: f.except, dryRun: f.dryRun, check: f.check, backup: f.backup, plan: f.plan, watch: f.watch, watchPoll: f.watchPoll, jsonOut: f.jsonOut, allTargets: f.allTargets, diff: f.diff, format: f.format, gitignore: f.gitignoreFlag, jobs: f.jobs})
	}
	if err := refuseGlobalHome(".", globalHomeSyncRemedy); err != nil {
		return err
	}

	var tree *againstTree
	if f.against != "" {
		entered, err := enterAgainstTree(f.against)
		if err != nil {
			return err
		}
		tree = entered
		defer tree.leave()
	}
	// The state file lists every output, so one read serves the
	// whole run. --against reads the trees it enters instead.
	var read *ledgerSnapshot
	if f.writes() {
		command := "sync"
		if f.watch {
			command += " --watch"
		}
		lock, err := openProjectLock(".", command)
		if err != nil {
			return err
		}
		defer func() { _ = lock.Close() }()
		if tree == nil {
			l := readLedger(".")
			if err := l.refuseNewer(); err != nil {
				return err
			}
			read = &l
		}
		if err := ensureManagedGitignore("."); err != nil {
			return fmt.Errorf("ignore project lock: %w", err)
		}
	}
	// Under -v the write pass prints its load lines after its
	// report, so they wait for it.
	holdLoadLines := verbosity == levelVerbose && f.writes() && !f.jsonOut
	load := func() (projectLoad, error) {
		var held strings.Builder
		out := logOut
		if holdLoadLines {
			logOut = &held
		}
		defer func() { logOut = out }()
		cfg, bundle, err := loadProject(".")
		return projectLoad{cfg: cfg, bundle: bundle, log: held.String()}, err
	}
	project, err := load()
	if err != nil {
		return err
	}
	warnShadowedSpecs(cmd.ErrOrStderr(), project.bundle)
	cfg := project.cfg
	if noTargetsKept(cfg) && !f.allowEmptyTargets && !f.watch {
		return runEmptyTargets(cmd, f, cfg, tree)
	}
	if tree == nil && read == nil {
		l := readLedger(".")
		read = &l
	}
	ledger := func() ledgerSnapshot {
		if read != nil {
			return *read
		}
		return readLedger(".")
	}
	run := syncRun{cmd: cmd, f: f, cfg: cfg, bundle: project.bundle, project: project, tree: tree, ledger: ledger}
	base, picked, err := pickSyncTargets(cmd, f, cfg, ledger)
	if err != nil {
		return err
	}
	if picked {
		// The picker saved the targets to the config.
		if run.project, err = load(); err != nil {
			return err
		}
	}
	if run.effective, err = filterTargets(base, f.only, f.except); err != nil {
		return err
	}
	// A preview of a tool use has not imported yet would pass while
	// the next write would replace its native config.
	if tree == nil {
		if err := ledger().refuseNewer(); err != nil {
			return err
		}
		if err := stopOnPendingImports(ledger().state, cfg); err != nil {
			return err
		}
	}
	return run.dispatch()
}

// validateSyncFlags rejects flag combinations sync cannot honor, in a
// fixed order so the first conflict named stays the same. --global
// checks the rest of its flags itself.
func validateSyncFlags(f syncFlags) error {
	if f.jobs < 0 {
		return fmt.Errorf("--jobs: want 0 (one per CPU) or more, got %d", f.jobs)
	}
	if f.allowEmptyTargets && (f.check || f.plan || f.watch || f.global) {
		return errs.Coded(errs.CodeFlagConflict, "--allow-empty-targets cannot be combined with --check, --plan, --watch, or --global")
	}
	if f.keepEdits && (f.check || f.plan || f.watch || f.global) {
		return errs.Coded(errs.CodeFlagConflict, "--keep-edits cannot be combined with --check, --plan, --watch, or --global")
	}
	if err := validateAgainst(f.against, f.check, f.plan, f.watch, f.global); err != nil {
		return err
	}
	if f.untrack && (f.check || f.plan || f.dryRun || f.watch || f.global) {
		return errs.Coded(errs.CodeFlagConflict, "--untrack cannot be combined with --check, --plan, --dry-run, --watch, or --global")
	}
	if f.global {
		return nil
	}
	if err := validateGitignoreFlag(f.gitignoreFlag); err != nil {
		return err
	}
	if err := validateCheckFormat(f.format); err != nil {
		return err
	}
	if len(f.only) > 0 && len(f.except) > 0 {
		return errs.Coded(errs.CodeFlagConflict, "--only and --except are mutually exclusive")
	}
	if f.watch && f.check {
		return errs.Coded(errs.CodeFlagConflict, "--watch and --check are incompatible")
	}
	if f.diff && (f.watch || f.keepEdits) {
		return errs.Coded(errs.CodeFlagConflict, "--diff cannot be combined with --watch or --keep-edits")
	}
	if f.jsonOut && (f.watch || f.diff) {
		return errs.Coded(errs.CodeFlagConflict, "--json cannot be combined with --watch or --diff")
	}
	if f.diff && !f.check && !f.dryRun && !f.plan {
		return errs.Coded(errs.CodeFlagConflict, "--diff requires --check, --plan, or --dry-run; add --check to preview drift without writing")
	}
	return nil
}

// runEmptyTargets handles a config whose targets list is empty without
// --allow-empty-targets: it keeps every generated file and says so, and
// --check fails while generated files remain.
func runEmptyTargets(cmd *cobra.Command, f syncFlags, cfg *config.Config, tree *againstTree) error {
	kept := false
	if f.check {
		ledger := readLedger(".")
		if tree == nil {
			if err := ledger.checkError(); err != nil {
				return err
			}
		}
		kept = generatedFilesKept(cfg, ledger)
		if tree == nil && ledger.missing() {
			kept = true
		}
		if tree != nil {
			dropped, _, err := tree.droppedOutputs(false, nil)
			if err != nil {
				return err
			}
			for _, p := range dropped {
				if !inSpecsFolder(p) {
					kept = true
				}
			}
		}
	}
	if f.jsonOut {
		command := "sync"
		switch {
		case f.check:
			command = "sync --check"
		case f.plan:
			command = "sync --plan"
		case f.dryRun:
			command = "sync --dry-run"
		}
		if err := writeEmptyTargetsJSON(cmd, command, f.check && kept); err != nil {
			return err
		}
		if f.check && kept {
			return errEmptyTargetsCheck()
		}
		return nil
	}
	if f.check && kept {
		if f.format == checkFormatGitHub {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "::error::%s\n", githubData(errEmptyTargetsCheck().Error()))
		}
		return errEmptyTargetsCheck()
	}
	if len(f.targets) > 0 {
		warnEmptyTargetsIgnoring(cmd.ErrOrStderr())
	} else {
		warnEmptyTargets(cmd.ErrOrStderr())
	}
	return nil
}

// pickSyncTargets returns the targets this run starts from, before
// --only and --except, and whether the first-sync picker chose them.
func pickSyncTargets(cmd *cobra.Command, f syncFlags, cfg *config.Config, ledger func() ledgerSnapshot) ([]string, bool, error) {
	// A config target may name an external adapter a teammate has
	// not installed, so emit only warns about it. A name typed on
	// the command line is a request, and a typo must not pass as
	// "up to date".
	if err := resolveRequestedTargets(f.targets); err != nil {
		return nil, false, err
	}
	base := f.targets
	if len(base) == 0 {
		base = cfg.Targets
	}
	picked := false
	if f.writes() && !f.allTargets && !f.filtered() && shouldPromptTargetSelectionFor(ledger(), cfg) {
		pickerOut := cmd.OutOrStdout()
		if f.jsonOut {
			pickerOut = cmd.ErrOrStderr()
		}
		chosen, err := firstSyncTargetSelection(".", cmd.InOrStdin(), pickerOut)
		if err != nil {
			return nil, false, err
		}
		if len(chosen) > 0 {
			base, picked = chosen, true
		}
	}
	return base, picked, nil
}

// syncRun is one project sync after its flags, lock, project, and
// targets are settled. cfg and bundle are the project as first loaded;
// project is the one the write pass uses, reloaded after the picker.
type syncRun struct {
	cmd       *cobra.Command
	f         syncFlags
	cfg       *config.Config
	bundle    spec.Bundle
	project   projectLoad
	effective []string
	tree      *againstTree
	ledger    func() ledgerSnapshot
}

func (r syncRun) dispatch() error {
	f := r.f
	if f.plan || f.check || f.jsonOut && f.dryRun {
		warnBareCapabilitiesTo(r.cmd.ErrOrStderr(), r.cfg, r.bundle, r.effective)
	}
	if f.plan {
		return r.plan()
	}
	if f.check {
		return r.checkDrift()
	}
	if f.watchPoll && !f.watch {
		return errors.New("--watch-poll requires --watch")
	}
	if f.watch {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return watchSync(ctx, 200*time.Millisecond, ".", watchTargets(f.targets, f.only, f.except, r.effective), watchOptions{dryRun: f.dryRun, backup: f.backup, gitignoreFlag: f.gitignoreFlag, jobs: f.jobs}, f.watchPoll)
	}
	if f.dryRun && f.diff {
		reports, _, notesErr, err := r.preview(previewOptions{removals: r.withEmptyTargetsRemovals})
		if err != nil {
			return err
		}
		printDriftDiffs(r.cmd, reports)
		printDriftRemovals(r.cmd, reports)
		return notesErr
	}
	if f.jsonOut && f.dryRun {
		reports, drops, notesErr, err := r.preview(previewOptions{removals: r.withEmptyTargetsRemovals, keepEdits: f.keepEdits})
		if err != nil {
			return err
		}
		return printSyncPlanJSON(r.cmd, "sync --dry-run", reports, true, drops, notesErr)
	}
	opts := syncRunOptions{dryRun: f.dryRun, backup: f.backup, keepEdits: f.keepEdits, untrack: f.untrack, gitignoreFlag: f.gitignoreFlag, jobs: f.jobs}
	if f.jsonOut {
		return runSyncJSON(r.cmd, ".", r.ledger(), r.project, r.effective, opts)
	}
	if f.dryRun && f.allowEmptyTargets && noTargetsKept(r.cfg) {
		return r.previewEmptyTargets(opts)
	}
	return runSyncPassWith(".", r.ledger(), &r.project, r.effective, opts)
}

func (r syncRun) preview(opts previewOptions) ([]driftReport, syncDrops, error, error) {
	return previewDrift(r.cfg, r.bundle, r.effective, r.f.backup, r.f.gitignoreFlag, r.ledger(), opts)
}

// plan prints per-target counts, and with --check still gates, so a CI
// step that asks for the short report does not pass on drift.
func (r syncRun) plan() error {
	reports, drops, notesErr, err := r.preview(previewOptions{})
	if err != nil {
		return err
	}
	var driftErr error
	if r.f.check && slices.ContainsFunc(reports, driftReport.hasDrift) {
		driftErr = errDriftDetected()
	}
	// The ignore files are listed but never gate --check, which does not
	// compare them either; sync reports their errors.
	if resolveGitignore(r.cfg, r.f.gitignoreFlag) {
		cfg := *r.cfg
		cfg.Gitignore.Enabled = true
		ignores, err := planIgnoreFiles(".", &cfg, r.bundle, r.effective, reports, r.ledger())
		if err != nil {
			_, _ = fmt.Fprintf(r.cmd.ErrOrStderr(), "! cannot plan the ignore files: %v\n", err)
		} else {
			reports = withIgnorePlan(reports, ignores)
		}
	}
	if r.f.check {
		notesErr = errors.Join(notesErr, r.ledger().checkError())
	}
	if r.f.jsonOut {
		return errors.Join(printSyncPlanJSON(r.cmd, "sync --plan", reports, false, drops, notesErr), driftErr)
	}
	printSyncPlan(r.cmd, reports)
	if r.f.diff {
		printDriftDiffs(r.cmd, reports)
		printDriftRemovals(r.cmd, reports)
	}
	return errors.Join(notesErr, driftErr)
}

// checkDrift fails on drift between the specs and the files, or under
// --against, what Git holds.
func (r syncRun) checkDrift() error {
	f, cmd, cfg, tree := r.f, r.cmd, r.cfg, r.tree
	reports, drops, notesErr, err := r.preview(previewOptions{tree: tree})
	if err != nil {
		return err
	}
	// --against compares what Git holds, and Git does not hold the
	// ledger, so a hook gating a commit only hears about it.
	if tree == nil {
		notesErr = errors.Join(notesErr, r.ledger().checkError())
	} else if err := unparsableLedger(tree.origin); err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "! %v; run agnostic-ai sync to rebuild the ledger\n", err)
	}
	var linkFailures []againstSkillLinkFailure
	if tree != nil {
		dropped, note, err := tree.droppedOutputs(f.filtered(), reports)
		if err != nil {
			return err
		}
		if note != "" && verbosity >= levelDefault {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "note: %s\n", note)
		}
		linkFailures, err = tree.diagnoseSkillLinks(cfg, reports)
		if err != nil {
			return err
		}
		notesErr = errors.Join(notesErr, againstSkillLinkError(linkFailures))
		if reports, err = tree.trackedDrift(reports, dropped); err != nil {
			return err
		}
		markAgainst(reports, f.against)
		unmanaged, err := tree.trackedUnmanaged(cfg)
		if err != nil {
			return err
		}
		if len(unmanaged) > 0 {
			reports = append(reports, driftReport{Target: "unmanaged", Unmanaged: unmanaged})
		}
	}
	if slices.ContainsFunc(reports, driftReport.hasDrift) {
		links := trackedLinks
		if tree != nil {
			links = tree.exportedLinks
		}
		inputs, err := projectInputs(cfg)
		if err != nil {
			return err
		}
		plain, err := plainFileLinkFailures(".", links(), checkedPaths(inputs, reports), linkFailures, f.against)
		if err != nil {
			return err
		}
		notesErr = errors.Join(notesErr, againstSkillLinkError(plain))
		linkFailures = append(linkFailures, plain...)
	}
	if f.jsonOut {
		return printSyncCheckJSON(cmd, reports, drops, notesErr)
	}
	for _, failure := range linkFailures {
		if f.format == checkFormatGitHub {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "::error file=%s::%s\n", githubProp(failure.path), githubData(failure.message))
		} else {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), failure.message)
		}
	}
	err = reportCheckDrift(cmd, reports, f.format, f.diff)
	if err != nil && tree != nil && regeneratedDrift(reports) {
		err = errors.New(againstHint(f.against))
	}
	return errors.Join(err, notesErr)
}

// withEmptyTargetsRemovals adds the files --allow-empty-targets removes.
// The drift reports keep every file when targets is empty, so they name
// none of them.
func (r syncRun) withEmptyTargetsRemovals(reports []driftReport) ([]driftReport, error) {
	if !r.f.allowEmptyTargets || !noTargetsKept(r.cfg) {
		return reports, nil
	}
	links, err := plannedSkillLinks(r.cfg, r.bundle, r.effective)
	if err != nil {
		return nil, err
	}
	return append(reports, emptyTargetsRemovals(r.cfg, r.ledger(), reports, links, r.f.keepEdits)), nil
}

// previewEmptyTargets runs the dry-run pass for an empty targets list
// under --allow-empty-targets, then names each file the sweep it skips
// would remove.
func (r syncRun) previewEmptyTargets(opts syncRunOptions) error {
	if err := runSyncPassWith(".", r.ledger(), &r.project, r.effective, opts); err != nil {
		return err
	}
	resetDrops()
	reports, err := collectLoadedDriftWith(r.ledger(), r.cfg, r.bundle, r.effective, nil, r.f.gitignoreFlag)
	resetDrops()
	if err != nil {
		return err
	}
	if reports, err = r.withEmptyTargetsRemovals(reports); err != nil {
		return err
	}
	removals, err := previewMergedLeftovers(reports[len(reports)-1:], r.ledger().state, r.cfg.Sync.Unmanaged, r.f.keepEdits)
	if err != nil {
		return err
	}
	printDriftRemovals(r.cmd, removals)
	return nil
}
