package spec

import (
	"slices"
	"testing"
)

func TestEnvRefSyntax_WriteAndReadBack(t *testing.T) {
	for _, tc := range []struct {
		syntax EnvRefSyntax
		native string
	}{
		{EnvRefDollar, "Bearer ${API_KEY}"},
		{EnvRefDollarEnv, "Bearer ${env:API_KEY}"},
		{EnvRefBraceEnv, "Bearer {env:API_KEY}"},
	} {
		if got := tc.syntax.Write("Bearer ${API_KEY}"); got != tc.native {
			t.Errorf("Write = %q, want %q", got, tc.native)
		}
		if got := tc.syntax.Read(tc.native, EnvRefReading{}); got != "Bearer ${API_KEY}" {
			t.Errorf("Read(%q) = %q", tc.native, got)
		}
	}
}

func TestEnvRefSyntax_WriteLeavesOtherTokens(t *testing.T) {
	if got := EnvRefDollarEnv.Write("${A:-x} ${input:b} ${C}"); got != "${A:-x} ${input:b} ${env:C}" {
		t.Errorf("Write = %q", got)
	}
}

func TestEnvRefSyntax_ReadsOnlyTheFormsAToolExpands(t *testing.T) {
	if got := EnvRefDollar.Read("$TOKEN", EnvRefReading{Unbraced: true}); got != "${TOKEN}" {
		t.Errorf("unbraced read = %q", got)
	}
	for _, literal := range []string{"Bearer $TOKEN", "pa55$word", "a%B%c"} {
		if got := EnvRefDollar.Read(literal, EnvRefReading{Unbraced: true, Percent: true}); got != literal {
			t.Errorf("only a whole value reads back: %q became %q", literal, got)
		}
	}
	if got := EnvRefDollar.Read("%TOKEN%", EnvRefReading{Percent: true}); got != "${TOKEN}" {
		t.Errorf("percent read = %q", got)
	}
	if got := EnvRefDollar.Read("$TOKEN %TOKEN%", EnvRefReading{}); got != "$TOKEN %TOKEN%" {
		t.Errorf("a tool without those forms must keep them: %q", got)
	}
}

func TestEnvRefTokens_ClassifiesEveryToken(t *testing.T) {
	tokens := EnvRefTokens("${A}:${B_2:-x} ${env:C} ${D:-}")
	if len(tokens) != 4 {
		t.Fatalf("tokens = %v", tokens)
	}
	if tokens[0].Name != "A" || tokens[0].HasDefault {
		t.Errorf("plain token = %+v", tokens[0])
	}
	if tokens[1].Name != "B_2" || tokens[1].Default != "x" || tokens[1].Display() != "${B_2:-...}" {
		t.Errorf("default token = %+v", tokens[1])
	}
	if tokens[2].Known() || tokens[2].Display() != "${env:C}" {
		t.Errorf("unknown token = %+v", tokens[2])
	}
	if name, ok := WholeEnvRef("${TOKEN}"); !ok || name != "TOKEN" {
		t.Errorf("WholeEnvRef = %q, %v", name, ok)
	}
	for _, v := range []string{"Bearer ${TOKEN}", "${TOKEN:-x}", "${env:TOKEN}"} {
		if _, ok := WholeEnvRef(v); ok {
			t.Errorf("WholeEnvRef(%q) must be false", v)
		}
	}
	if !HasEnvRef("${TOKEN:-dev}") || HasEnvRef("ghp_example") {
		t.Error("HasEnvRef must accept any ${...} and reject a literal")
	}
}

func TestOnlyEnvRefs(t *testing.T) {
	for value, want := range map[string]bool{
		"${A}":                         true,
		"Bearer ${A}":                  true,
		"${A:-x} ${B}":                 true,
		"${A} ${input:b}":              false,
		"postgres://u:hunter2@${HOST}": false,
		"Bearer sk-1 ${EXTRA}":         false,
		"literal":                      false,
	} {
		if got := OnlyEnvRefs(value); got != want {
			t.Errorf("OnlyEnvRefs(%q) = %v, want %v", value, got, want)
		}
	}
}

func TestStripEnvRefDefaults(t *testing.T) {
	got, names := StripEnvRefDefaults("${A:-sk-live} ${B:-} ${C}")
	if got != "${A} ${B:-} ${C}" || !slices.Equal(names, []string{"A"}) {
		t.Errorf("StripEnvRefDefaults = %q, %v", got, names)
	}
}

func TestEnvVarName(t *testing.T) {
	for in, want := range map[string]string{
		"GITHUB_TOKEN":    "GITHUB_TOKEN",
		"gh_X-Api-Key":    "gh_X_Api_Key",
		"1password_TOKEN": "_1password_TOKEN",
	} {
		if got := EnvVarName(in); got != want {
			t.Errorf("EnvVarName(%q) = %q, want %q", in, got, want)
		}
	}
}
