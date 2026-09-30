package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HookTrustNamesInactiveHookWithoutTrustingIt(t *testing.T) {
	testutil.TempCwd(t)
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	original := "model = \"test\"\n"
	path := filepath.Join(home, "config.toml")
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	var notes bytes.Buffer
	prev := emit.Warner
	emit.Warner = &notes
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	emit.ResetCoverageNotes()
	entry := spec.Entry{Kind: spec.KindHook, Name: "env-guard", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "guard-env"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{OnUnsupported: "error"}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if got := notes.String(); !strings.Contains(got, "guard-env") || !strings.Contains(got, "untrusted") || !strings.Contains(got, "/hooks") {
		t.Errorf("missing trust setup note: %s", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Errorf("user trust config changed: %s", got)
	}
}

func TestHookTrust_RuntimeHashFixtures(t *testing.T) {
	raw, err := os.ReadFile("testdata/hook-trust.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name, Event, Matcher, Platform, Hash, Canonical string
		Handler                                         map[string]any
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			group := map[string]any{"matcher": fixture.Matcher, "hooks": []any{fixture.Handler}}
			body, err := json.Marshal(map[string]any{"hooks": map[string]any{fixture.Event: []any{group}}})
			if err != nil {
				t.Fatal(err)
			}
			eventKey := ""
			for _, event := range hookTrustEvents {
				if event.name == fixture.Event {
					eventKey = event.key
				}
			}
			path := "/project/.codex/hooks.json"
			key := fmt.Sprintf("%s:%s:0:0", path, eventKey)
			config := []byte(fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", key, fixture.Hash))
			findings, err := inspectHookTrust(path, body, config, fixture.Platform)
			if err != nil {
				t.Fatal(err)
			}
			if len(findings) != 0 {
				t.Errorf("runtime fixture hash not recognized: %+v; identity %s", findings, fixture.Canonical)
			}
		})
	}
}

func TestHookTrust_ModifiedDisabledAndProjectState(t *testing.T) {
	body := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard-env"}]}]}}`)
	path := "/project/.codex/hooks.json"
	key := path + ":pre_tool_use:0:0"
	hash := "sha256:e9d9cf155ba9ce2edebca3b635c99c5aa19d57db27499690a9f260388ada88fc"
	config := []byte(fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", key, hash))
	findings, err := inspectHookTrust(path, bytes.ReplaceAll(body, []byte("guard-env"), []byte("changed-guard")), config, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Status != "modified" {
		t.Errorf("changed command trusted: %+v", findings)
	}
	config = append(config, []byte("enabled = false\n")...)
	findings, err = inspectHookTrust(path, body, config, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Status != "disabled" {
		t.Errorf("disabled hook status: %+v", findings)
	}
	findings, err = inspectHookTrust(path, body, nil, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Status != "untrusted" {
		t.Errorf("missing USER state status: %+v", findings)
	}
}

func TestHookTrust_UserConfigOnlyAndCodexHomeAlias(t *testing.T) {
	testutil.TempCwd(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(home, alias); err != nil {
		t.Skipf("symlink: %v", err)
	}
	t.Setenv("CODEX_HOME", alias)
	body := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard-env"}]}]}}`)
	config := fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", filepath.Join(dir, ".codex/hooks.json")+":pre_tool_use:0:0", "sha256:e9d9cf155ba9ce2edebca3b635c99c5aa19d57db27499690a9f260388ada88fc")
	if err := os.MkdirAll(".codex", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".codex/config.toml", []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err := HookTrustFindings(".codex/../.codex/hooks.json", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Status != "untrusted" {
		t.Errorf("project state accepted: %+v", findings)
	}
	resolvedHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(resolvedHome, "hooks.json") + ":pre_tool_use:0:0"
	config = fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", key, "sha256:e9d9cf155ba9ce2edebca3b635c99c5aa19d57db27499690a9f260388ada88fc")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	findings, err = HookTrustFindings(filepath.Join(alias, "hooks.json"), body)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("canonical CODEX_HOME state not recognized: %+v", findings)
	}
}

func TestHookTrust_MalformedConfigAndInvalidMCPInput(t *testing.T) {
	body := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"guard"}]}]}}`)
	if _, err := inspectHookTrust("/hooks.json", body, []byte("[broken"), "linux"); err == nil {
		t.Error("malformed user config accepted")
	}
	body = []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"mcp_tool","server":"s","tool":"t","input":{"bad":null}}]}]}}`)
	if _, err := inspectHookTrust("/hooks.json", body, nil, "linux"); err == nil {
		t.Error("runtime-invalid MCP input accepted")
	}
}

func TestEmit_HookTrustDryRunAndCapture(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	entry := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "command": "guard"}}
	var notes bytes.Buffer
	prev := emit.Warner
	emit.Warner = &notes
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{OnUnsupported: "silent"}, true); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "/hooks") {
		t.Errorf("dry-run lacks trust preview: %s", notes.String())
	}
	if _, err := os.Stat(".codex/hooks.json"); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote hooks: %v", err)
	}
	notes.Reset()
	sess := emit.NewSession()
	sess.StartCapture()
	if err := New().Emit(sess, spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if notes.Len() != 0 {
		t.Errorf("capture inspected host trust: %s", notes.String())
	}
}

func TestHookTrust_GroupIndexesAndMalformedState(t *testing.T) {
	path := "/project/.codex/hooks.json"
	body := []byte(`{"hooks":{"PreToolUse":[{"matcher":"ignored","hooks":[{"type":"prompt"}]},{"matcher":"Bash","hooks":[{"type":"agent"},{"type":"command","command":"guard-env"}]}]}}`)
	config := []byte(fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", path+":pre_tool_use:1:1", "sha256:e9d9cf155ba9ce2edebca3b635c99c5aa19d57db27499690a9f260388ada88fc"))
	findings, err := inspectHookTrust(path, body, config, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("hash included sibling handlers or changed indexes: %+v", findings)
	}
	config = append(config, []byte("enabled = 'bad'\n")...)
	findings, err = inspectHookTrust(path, body, config, "linux")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Status != "untrusted" {
		t.Errorf("malformed state partially trusted: %+v", findings)
	}
}

func TestEmit_HookTrustDoesNotClaimAnUnmanagedFileWasInstalled(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("CODEX_HOME", t.TempDir())
	var notes bytes.Buffer
	prev := emit.Warner
	emit.Warner = &notes
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	sess := emit.NewSession()
	sess.SetUnmanaged([]string{".codex/hooks.json"})
	entry := spec.Entry{Kind: spec.KindHook, Meta: map[string]any{"event": "Stop", "command": "guard"}}
	if err := New().Emit(sess, spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(notes.String(), "/hooks") {
		t.Errorf("claimed trust status for skipped output: %s", notes.String())
	}
}

func TestEmit_HookTrustRecognizesUserTrustAndChangedCommand(t *testing.T) {
	hash := "sha256:fd022754fe4b7274aa51f6229943be4f6bb80d966b69cffe4d793578e14ce6e8"
	if runtime.GOOS == "windows" {
		hash = "sha256:e9d9cf155ba9ce2edebca3b635c99c5aa19d57db27499690a9f260388ada88fc"
	}
	testutil.TempCwd(t)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	key := filepath.Join(dir, ".codex/hooks.json") + ":pre_tool_use:0:0"
	userConfig := fmt.Sprintf("[hooks.state.%q]\ntrusted_hash = %q\n", key, hash)
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(userConfig), 0600); err != nil {
		t.Fatal(err)
	}
	var notes bytes.Buffer
	prev := emit.Warner
	emit.Warner = &notes
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	entry := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "guard-env"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(notes.String(), "/hooks") {
		t.Errorf("already trusted hook reported: %s", notes.String())
	}
	notes.Reset()
	entry.Meta["command"] = "changed-guard"
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "modified") || !strings.Contains(notes.String(), "changed-guard") {
		t.Errorf("changed hook status: %s", notes.String())
	}
}

func TestHookTrust_RejectsMalformedHooksFile(t *testing.T) {
	for _, body := range []string{`null`, `{"version":1,"hooks":{}}`, `{"hooks":{"Stop":[{"hooks":[{"command":"guard"}]}]}}`, `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"guard","async":null}]}]}}`} {
		if _, err := inspectHookTrust("/hooks.json", []byte(body), nil, "linux"); err == nil {
			t.Errorf("runtime-invalid hooks accepted: %s", body)
		}
	}
}
