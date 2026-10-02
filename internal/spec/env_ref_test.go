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
		if got := tc.syntax.Read(tc.native, false); got != "Bearer ${API_KEY}" {
			t.Errorf("Read(%q) = %q", tc.native, got)
		}
	}
}

func TestEnvRefSyntax_ReadUnbracedOnlyWhenAsked(t *testing.T) {
	if got := EnvRefDollar.Read("$TOKEN", true); got != "${TOKEN}" {
		t.Errorf("unbraced read = %q", got)
	}
	if got := EnvRefDollar.Read("$TOKEN", false); got != "$TOKEN" {
		t.Errorf("a tool without the bare form must keep it: %q", got)
	}
	if got := EnvRefDollar.Read("pa$TOKEN", true); got != "pa$TOKEN" {
		t.Errorf("only a whole value reads as a bare reference: %q", got)
	}
}

func TestEnvRefNames(t *testing.T) {
	if got := EnvRefNames("${A}:${B_2} ${env:C} ${D:-x}"); !slices.Equal(got, []string{"A", "B_2"}) {
		t.Errorf("EnvRefNames = %v", got)
	}
	if name, ok := WholeEnvRef("${TOKEN}"); !ok || name != "TOKEN" {
		t.Errorf("WholeEnvRef = %q, %v", name, ok)
	}
	if _, ok := WholeEnvRef("Bearer ${TOKEN}"); ok {
		t.Error("an embedded reference is not a whole value")
	}
	if !HasEnvRef("${TOKEN:-dev}") || HasEnvRef("ghp_example") {
		t.Error("HasEnvRef must accept any ${...} and reject a literal")
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
