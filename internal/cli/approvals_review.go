package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
	// legacyKey is an older key the tool copies into key when key is
	// unset, so a value there is the user's mode too.
	legacyKey string
	// off is a value in the same file that keeps the reviewer from running.
	off *reviewerOff
	// overrideEnv names a variable that makes the tool ignore the key in
	// the file sync --global writes.
	overrideEnv string
	// reviewNote is a fixed limit status names whenever the mode is review.
	reviewNote string
	// toolDefault is the value the tool writes itself when nobody chose
	// one, so review may replace it as if the key were unset.
	toolDefault any
}

type reviewerOff struct {
	key   string
	value any
	why   string
}

// nativeReviewers carry out review, each through a key in the user
// settings file sync --global owns. Claude Code honors auto mode only from
// user, policy, or flag settings.
var nativeReviewers = map[string]nativeReviewer{
	"claude": {name: "Claude Code auto mode", key: "permissions.defaultMode", value: "auto"},
	"codebuddy": {name: "CodeBuddy auto mode", key: "permissions.defaultMode", value: "auto", overrideEnv: "CODEBUDDY_CONFIG_DIR",
		off: &reviewerOff{key: "permissions.disableAutoMode", value: "disable", why: "CodeBuddy starts in default instead"}},
	"codex": {name: "Codex Auto-review", key: "approvals_reviewer", value: "auto_review",
		off: &reviewerOff{key: "approval_policy", value: "never", why: "it reviews only prompts, so use on-request or a granular policy"}},
	"windsurf": {name: "Devin smart mode", key: "permission_mode", value: "smart", overrideEnv: "DEVIN_PERMISSION_MODE",
		off: &reviewerOff{key: "permission_mode", value: "dangerous", why: "Devin approves every call instead"}},
	"vibe": {name: "Vibe smart-approve", key: "default_agent", value: "smart-approve", overrideEnv: "VIBE_DEFAULT_AGENT",
		off: &reviewerOff{key: "bypass_tool_permissions", value: true, why: "Vibe approves every call instead"}},
	"cursor": {name: "cursor-agent Auto-review", key: "approvalMode", value: "auto-review",
		toolDefault: "allowlist",
		reviewNote:  "the Cursor desktop app keeps its own approval setting; turn on Auto-review there too"},
	"qoder": {name: "Qoder auto mode", key: "general.defaultPermissionMode", value: "auto", legacyKey: "tools.approvalMode"},
	"qwen":  {name: "Qwen Code auto mode", key: "tools.approvalMode", value: "auto", defaultOn: "in a trusted folder", askValue: "default"},
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
	if !slices.ContainsFunc(p.targets, func(target string) bool { _, ok := nativeReviewers[target]; return ok || target == "kiro" }) {
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
func (p *globalPlan) approvalsReviewSetting(target, path, format string, want []globalSetting, previous map[string]any) ([]globalSetting, map[string]any, error) {
	reviewer, ok := nativeReviewers[target]
	mode, from := homeToolMode(p.approvals, target)
	if !ok || mode != config.ApprovalsReview {
		return want, previous, nil
	}
	for _, s := range want {
		if settingsKeysOverlap(s.key, reviewer.key, format) || (reviewer.legacyKey != "" && settingsKeysOverlap(s.key, reviewer.legacyKey, format)) {
			return want, previous, nil
		}
	}
	data := p.baseFor(path)
	if data == nil {
		read, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("read %s: %w", path, err)
		}
		data = read
	}
	current, err := settingsValues(path, format, data)
	if err != nil {
		return nil, nil, err
	}
	have, present := settingsLookup(current, reviewer.key, format)
	recorded, owned := previous[reviewer.key]
	if present && !owned && (have == nil || reviewer.toolDefault != nil && sameSetting(have, reviewer.toolDefault)) {
		// Record the tool's own default as ours, so the merge replaces it
		// and removing review hands the key back to the tool.
		previous = maps.Clone(previous)
		if previous == nil {
			previous = map[string]any{}
		}
		previous[reviewer.key] = have
		recorded, owned = have, true
	}
	if legacy, ok := settingsLookup(current, reviewer.legacyKey, format); reviewer.legacyKey != "" && ok && !present {
		_, err := fmt.Fprintf(p.warn, "warning: %s: %s: review leaves %s = %s in %s, set outside agnostic-ai; %s stays off until you remove it\n",
			target, from, reviewer.legacyKey, settingsValueText(reviewer.legacyKey, legacy), path, reviewer.name)
		if err != nil {
			return nil, nil, fmt.Errorf("write approvals warning: %w", err)
		}
		return want, previous, nil
	}
	if present && (!owned || !sameSetting(have, recorded)) {
		if sameSetting(have, reviewer.value) {
			return want, previous, nil
		}
		_, err := fmt.Fprintf(p.warn, "warning: %s: %s: review leaves %s = %s in %s, set outside agnostic-ai; %s stays off until you remove it\n",
			target, from, reviewer.key, settingsValueText(reviewer.key, have), path, reviewer.name)
		if err != nil {
			return nil, nil, fmt.Errorf("write approvals warning: %w", err)
		}
		return want, previous, nil
	}
	return append(want, globalSetting{target: target, key: reviewer.key, value: reviewer.value, field: from, source: filepath.Join(p.source, config.ConfigFileName)}), previous, nil
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
	rs.present = rs.present && rs.have != nil
	rs.on = sameSetting(rs.have, reviewer.value) || (!rs.present && reviewer.defaultOn != "")
	recorded, owned := state.Settings[tool][reviewer.key]
	rs.written = rs.on && owned && sameSetting(recorded, rs.have)
	return rs, true, nil
}

// notes say what keeps the reviewer from matching mode: the key missing
// or changed under review, the key still on where the mode is not review,
// a value that switches the reviewer off, or a variable that overrides the key.
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
	case mode != config.ApprovalsReview && rs.on && tool != "claude":
		// Auto mode is Claude Code's own starting mode, so a value set by
		// hand there is not worth a note.
		notes = append(notes, fmt.Sprintf("%s is on: %s = %s in %s was set outside agnostic-ai", r.name, r.key, want, rs.path))
	}
	if mode == config.ApprovalsReview && r.reviewNote != "" {
		notes = append(notes, r.reviewNote)
	}
	if mode != config.ApprovalsReview && !rs.on {
		return notes
	}
	format := globalTargets[tool].settings.format
	if off := r.off; off != nil {
		if have, ok := settingsLookup(rs.values, off.key, format); ok && sameSetting(have, off.value) {
			notes = append(notes, fmt.Sprintf("%s = %s in %s keeps %s from running; %s", off.key, settingsValueText(off.key, off.value), rs.path, r.name, off.why))
		}
	}
	if r.overrideEnv != "" && strings.TrimSpace(os.Getenv(r.overrideEnv)) != "" {
		notes = append(notes, fmt.Sprintf("%s is set, so %s does not take %s from %s", r.overrideEnv, tool, r.key, rs.path))
	}
	return notes
}
