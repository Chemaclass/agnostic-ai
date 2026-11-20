package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// nativeReviewer is the user settings key that hands a tool's permission
// prompts to its own reviewer model.
type nativeReviewer struct {
	name, key string
	value     any
}

// nativeReviewers carry out review. Claude Code honors auto mode only
// from user, policy, or flag settings, and Codex reads approvals_reviewer
// from config.toml, so both live in user files that sync --global owns.
var nativeReviewers = map[string]nativeReviewer{
	"claude": {name: "Claude Code auto mode", key: "permissions.defaultMode", value: "auto"},
	"codex":  {name: "Codex Auto-review", key: "approvals_reviewer", value: "auto_review"},
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

// loadPlanApprovals reads the home approvals block for a global plan.
func loadPlanApprovals(source string) (config.ApprovalsConfig, error) {
	abs, err := filepath.Abs(source)
	if err != nil {
		return config.ApprovalsConfig{}, fmt.Errorf("resolve %s: %w", source, err)
	}
	return config.LoadHomeApprovals(abs)
}

// approvalsReviewSetting adds the native reviewer key to want when the
// home config sets review for target. A settings spec that sets the key
// wins, and a value set outside agnostic-ai stays, with a warning.
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
	if present && !sameSetting(have, reviewer.value) && (!owned || !sameSetting(have, recorded)) {
		_, err := fmt.Fprintf(p.warn, "warning: %s: %s: review leaves %s = %s in %s, set outside agnostic-ai; %s stays off until you remove it\n",
			target, from, reviewer.key, settingsValueText(reviewer.key, have), path, reviewer.name)
		if err != nil {
			return nil, fmt.Errorf("write approvals warning: %w", err)
		}
		return want, nil
	}
	return append(want, globalSetting{target: target, key: reviewer.key, value: reviewer.value, field: from, source: filepath.Join(p.source, config.ConfigFileName)}), nil
}

// reviewerNotes says what keeps a tool's native reviewer from matching
// the mode status resolved: the key missing or changed under review, a
// Codex approval policy that never asks, or the key still on where the
// mode is not review.
func reviewerNotes(tool string, decision config.ApprovalDecision, home config.ApprovalsConfig, userHome string) []string {
	reviewer, ok := nativeReviewers[tool]
	if !ok {
		return nil
	}
	g := globalTargets[tool]
	path := g.path(userHome, g.settings.path)
	values, err := readSettingsValues(path, g.settings.format)
	if err != nil {
		return []string{err.Error()}
	}
	have, present := settingsLookup(values, reviewer.key, g.settings.format)
	on := present && sameSetting(have, reviewer.value)
	want := settingsValueText(reviewer.key, reviewer.value)
	homeMode, _ := homeToolMode(home, tool)
	var notes []string
	switch {
	case decision.Mode == config.ApprovalsReview && !present:
		notes = append(notes, fmt.Sprintf("%s is off: %s is not set in %s; run sync --global to write %s", reviewer.name, reviewer.key, path, want))
	case decision.Mode == config.ApprovalsReview && !on:
		notes = append(notes, fmt.Sprintf("%s is off: %s is %s in %s, not %s", reviewer.name, reviewer.key, settingsValueText(reviewer.key, have), path, want))
	case decision.Mode != config.ApprovalsReview && on && (tool == "codex" || homeMode == config.ApprovalsReview):
		// Auto mode is Claude Code's own starting mode, so only the value
		// review wrote is worth a note.
		notes = append(notes, fmt.Sprintf("%s stays on from %s (%s = %s) until sync --global removes it or you change it", reviewer.name, path, reviewer.key, want))
	}
	if tool == "codex" && (decision.Mode == config.ApprovalsReview || on) {
		if policy, _ := values["approval_policy"].(string); policy == "never" {
			notes = append(notes, fmt.Sprintf(`approval_policy = "never" in %s keeps %s from running; it reviews only prompts, so use on-request or a granular policy`, path, reviewer.name))
		}
	}
	return notes
}

func readSettingsValues(path, format string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return settingsValues(path, format, data)
}
