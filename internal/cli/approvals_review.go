package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// nativeReviewer is the user settings key that hands a tool's permission
// prompts to its own reviewer model.
type nativeReviewer struct {
	name, key string
	value     any
	// defaultOn says where the tool runs the reviewer with the key unset;
	// askValue is the value that shows its prompts instead.
	defaultOn string
	askValue  any
}

// nativeReviewers carry out review, each through a key in the user
// settings file sync --global owns. Claude Code honors auto mode only from
// user, policy, or flag settings.
var nativeReviewers = map[string]nativeReviewer{
	"claude": {name: approvals.RouteFor("claude").Review.Name, key: "permissions.defaultMode", value: "auto"},
	"codex":  {name: approvals.RouteFor("codex").Review.Name, key: "approvals_reviewer", value: "auto_review"},
	"qwen":   {name: approvals.RouteFor("qwen").Review.Name, key: "tools.approvalMode", value: "auto", defaultOn: "in a trusted folder", askValue: "default"},
}

// homeToolMode is the mode the home config gives tool everywhere, which
// is all a user settings key can follow: projects and the environment
// change it per folder or session.
func homeToolMode(home config.ApprovalsConfig, tool string) (mode, from string) {
	if mode := home.Tools[tool]; mode != "" {
		return mode, "approvals.tools." + tool
	}
	return home.Mode, "approvals.mode"
}

// loadPlanApprovals reads the home approvals block when a target with a
// native reviewer is selected. A block that does not load writes no
// reviewer key, with a warning: approvals then fall back to ask, and the
// other surfaces still sync.
func (p *globalPlan) loadPlanApprovals() error {
	if !slices.ContainsFunc(p.targets, func(target string) bool { _, ok := nativeReviewers[target]; return ok }) {
		return nil
	}
	approvals, err := config.LoadHomeApprovals(p.source)
	if err == nil {
		p.approvals = approvals
		return nil
	}
	if _, werr := fmt.Fprintf(p.warn, "warning: %v; sync --global writes no native reviewer key\n", err); werr != nil {
		return fmt.Errorf("write approvals warning: %w", werr)
	}
	return nil
}

// approvalsReviewSetting adds the native reviewer key to want when the
// home config sets review for target. A settings spec that sets the key
// wins, and a value set outside agnostic-ai stays unowned: a different
// one with a warning, the same one so removing review keeps it.
func (p *globalPlan) approvalsReviewSetting(target, path, format string, want []globalSetting, previous map[string]any) ([]globalSetting, error) {
	reviewer, ok := nativeReviewers[target]
	mode, from := homeToolMode(p.approvals, target)
	if !ok || mode != config.ApprovalsReview {
		return want, nil
	}
	for _, s := range want {
		if settingsKeysOverlap(s.key, reviewer.key, format) {
			return want, nil
		}
	}
	data := p.baseFor(path)
	if data == nil {
		read, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
	}
	current, err := settingsValues(path, format, data)
	if err != nil {
		return nil, err
	}
	have, present := settingsLookup(current, reviewer.key, format)
	recorded, owned := previous[reviewer.key]
	if present && (!owned || !sameSetting(have, recorded)) {
		if sameSetting(have, reviewer.value) {
			return want, nil
		}
		_, err := fmt.Fprintf(p.warn, "warning: %s: %s: review leaves %s = %s in %s, set outside agnostic-ai; %s stays off until you remove it\n",
			target, from, reviewer.key, settingsValueText(reviewer.key, have), path, reviewer.name)
		if err != nil {
			return nil, fmt.Errorf("write approvals warning: %w", err)
		}
		return want, nil
	}
	return append(want, globalSetting{target: target, key: reviewer.key, value: reviewer.value, field: from, source: filepath.Join(p.source, config.ConfigFileName)}), nil
}

// reviewerState is a native reviewer key as the user settings file holds
// it, and whether sync --global wrote it there.
type reviewerState struct {
	reviewer             nativeReviewer
	path                 string
	values               map[string]any
	have                 any
	present, on, written bool
}

func readReviewerState(tool, userHome string, state globalState) (reviewerState, bool, error) {
	reviewer, ok := nativeReviewers[tool]
	if !ok {
		return reviewerState{}, false, nil
	}
	g := globalTargets[tool]
	rs := reviewerState{reviewer: reviewer, path: g.path(userHome, g.settings.path)}
	data, err := os.ReadFile(rs.path)
	switch {
	case os.IsNotExist(err):
		rs.values = map[string]any{}
	case err != nil:
		return rs, true, fmt.Errorf("read %s: %w", rs.path, err)
	default:
		if rs.values, err = settingsValues(rs.path, g.settings.format, data); err != nil {
			return rs, true, err
		}
	}
	rs.have, rs.present = settingsLookup(rs.values, reviewer.key, g.settings.format)
	rs.on = sameSetting(rs.have, reviewer.value) || (!rs.present && reviewer.defaultOn != "")
	recorded, owned := state.Settings[tool][reviewer.key]
	rs.written = rs.on && owned && sameSetting(recorded, rs.have)
	return rs, true, nil
}

// notes say what keeps the reviewer from matching mode: the key missing
// or changed under review, a Codex approval policy that never asks, or
// the key still on where the mode is not review.
func (rs reviewerState) notes(tool, mode, homeMode string) []string {
	r := rs.reviewer
	want := settingsValueText(r.key, r.value)
	var notes []string
	switch {
	case mode == config.ApprovalsReview && !rs.present && r.defaultOn == "":
		notes = append(notes, fmt.Sprintf("%s is off: %s is not set in %s; run sync --global to write %s", r.name, r.key, rs.path, want))
	case mode == config.ApprovalsReview && !rs.on:
		notes = append(notes, fmt.Sprintf("%s is off: %s is %s in %s, not %s", r.name, r.key, settingsValueText(r.key, rs.have), rs.path, want))
	case mode != config.ApprovalsReview && rs.written && homeMode == config.ApprovalsReview:
		notes = append(notes, fmt.Sprintf("%s stays on here: sync --global wrote %s = %s to %s, which every folder reads", r.name, r.key, want, rs.path))
	case mode != config.ApprovalsReview && rs.written:
		notes = append(notes, fmt.Sprintf("%s stays on from %s until sync --global removes %s", r.name, rs.path, r.key))
	case mode != config.ApprovalsReview && rs.on && !rs.present:
		notes = append(notes, fmt.Sprintf("%s is on %s while %s is unset in %s; to see prompts, set x-%s.%s to %s in a home settings spec",
			r.name, r.defaultOn, r.key, rs.path, tool, r.key, settingsValueText(r.key, r.askValue)))
	case mode != config.ApprovalsReview && rs.on && tool == "codex":
		// Auto mode is Claude Code's own starting mode, so a value set by
		// hand there is not worth a note.
		notes = append(notes, fmt.Sprintf("%s is on: %s = %s in %s was set outside agnostic-ai", r.name, r.key, want, rs.path))
	}
	if tool == "codex" && (mode == config.ApprovalsReview || rs.on) {
		if policy, _ := rs.values["approval_policy"].(string); policy == "never" {
			notes = append(notes, fmt.Sprintf(`approval_policy = "never" in %s keeps %s from running; it reviews only prompts, so use on-request or a granular policy`, rs.path, r.name))
		}
	}
	return notes
}
