package cli

import (
	"fmt"
	"os"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// kiroReviewerName is the route name of Kiro's reviewer, which a preset
// in the user permissions file turns on rather than a settings key.
const kiroReviewerName = "Kiro Approve for me"

// kiroPermissionsFile is Kiro's user-scoped permissions file, which the
// user and Kiro's own /permissions page also write.
const kiroPermissionsFile = globalPathHome + ".kiro/settings/permissions.yaml"

// globalReviewPolicy is a reviewer preset sync listed in a permissions
// file, and what else it added there.
type globalReviewPolicy struct {
	Path string `json:"path"`
	approvals.KiroPolicyOwned
}

func kiroPermissionsPath(home string) string {
	return globalTargets["kiro"].path(home, kiroPermissionsFile)
}

// planKiroReview lists approve-for-me in Kiro's user permissions file
// while the home config sets review for Kiro, and takes out only the
// item it listed once review goes. A preset the user listed stays
// theirs, and a file it cannot edit in place gets a warning instead.
func (p *globalPlan) planKiroReview(target string, _ globalTarget) error {
	if target != "kiro" {
		return nil
	}
	path := kiroPermissionsPath(p.home)
	recorded, owned := p.old.ReviewPolicies[target]
	mode, from := homeToolMode(p.approvals, target)
	if owned && (mode != config.ApprovalsReview || recorded.Path != path) {
		if err := p.removeKiroPolicy(target, recorded); err != nil {
			return err
		}
		owned = false
	}
	if mode != config.ApprovalsReview {
		return nil
	}
	data, err := p.kiroPermissionsData(path)
	if err != nil {
		return err
	}
	add, err := approvals.AddKiroPolicy(path, data, approvals.KiroReviewPreset)
	if err != nil {
		return p.kiroReviewWarning(target, from, err)
	}
	switch {
	case !add.Changed && owned:
		p.next.ReviewPolicies[target] = recorded
	case add.Changed:
		p.next.ReviewPolicies[target] = globalReviewPolicy{Path: path, KiroPolicyOwned: add.Owned}
		return p.placeEdit(path, globalSettingsMerge{data: add.Data, changes: []string{"policies"}}, 0o644)
	}
	return nil
}

func (p *globalPlan) removeKiroPolicy(target string, recorded globalReviewPolicy) error {
	data, err := p.kiroPermissionsData(recorded.Path)
	if err != nil {
		return err
	}
	out, err := approvals.RemoveKiroPolicy(recorded.Path, data, approvals.KiroReviewPreset, recorded.KiroPolicyOwned)
	if err != nil {
		// Keep the record, so a later run takes the item out once it can.
		p.next.ReviewPolicies[target] = recorded
		_, from := homeToolMode(p.approvals, target)
		return p.kiroReviewWarning(target, from, err)
	}
	if string(out) == string(data) {
		return nil
	}
	return p.placeEdit(recorded.Path, globalSettingsMerge{data: out, changes: []string{"policies"}}, 0o644)
}

// kiroPermissionsData is the file as this run planned it, or as it is
// on disk, empty when it does not exist.
func (p *globalPlan) kiroPermissionsData(path string) ([]byte, error) {
	if data := p.baseFor(path); data != nil {
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

func (p *globalPlan) kiroReviewWarning(target, from string, cause error) error {
	if _, err := fmt.Fprintf(p.warn, "warning: %s: %s: sync --global leaves %s alone (%v); list or remove %s under policies by hand\n",
		target, from, kiroPermissionsFile, cause, approvals.KiroReviewPreset); err != nil {
		return fmt.Errorf("write approvals warning: %w", err)
	}
	return nil
}

// kiroReviewerOn reports whether Kiro's user permissions file lists
// approve-for-me, the only place a user-wide choice can live.
func kiroReviewerOn(home string) (bool, string, error) {
	path := kiroPermissionsPath(home)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, path, nil
	}
	if err != nil {
		return false, path, fmt.Errorf("read %s: %w", path, err)
	}
	on, err := approvals.KiroListsPolicy(path, data, approvals.KiroReviewPreset)
	return on, path, err
}

// kiroReviewStatus notes whether Kiro's user permissions file matches
// mode, and falls back to ask when review finds the preset missing. It
// reports whether a preset sync listed is still there.
func kiroReviewStatus(status *approvalsToolStatus, home, mode string, state globalState) bool {
	on, path, err := kiroReviewerOn(home)
	if err != nil {
		status.Notes = append(status.Notes, err.Error())
	}
	recorded, owned := state.ReviewPolicies["kiro"]
	written := on && owned && recorded.Path == path
	switch {
	case mode == config.ApprovalsReview && !on:
		status.Notes = append(status.Notes, fmt.Sprintf("%s is off: %s is not listed under policies in %s; run sync --global to list it", kiroReviewerName, approvals.KiroReviewPreset, path))
		if status.Effective == config.ApprovalsReview {
			status.Effective, status.Route = config.ApprovalsAsk, ""
			status.Reason = "the reviewer is not on in the tool's user settings"
		}
	case mode == config.ApprovalsReview:
		status.Notes = append(status.Notes, "Kiro asks anyway when its trust classifier is unavailable, or in a session whose /permissions page does not offer approve for me")
	case on && written:
		status.Notes = append(status.Notes, fmt.Sprintf("%s stays on from %s until sync --global removes %s", kiroReviewerName, path, approvals.KiroReviewPreset))
	case on:
		status.Notes = append(status.Notes, fmt.Sprintf("%s stays on: %s lists %s, set outside agnostic-ai", kiroReviewerName, path, approvals.KiroReviewPreset))
	}
	return written
}
