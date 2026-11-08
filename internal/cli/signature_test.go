package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSignature_LinksTheNameOnATerminal(t *testing.T) {
	want := "Made by \x1b]8;;https://chemaclass.com\x1b\\Chemaclass\x1b]8;;\x1b\\"
	if got := signature(true); got != want {
		t.Errorf("signature(true) = %q, want %q", got, want)
	}
}

func TestSignature_SpellsOutTheAddressOffATerminal(t *testing.T) {
	if got, want := signature(false), "Made by Chemaclass · https://chemaclass.com"; got != want {
		t.Errorf("signature(false) = %q, want %q", got, want)
	}
}

func TestRoot_HelpEndsWithThePlainSignatureAndSubcommandHelpDoesNot(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, c := range []struct {
		args   []string
		signed bool
	}{{[]string{"--help"}, true}, {[]string{"sync", "--help"}, false}} {
		root := NewRootCmd("test")
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetArgs(c.args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got := strings.HasSuffix(out.String(), "\n\n"+signature(false)+"\n"); got != c.signed {
			t.Errorf("%v: signed = %v, want %v:\n%s", c.args, got, c.signed, out.String())
		}
	}
}

func TestRoot_PipedVersionStaysOneLine(t *testing.T) {
	root := NewRootCmd("0.91.0")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "agnostic-ai version 0.91.0\n"; got != want {
		t.Errorf("version = %q, want %q", got, want)
	}
}

func TestSignVersion_FollowsOnlyTheVersionFlag(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, c := range []struct {
		args []string
		want string
	}{{[]string{"--version"}, signature(true) + "\n"}, {[]string{"sync", "--help"}, ""}, {[]string{"--help", "--version"}, ""}, {[]string{"--version=false"}, ""}} {
		root := NewRootCmd("0.91.0")
		root.SetOut(&bytes.Buffer{})
		root.SetArgs(c.args)
		ran, err := root.ExecuteC()
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		signVersion(ran, &out)
		if out.String() != c.want {
			t.Errorf("%v: signed %q, want %q", c.args, out.String(), c.want)
		}
	}
}
