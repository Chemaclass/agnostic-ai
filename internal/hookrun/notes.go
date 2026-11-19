package hookrun

// ResultNotes is what one command's result adds to a target's run:
// the notes to print, in order, and why the result is not counted (""
// when it is).
func ResultNotes(target, event string, r Result) (notes []string, uncounted string) {
	switch target {
	case "antigravity":
		return nonEmptyNotes(AntigravityNote(event, r)), AntigravityUncounted(event, r)
	case "kiro":
		return nonEmptyNotes(KiroNote(event, r)), KiroUncounted(event, r)
	case "windsurf":
		return nonEmptyNotes(WindsurfNote(event, r)), WindsurfUncounted(event, r)
	case "augment":
		return nonEmptyNotes(AugmentNote(event, r)), ""
	case "cline":
		return ClineNotes(event, r), ""
	case "cursor":
		if CursorAsks(event, r) {
			notes = append(notes, "replied ask: Cursor asks the user before the action runs; read as block")
		}
	case "crush":
		if CrushHalts(r) {
			notes = append(notes, "halt: Crush ends the whole turn, not only this tool call")
		}
	case "qoder":
		if QoderAsks(event, r) {
			notes = append(notes, "replied ask: Qoder asks the user before the tool runs; read as block")
		}
	case "copilot":
		if CopilotAsks(event, r) {
			notes = append(notes, "replied ask: Copilot CLI asks the user before the tool runs, and cloud agent denies it; read as block")
		}
		if CopilotErrored(event, r) {
			notes = append(notes, "the hook failed, and Copilot denies a preToolUse hook that fails; read as block")
		}
	}
	return notes, ""
}

// AdjustDecision applies the rules where a target reads one command's
// result apart from DecideHandler. It returns the decision, the notes
// that explain it, which come before ResultNotes, and whether the result
// also counts as a failed run.
func AdjustDecision(target, event string, body []byte, d Decision, r Result) (Decision, []string, bool) {
	switch target {
	case "qoder":
		if d == Block && QoderPolicyChange(event, body) {
			return Error, []string{"Qoder enforces a policy_settings change; the hook runs for audit and cannot block it"}, false
		}
	case "factory":
		if d == Allow && FactoryAsks(event, r) {
			return d, []string{"replied ask: Factory's normal permissions decide, so a call they allow runs without a prompt; read as allow"}, false
		}
	case "copilot":
		return d, nil, CopilotErrored(event, r)
	}
	return d, nil, false
}

func nonEmptyNotes(note string) []string {
	if note == "" {
		return nil
	}
	return []string{note}
}
