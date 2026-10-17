package authority

import "strings"

// declaredVerbs word a declared decision after "The specs".
var declaredVerbs = map[Effect]string{
	EffectAllowed: "permit it",
	EffectAsk:     "require approval for it",
	EffectDenied:  "block it",
}

// explanation words the harness decision for a person. An effect that
// holds only once the generated file is in sync reads conditionally.
func (bd *builder) explanation(d *declaration, eff effective, installed, file string, hypothetical bool) string {
	label := TargetLabel(bd.target)
	basis := basisSentence(d, eff, label)
	switch {
	case hypothetical:
		return conditionalSentence(eff.effect, installed, file, label) + " " + basis
	case eff.effect == EffectDefault:
		return basis
	case eff.effect == EffectUnresolved:
		return "Unresolved on " + label + ". " + basis
	}
	return effectLabels[eff.effect] + ". " + basis
}

// basisSentence says what decided the operation and how the harness
// carries it.
func basisSentence(d *declaration, eff effective, label string) string {
	switch {
	case eff.resolution == ResolutionNoRule:
		return "No rule applies; the tool's own default decides."
	case eff.resolution == ResolutionDisabled:
		return "The server's spec sets disabled: true; " + eff.note + "."
	case d.disabled:
		return "The server's spec sets disabled: true, but " + eff.note + "."
	case d.win == nil:
		return "No rule applies; the tool's own default decides."
	case d.win.kind == grantNarrowing:
		cause := eff.cause
		if cause == "" {
			cause = d.win.grant.Note
		}
		if eff.effect == EffectUnavailable {
			return cause + "; " + eff.note + "."
		}
		return cause + ", but " + eff.note + "."
	}
	phrase := grantPhrase(d.win)
	native := strings.Join(eff.native, ", ")
	switch eff.resolution {
	case ResolutionTranslated:
		text := "Decided by " + phrase + ", written for " + label + " as " + native + "."
		if eff.note != "" {
			text += " " + sentence(eff.note) + "."
		}
		return text
	case ResolutionHook:
		return "Decided by " + phrase + "; " + eff.note + "."
	case ResolutionTranslatedUnverified:
		return sentence(phrase) + " is written for " + label + " as " + native + ", but " + eff.note + "."
	}
	return "The specs " + declaredVerbs[d.declared] + " (" + phrase + "), but " + eff.note + "."
}

// grantPhrase names a settings declaration and where it is written.
func grantPhrase(c *candidate) string {
	if c.kind == grantProtected {
		return "protected path " + c.protect.path + " (decision: " + c.protect.decision + ") in " + c.protect.source.Path
	}
	return c.list + " " + c.rule.raw + " in " + c.rule.source.Path
}

// conditionalSentence words an effect that holds only once the file
// that carries it is in sync.
func conditionalSentence(effect Effect, installed, file, label string) string {
	subject, verb, pronoun, state := file, "is", "it", "the file on disk differs"
	if file == "" {
		subject, verb, pronoun, state = "the generated files for "+label, "are", "them", "the files on disk differ"
	}
	start := "Would be " + conditionalPhrases[effect] + " once " + subject + " " + verb
	switch installed {
	case InstalledStale:
		return start + " synced; " + state + " from what sync writes."
	case InstalledNotGenerated:
		return start + " synced; sync has not written " + pronoun + " yet."
	}
	return start + " in sync; nothing was compared with disk."
}
