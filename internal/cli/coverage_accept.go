package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// matchCoverageAccept emits every target in memory and matches the
// coverage notes it raises against coverage.accept. It returns the
// accepted notes and the entries that matched none. The policy is forced
// to warn so a note that on-unsupported: error turns into a failure is
// still raised and can be matched.
func matchCoverageAccept(cfg *config.Config, b spec.Bundle, targets []string) ([]adapters.AcceptedNote, []config.CoverageAccept, error) {
	if len(cfg.Coverage.Accept) == 0 {
		return nil, nil, nil
	}
	restore := adapters.SetAsideNotes()
	defer restore()
	view := *cfg
	view.OnUnsupported = "warn"
	sess := adapters.NewSession()
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		if _, err := captureAdapterFiles(sess, adapter, b, &view); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", t, err)
		}
	}
	adapters.OrderBufferedDropsByTarget(targets)
	accepted, unmatched := adapters.AcceptCoverageNotes(cfg.Coverage.Accept)
	return accepted, unmatched, nil
}

// lintCoverageAccept flags a coverage.accept entry that matches no note
// (LINT022, warn), so an entry goes stale visibly once a target starts
// supporting the field.
func lintCoverageAccept(cfg *config.Config, b spec.Bundle, targets []string) ([]lintFinding, error) {
	_, unmatched, err := matchCoverageAccept(cfg, b, targets)
	if err != nil {
		return nil, err
	}
	var findings []lintFinding
	for _, a := range unmatched {
		findings = append(findings, lintFinding{
			Code:     "LINT022",
			Severity: lintWarn,
			Path:     config.ConfigFileName,
			Message:  fmt.Sprintf("coverage.accept entry %s matches no coverage note; remove it", a),
		})
	}
	return findings, nil
}

// reportAcceptedCoverageNotes prints how many coverage notes
// coverage.accept hides from sync. Silent when the project accepts none.
func reportAcceptedCoverageNotes(cmd *cobra.Command, scope checkScope) error {
	if len(scope.cfg.Coverage.Accept) == 0 {
		return nil
	}
	accepted, _, err := matchCoverageAccept(scope.cfg, scope.bundle, scope.targets)
	if err != nil {
		return err
	}
	cmd.Println()
	cmd.Println("Coverage notes:")
	cmd.Printf("  %d accepted (sync -v lists them)\n", len(accepted))
	return nil
}
