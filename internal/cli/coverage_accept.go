package cli

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// applyCoverageAccept takes the notes coverage.accept matches out of the
// buffers and returns them. With coverage.fail-on-notes it also returns
// an error when a note that names a target is still buffered.
func applyCoverageAccept(cfg *config.Config, targets []string) ([]adapters.AcceptedNote, error) {
	adapters.OrderBufferedDropsByTarget(targets)
	accepted, _ := adapters.AcceptCoverageNotes(cfg.Coverage.Accept)
	if !cfg.Coverage.FailOnNotes {
		return accepted, nil
	}
	if n := adapters.PendingTargetCoverageNotesCount(); n > 0 {
		return accepted, fmt.Errorf("coverage.fail-on-notes: %d coverage note%s not accepted; fix the spec, or list the note under coverage.accept in agnostic-ai.yaml with a reason", n, plural(n))
	}
	return accepted, nil
}

// flushFailedCoverageNotes prints the notes that failed
// coverage.fail-on-notes. Under -q they still go to stderr, so the
// failure names them.
func flushFailedCoverageNotes() {
	if verbosity < levelDefault {
		adapters.SetWarner(os.Stderr)
		defer adapters.SetWarner(io.Discard)
	}
	adapters.FlushCoverageNotes()
}

// checkCoverageNotes applies coverage.fail-on-notes to the notes a
// capture-only run (check, plan, dry-run JSON) buffered. On failure it
// prints the notes, which those runs otherwise leave out.
func checkCoverageNotes(cfg *config.Config, targets []string) error {
	_, err := applyCoverageAccept(cfg, targets)
	if err != nil {
		flushFailedCoverageNotes()
	}
	adapters.ResetCoverageNotes()
	return err
}

// validateCoverageTargets rejects a coverage.accept target that is neither
// a built-in adapter nor listed in targets, where an external adapter
// would be named.
func validateCoverageTargets(cfg *config.Config) error {
	known := adapters.Names()
	for i, a := range cfg.Coverage.Accept {
		for _, t := range a.Target {
			if slices.Contains(known, t) || slices.Contains(cfg.Targets, t) {
				continue
			}
			if s := suggest.Name(t, known); s != "" {
				return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: unknown target %q (did you mean %s?)", config.ConfigFileName, i, t, s)
			}
			return errs.Coded(errs.CodeConfigDecode, "%s: coverage.accept[%d]: unknown target %q", config.ConfigFileName, i, t)
		}
	}
	return nil
}

// coverageMatch is what emitting every target in memory says about
// coverage.accept.
type coverageMatch struct {
	accepted  int
	unmatched []config.CoverageAccept
	// failed names each target whose emission failed, so its entries
	// cannot be checked.
	failed map[string]error
}

// matchCoverageAccept emits every target in memory and matches the notes
// they raise against coverage.accept. An entry's targets outside targets
// are not emitted, so they never count as unmatched. The policy is forced to warn so a
// note that on-unsupported: error turns into a failure is still raised.
// A target that fails to resolve or emit is recorded and skipped.
func matchCoverageAccept(cfg *config.Config, b spec.Bundle, targets []string) coverageMatch {
	var m coverageMatch
	if len(cfg.Coverage.Accept) == 0 {
		return m
	}
	restore := adapters.SetAsideNotes()
	defer restore()
	view := *cfg
	view.OnUnsupported = "warn"
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err == nil {
			_, err = captureAdapterFiles(sess, adapter, b, &view)
		}
		if err != nil {
			if m.failed == nil {
				m.failed = map[string]error{}
			}
			m.failed[t] = err
		}
	}
	adapters.OrderBufferedDropsByTarget(targets)
	accepted, unmatched := adapters.AcceptCoverageNotes(cfg.Coverage.Accept)
	m.accepted = len(accepted)
	for _, a := range unmatched {
		a.Target = slices.DeleteFunc(a.Target, func(t string) bool { return m.failed[t] != nil || !slices.Contains(targets, t) })
		if len(a.Target) > 0 {
			m.unmatched = append(m.unmatched, a)
		}
	}
	return m
}

// lintCoverageAccept flags a coverage.accept entry that matches no note
// (LINT024, warn), so an entry goes stale visibly once a target starts
// supporting the field. A target that fails to emit gets a finding of
// its own instead.
func lintCoverageAccept(m coverageMatch) []lintFinding {
	var findings []lintFinding
	for _, a := range m.unmatched {
		findings = append(findings, lintFinding{
			Code:     "LINT024",
			Severity: lintWarn,
			Path:     config.ConfigFileName,
			Message:  fmt.Sprintf("coverage.accept entry %s matches no coverage note; remove it", a),
		})
	}
	failed := make([]string, 0, len(m.failed))
	for t := range m.failed {
		failed = append(failed, t)
	}
	slices.Sort(failed)
	for _, t := range failed {
		findings = append(findings, lintFinding{
			Code:     "LINT024",
			Severity: lintWarn,
			Path:     config.ConfigFileName,
			Message:  fmt.Sprintf("coverage.accept entries for %s were not checked: %v", t, m.failed[t]),
		})
	}
	return findings
}

// reportAcceptedCoverageNotes prints how many coverage notes
// coverage.accept hides from sync. Silent when the project accepts none.
func reportAcceptedCoverageNotes(cmd *cobra.Command, cfg *config.Config, accepted int) {
	if len(cfg.Coverage.Accept) == 0 {
		return
	}
	cmd.Println()
	cmd.Println("Coverage notes:")
	cmd.Printf("  %d accepted (sync -v lists them)\n", accepted)
}
