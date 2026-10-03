package spec

import "testing"

func TestEnvRefEscape_IsNotAReference(t *testing.T) {
	value := "x-chroma-token: $${X_CHROMA_TOKEN}"
	if tokens := EnvRefTokens(value); len(tokens) != 0 {
		t.Errorf("EnvRefTokens(%q) = %+v, want none", value, tokens)
	}
	if HasEnvRef(value) || OnlyEnvRefs("$${X}") {
		t.Errorf("an escaped placeholder must not count as a reference")
	}
	if got := EnvRefDollarEnv.Write("$${X} ${Y}"); got != "$${X} ${env:Y}" {
		t.Errorf("Write = %q, want the escape kept and the reference rewritten", got)
	}
	if got, names := StripEnvRefDefaults("$${X:-a} ${Y:-b}"); got != "$${X:-a} ${Y}" || len(names) != 1 {
		t.Errorf("StripEnvRefDefaults = %q %v", got, names)
	}
}

func TestDecodeEnvRefEscapes_TurnsOnlyTheEscapeIntoLiteralText(t *testing.T) {
	for in, want := range map[string]string{
		"$${X}":           "${X}",
		"a $${X} b ${Y}":  "a ${X} b ${Y}",
		"pa$$word $$HOME": "pa$$word $$HOME",
		"$${X:-default}":  "${X:-default}",
		"no placeholder":  "no placeholder",
		"$$${X}":          "$${X}",
	} {
		if got := DecodeEnvRefEscapes(in); got != want {
			t.Errorf("DecodeEnvRefEscapes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEscapeEnvRefs_EscapesReferencesButNotToolVariables(t *testing.T) {
	in := "--h ${X} ${X:-d} ${workspaceFolder} ${input:token} ${env:Y} $${Z}"
	want := "--h $${X} $${X:-d} ${workspaceFolder} ${input:token} ${env:Y} $$${Z}"
	if got := EscapeEnvRefs(in); got != want {
		t.Errorf("EscapeEnvRefs = %q, want %q", got, want)
	}
	for _, native := range []string{"${X}", "$${X}", "a $${X} ${Y}"} {
		if got := DecodeEnvRefEscapes(EscapeEnvRefs(native)); got != native {
			t.Errorf("round trip of %q = %q", native, got)
		}
	}
}

func TestOnlyEscapedEnvRefs(t *testing.T) {
	for in, want := range map[string]bool{
		"$${X}":          true,
		"Bearer $${X}":   true,
		" $${X} $${Y} ":  true,
		"x-token: $${X}": false,
		"${X}":           false,
		"":               false,
	} {
		if got := OnlyEscapedEnvRefs(in); got != want {
			t.Errorf("OnlyEscapedEnvRefs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLaunchRefs_SkipsAnEscapedToolForm(t *testing.T) {
	if refs := EnvRefDollarEnv.LaunchRefs("$${env:X} ${env:Y}"); len(refs) != 1 || refs[0] != "${env:Y}" {
		t.Errorf("LaunchRefs = %v, want only ${env:Y}", refs)
	}
}

// OpenCode's `{env:NAME}` must not match the `{` left after a masked
// escape, or `$${env:X}` reads as a reference.
func TestEnvRefEscape_BraceEnvIgnoresAnEscape(t *testing.T) {
	if refs := EnvRefBraceEnv.LaunchRefs("$${env:X}"); len(refs) != 0 {
		t.Errorf("LaunchRefs = %q, want none", refs)
	}
	got := EnvRefBraceEnv.ReadLaunch(EscapeEnvRefs("$${env:X}"), EnvRefReading{})
	if got != "$$${env:X}" {
		t.Errorf("ReadLaunch(EscapeEnvRefs) = %q, want %q", got, "$$${env:X}")
	}
}

func TestEscapeEnvRefEscapes_KeepsANativeEscapeThroughDecode(t *testing.T) {
	for _, native := range []string{"$${X}", "a $${X} ${Y}", "${X}", "pa$$word"} {
		if got := DecodeEnvRefEscapes(EscapeEnvRefEscapes(native)); got != native {
			t.Errorf("round trip of %q = %q", native, got)
		}
	}
	if got := EscapeEnvRefEscapes("$${X} ${Y}"); got != "$$${X} ${Y}" {
		t.Errorf("EscapeEnvRefEscapes = %q, want only the escape doubled", got)
	}
}
