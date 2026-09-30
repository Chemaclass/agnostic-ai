package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestDoctor_ReportsCodexHookTrustInTextAndJSON(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteGlobalTest(t, ".agnostic-ai/hooks/guard.yaml", "event: PreToolUse\nmatcher: Bash\ncommand: guard-env\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"doctor", "-t", "codex"})
	err := root.Execute()
	if err == nil {
		t.Error("doctor passed with an untrusted guard")
	}
	if got := output.String(); !strings.Contains(got, "guard-env") || !strings.Contains(got, "untrusted") || !strings.Contains(got, "/hooks") || strings.Contains(got, "All checks passed") {
		t.Errorf("missing doctor trust diagnosis: %s", got)
	}
	output.Reset()
	root = NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"doctor", "--json", "-t", "codex"})
	if err := root.Execute(); err == nil {
		t.Error("JSON doctor passed with an untrusted guard")
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("JSON doctor: %v: %s", err, output.String())
	}
	if !bytes.Contains(report["hook_trust"], []byte("untrusted")) {
		t.Errorf("JSON missing hook trust: %s", output.String())
	}
}

func TestSyncGlobal_CodexHookTrustUsesMergedCommandsAndPreservesState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	source := filepath.Join(home, "source")
	t.Setenv("AGNOSTIC_AI_HOME", source)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [codex]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "event: PreToolUse\nmatcher: Bash\ncommand: guard-env\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "model.yaml"), "model: gpt-test\n")
	state := "[hooks.state.\"retained-key\"]\ntrusted_hash = \"sha256:retained\"\nenabled = false\n"
	mustWriteGlobalTest(t, filepath.Join(codexHome, "config.toml"), state)
	mustWriteGlobalTest(t, filepath.Join(codexHome, "hooks.json"), `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"handwritten"}]}]}}`)
	var output, notes bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&notes)
	root.SetArgs([]string{"sync", "--global", "--only", "codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := notes.String()
	if !strings.Contains(got, "export AGNOSTIC_AI_TARGET=codex; guard-env") || !strings.Contains(got, "handwritten") || !strings.Contains(got, "/hooks") {
		t.Errorf("missing merged hook trust notes: %s", got)
	}
	config, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(config, []byte("trusted_hash = \"sha256:retained\"")) || !bytes.Contains(config, []byte("enabled = false")) {
		t.Errorf("user state was changed: %s", config)
	}

	resolvedHome, err := filepath.EvalSymlinks(codexHome)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(resolvedHome, "hooks.json") + ":pre_tool_use:0:0"
	trusted := append(config, []byte(fmt.Sprintf("\n[hooks.state.%q]\ntrusted_hash = %q\n", key, "sha256:fd022754fe4b7274aa51f6229943be4f6bb80d966b69cffe4d793578e14ce6e8"))...)
	mustWriteGlobalTest(t, filepath.Join(codexHome, "config.toml"), string(trusted))
	body, err := os.ReadFile(filepath.Join(codexHome, "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	findings, err := codex.HookTrustFindings(filepath.Join(codexHome, "hooks.json"), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Hook != "handwritten" {
		t.Errorf("merged final export command did not match runtime trust: %+v", findings)
	}
}

func TestDoctor_ReportsDisabledCodexHookWithoutFailure(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteGlobalTest(t, ".agnostic-ai/hooks/guard.yaml", "event: PreToolUse\nmatcher: Bash\ncommand: guard-env\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(cwd, ".codex/hooks.json") + ":pre_tool_use:0:0"
	mustWriteGlobalTest(t, filepath.Join(home, "config.toml"), fmt.Sprintf("[hooks.state.%q]\nenabled = false\n", key))
	var output bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"doctor", "-t", "codex"})
	if err := root.Execute(); err != nil {
		t.Errorf("intentional disabling failed doctor: %v", err)
	}
	if !strings.Contains(output.String(), "disabled") {
		t.Errorf("disabled hook not reported: %s", output.String())
	}
}

func TestDoctor_ReportsUserCodexHooksAndMalformedUserConfig(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	mustWriteGlobalTest(t, filepath.Join(home, "hooks.json"), `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"user-guard"}]}]}}`)
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	findings := collectCodexHookTrust(cfg, []string{"codex"})
	if len(findings) != 1 || findings[0].Hook != "user-guard" || findings[0].Status != "untrusted" {
		t.Errorf("user hook not inspected: %+v", findings)
	}
	mustWriteGlobalTest(t, filepath.Join(home, "config.toml"), "[broken")
	findings = collectCodexHookTrust(cfg, []string{"codex"})
	if len(findings) != 1 || findings[0].Status != "unknown" || !strings.Contains(findings[0].Problem, "config.toml") {
		t.Errorf("malformed trust config not diagnosed: %+v", findings)
	}
	if got := collectCodexHookTrust(cfg, []string{"claude"}); len(got) != 0 {
		t.Errorf("inspected unselected Codex: %+v", got)
	}
}

func TestDoctor_ReportsNullCodexHookGroupsInTextAndJSON(t *testing.T) {
	cases := []struct{ name, body string }{
		{"null group", `{"hooks":{"PreToolUse":[null]}}`},
		{"null handlers", `{"hooks":{"PreToolUse":[{"hooks":null}]}}`},
		{"null handler", `{"hooks":{"PreToolUse":[{"hooks":[null]}]}}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			home := t.TempDir()
			t.Setenv("CODEX_HOME", home)
			mustWriteGlobalTest(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
			root := NewRootCmd("test")
			root.SetArgs([]string{"sync"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			mustWriteGlobalTest(t, filepath.Join(home, "hooks.json"), test.body)
			var text bytes.Buffer
			root = NewRootCmd("test")
			root.SetOut(&text)
			root.SetErr(&text)
			root.SetArgs([]string{"doctor", "-t", "codex"})
			if err := root.Execute(); err == nil {
				t.Error("doctor accepted malformed hooks")
			}
			if !strings.Contains(text.String(), "cannot check hook trust") || strings.Contains(text.String(), "All checks passed") {
				t.Errorf("missing malformed hooks diagnosis: %s", text.String())
			}
			var output bytes.Buffer
			root = NewRootCmd("test")
			root.SetOut(&output)
			root.SetErr(&bytes.Buffer{})
			root.SetArgs([]string{"doctor", "--json", "-t", "codex"})
			if err := root.Execute(); err == nil {
				t.Error("JSON doctor accepted malformed hooks")
			}
			var report struct {
				HookTrust []struct{ Status, Problem string } `json:"hook_trust"`
			}
			if err := json.Unmarshal(output.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if len(report.HookTrust) != 1 || report.HookTrust[0].Status != "unknown" || report.HookTrust[0].Problem == "" {
				t.Errorf("missing unknown diagnostic: %s", output.String())
			}
		})
	}
}
