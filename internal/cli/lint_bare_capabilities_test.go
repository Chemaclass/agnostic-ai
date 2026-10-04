package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLintBareCapabilities_NamesEachTargetAndScopedAlternative(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Path: filepath.Join("settings", "permissions.yaml"), Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell", "shell"}, "ask": []any{"edit"}}}}
	got := lintBareCapabilities([]spec.Entry{entry}, []string{"claude", "opencode", "windsurf"})
	if len(got) != 2 {
		t.Fatalf("findings = %v", got)
	}
	for _, want := range []string{"LINT038", "permissions.allow", "shell(git status)", "claude: Bash", "opencode: bash", "windsurf: exec"} {
		if !strings.Contains(got[0].String(), want) {
			t.Errorf("missing %q: %s", want, got[0])
		}
	}
	if !strings.Contains(got[1].Message, "edit(src/**)") || !strings.Contains(got[1].Message, "permissions.ask") {
		t.Errorf("ask = %s", got[1])
	}
}

func TestLintBareCapabilities_SkipsScopedDeniedAndNativeRules(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell(git status)", "edit(src/**)", "read(src/**)", "mcp:github/get_issue", "Bash", "mcp:"}, "deny": []any{"shell", "edit", "read", "web", "mcp:github"}}}}
	if got := lintBareCapabilities([]spec.Entry{entry}, []string{"claude"}); len(got) != 0 {
		t.Errorf("findings = %v", got)
	}
}

func TestLintBareCapabilities_WarnsForQoderPermissions(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell"}}}}
	got := lintBareCapabilities([]spec.Entry{entry}, []string{"qoder"})
	if len(got) != 1 {
		t.Fatalf("findings = %v", got)
	}
	if !strings.Contains(got[0].Message, "qoder: Bash") {
		t.Errorf("message = %s", got[0].Message)
	}
}

func TestLintBareCapabilities_NamesMCPServerScope(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"mcp:github"}}}}
	got := lintBareCapabilities([]spec.Entry{entry}, []string{"claude", "cursor"})
	if len(got) != 1 {
		t.Fatalf("findings = %v", got)
	}
	for _, want := range []string{"mcp__github", "Mcp(github:*)", "mcp:github/<tool>"} {
		if !strings.Contains(got[0].Message, want) {
			t.Errorf("missing %q: %s", want, got[0])
		}
	}
}

func TestSync_BareCapabilitiesWarnOnceEveryRunAndHonorSilent(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	log := captureLogOut(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n  allow: [shell, shell]\n")
	for i := 0; i < 2; i++ {
		log.Reset()
		if err := runSyncOnce(dir, nil, false, false, "off", 1); err != nil {
			t.Fatal(err)
		}
		if count := strings.Count(log.String(), "LINT038"); count != 1 || !strings.Contains(log.String(), "claude: Bash") {
			t.Errorf("run %d: %s", i, log.String())
		}
	}
	log.Reset()
	warnBareCapabilities(&config.Config{OnUnsupported: "silent"}, spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell"}}}}}}, []string{"claude"})
	if log.Len() != 0 {
		t.Errorf("silent = %s", log.String())
	}
}

func TestLintBareCapabilities_RespectsTargetsAndNativeOverrides(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "a", Meta: map[string]any{"target": "windsurf", "permissions": map[string]any{"allow": []any{"shell"}}, "x-windsurf": map[string]any{"permissions": map[string]any{"deny": []any{"exec"}}}}},
		{Kind: spec.KindSettings, Name: "b", Meta: map[string]any{"target": "opencode", "permissions": map[string]any{"allow": []any{"shell"}}, "x-opencode": map[string]any{"permission": map[string]any{"bash": "deny"}}}},
		{Kind: spec.KindSettings, Name: "c", Meta: map[string]any{"target": "kilo", "permissions": map[string]any{"allow": []any{"shell"}}, "x-kilo": map[string]any{"permission": map[string]any{"read": "deny"}}}},
	}
	if got := lintBareCapabilities(entries, []string{"claude", "windsurf", "opencode", "kilo"}); len(got) != 0 {
		t.Errorf("findings = %v", got)
	}
}

func TestWarnBareCapabilities_QuietSuppressesSyncOnly(t *testing.T) {
	silence(t)
	log := captureLogOut(t)
	old := verbosity
	verbosity = levelQuiet
	t.Cleanup(func() { verbosity = old })
	b := spec.Bundle{Settings: []spec.Entry{{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell"}}}}}}
	warnBareCapabilities(&config.Config{}, b, []string{"claude"})
	if log.Len() != 0 {
		t.Errorf("quiet = %s", log.String())
	}
	if got := lintBareCapabilities(b.Settings, []string{"claude"}); len(got) != 1 {
		t.Errorf("lint must still report: %v", got)
	}
}

func TestSyncJSON_BareCapabilitiesKeepStdoutValid(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n  allow: [shell]\n")
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"sync", "--json", "--gitignore=off"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(stdout.Bytes()) {
		t.Errorf("stdout is not JSON: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "LINT038") {
		t.Errorf("stderr misses warning: %s", stderr.String())
	}
}

func TestLint_BareCapabilitiesWarnByDefaultAndFailStrict(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "settings", "permissions.yaml"), "permissions:\n  allow: [shell]\n")
	if out, err := runCLI(t, "lint"); err != nil || !strings.Contains(out, "LINT038") {
		t.Errorf("lint = %v, %s", err, out)
	}
	if out, err := runCLI(t, "lint", "--strict"); err == nil || !strings.Contains(out, "LINT038") {
		t.Errorf("strict = %v, %s", err, out)
	}
}
