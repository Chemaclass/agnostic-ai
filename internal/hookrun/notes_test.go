package hookrun

import (
	"slices"
	"testing"
)

func TestResultNotes_PerTarget(t *testing.T) {
	antigravity := Result{Stdout: `{"decision":"force_ask"}`}
	kiro := Result{Exit: 1}
	kiroStop := Result{Stdout: `{"decision":"block"}`}
	windsurf := Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"rtk git status"}}}`}
	augment := Result{Stdout: `{"decision":"block"}`}
	cline := Result{Exit: 1}
	for name, tc := range map[string]struct {
		target, event string
		r             Result
		notes         []string
		uncounted     string
	}{
		"antigravity":        {"antigravity", "PreToolUse", antigravity, []string{AntigravityNote("PreToolUse", antigravity)}, AntigravityUncounted("PreToolUse", antigravity)},
		"kiro uncounted":     {"kiro", "PreToolUse", kiro, nil, "Kiro's docs disagree on whether a non-zero exit other than 2 blocks"},
		"kiro note":          {"kiro", "Stop", kiroStop, []string{KiroNote("Stop", kiroStop)}, ""},
		"windsurf":           {"windsurf", "PreToolUse", windsurf, []string{WindsurfNote("PreToolUse", windsurf)}, ""},
		"windsurf uncounted": {"windsurf", "PostToolUse", Result{Exit: 2}, nil, "Devin CLI does not document what a block does on PostToolUse"},
		"augment":            {"augment", "SessionStart", augment, []string{"replied block: Auggie runs SessionStart hooks in the background and cannot block them; read as allow"}, ""},
		"cline":              {"cline", "PreToolUse", cline, ClineNotes("PreToolUse", cline), ""},
		"cursor ask":         {"cursor", "beforeShellExecution", Result{Stdout: `{"permission":"ask"}`}, []string{"replied ask: Cursor asks the user before the action runs; read as block"}, ""},
		"crush halt":         {"crush", "PreToolUse", Result{Exit: crushHaltExit}, []string{"halt: Crush ends the whole turn, not only this tool call"}, ""},
		"qoder ask":          {"qoder", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask"}}`}, []string{"replied ask: Qoder asks the user before the tool runs; read as block"}, ""},
		"copilot ask":        {"copilot", "preToolUse", Result{Stdout: `{"permissionDecision":"ask"}`}, []string{"replied ask: Copilot CLI asks the user before the tool runs, and cloud agent denies it; read as block"}, ""},
		"copilot failed":     {"copilot", "preToolUse", Result{Exit: 1}, []string{"the hook failed, and Copilot denies a preToolUse hook that fails; read as block"}, ""},
		"factory has none":   {"factory", "PreToolUse", Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"ask"}}`}, nil, ""},
		"claude has none":    {"claude", "PreToolUse", Result{Exit: 1}, nil, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if len(tc.notes) > 0 && tc.notes[0] == "" {
				t.Fatal("the fixture must produce a note")
			}
			notes, uncounted := ResultNotes(tc.target, tc.event, tc.r)
			if !slices.Equal(notes, tc.notes) {
				t.Errorf("notes = %q, want %q", notes, tc.notes)
			}
			if uncounted != tc.uncounted {
				t.Errorf("uncounted = %q, want %q", uncounted, tc.uncounted)
			}
		})
	}
}

func TestAdjustDecision_PerTarget(t *testing.T) {
	policy := []byte(`{"source":"policy_settings"}`)
	ask := Result{Stdout: `{"hookSpecificOutput":{"permissionDecision":"ask"}}`}
	for name, tc := range map[string]struct {
		target, event string
		body          []byte
		d             Decision
		r             Result
		want          Decision
		notes         []string
		failed        bool
	}{
		"qoder policy change cannot block": {"qoder", "ConfigChange", policy, Block, Result{Exit: 2}, Error, []string{"Qoder enforces a policy_settings change; the hook runs for audit and cannot block it"}, false},
		"qoder other source blocks":        {"qoder", "ConfigChange", []byte(`{"source":"user_settings"}`), Block, Result{Exit: 2}, Block, nil, false},
		"factory ask on allow":             {"factory", "PreToolUse", nil, Allow, ask, Allow, []string{"replied ask: Factory's normal permissions decide, so a call they allow runs without a prompt; read as allow"}, false},
		"factory ask on block":             {"factory", "PreToolUse", nil, Block, ask, Block, nil, false},
		"copilot failure":                  {"copilot", "preToolUse", nil, Block, Result{Exit: 1}, Block, nil, true},
		"copilot exit 2":                   {"copilot", "preToolUse", nil, Block, Result{Exit: 2}, Block, nil, false},
		"other targets keep the decision":  {"claude", "PreToolUse", nil, Error, Result{Exit: 1}, Error, nil, false},
	} {
		t.Run(name, func(t *testing.T) {
			d, notes, failed := AdjustDecision(tc.target, tc.event, tc.body, tc.d, tc.r)
			if d != tc.want || !slices.Equal(notes, tc.notes) || failed != tc.failed {
				t.Errorf("= %s, %q, %v; want %s, %q, %v", d, notes, failed, tc.want, tc.notes, tc.failed)
			}
		})
	}
}
