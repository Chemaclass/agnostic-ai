package crush

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitTargetHooks(t *testing.T, cfg *config.Config, entries ...spec.Entry) {
	t.Helper()
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
}

func readTargetFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertContainsAll(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %s in:\n%s", w, got)
		}
	}
}

// Crush runs hooks through its embedded POSIX shell on every platform.
func TestEmit_HookCommandsExportTheTarget(t *testing.T) {
	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "guard.sh"}})
	got := readTargetFile(t, "crush.json")
	assertContainsAll(t, got, `"command": "export AGNOSTIC_AI_TARGET=crush; guard.sh"`)
}

// Crush runs a command as a script only when it starts with ./, ../, or
// /, so a synced or relative script path gains ./ and everything else
// stays as written (#1695).
func TestScriptPathCommand(t *testing.T) {
	cases := map[string]string{
		".crush/hooks/guard.sh":         "./.crush/hooks/guard.sh",
		".crush/hooks/guard.sh --fast":  "./.crush/hooks/guard.sh --fast",
		"scripts/lint.sh":               "./scripts/lint.sh",
		"./hooks/guard.sh":              "./hooks/guard.sh",
		"../hooks/guard.sh":             "../hooks/guard.sh",
		"/usr/local/bin/guard":          "/usr/local/bin/guard",
		"~/bin/guard.sh":                "~/bin/guard.sh",
		"$HOME/bin/guard.sh":            "$HOME/bin/guard.sh",
		"bash .crush/hooks/guard.sh":    "bash .crush/hooks/guard.sh",
		"npx foo":                       "npx foo",
		"guard.sh":                      "guard.sh",
		"FOO=a/b .crush/hooks/guard.sh": "FOO=a/b .crush/hooks/guard.sh",
		`"dir with space/guard.sh"`:     `"dir with space/guard.sh"`,
		"a/b;rm -rf x":                  "a/b;rm -rf x",
		"a/$(x)":                        "a/$(x)",
		"":                              "",
	}
	for in, want := range cases {
		if got := scriptPathCommand(in); got != want {
			t.Errorf("scriptPathCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmit_SyncedHookScriptStartsWithDotSlash(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/guard.sh", []byte("exit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard.sh"}}}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readTargetFile(t, "crush.json")
	assertContainsAll(t, got, `"command": "export AGNOSTIC_AI_TARGET=crush; ./.crush/hooks/guard.sh"`)
}
