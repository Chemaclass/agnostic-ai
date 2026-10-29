package opencode

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// readAllowed is a whole read grant: OpenCode's own .env rules stay after it.
const readAllowed = "  read:\n    \"*\": allow\n    \"*.env\": ask\n    \"*.env.*\": ask\n    \"*.env.example\": allow\n"

func emitAgent(t *testing.T, meta map[string]any) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	entry, problem := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: meta, Body: "Review."}.NativeTools()
	if problem != "" {
		t.Fatalf("capability problem: %s", problem)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	return readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
}

func TestEmit_Agent_CanReadBecomesReadOnlyPermission(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "can": []any{"read"}})
	want := "permission:\n  \"*\": deny\n  doom_loop: ask\n  external_directory: ask\n" + readAllowed
	if !strings.Contains(got, want) {
		t.Errorf("want read-only permission map %q in:\n%s", want, got)
	}
}

func TestEmit_Agent_ScopedToolPutsCatchAllDenyFirst(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "can": []any{"read", "shell(git diff *)"}})
	want := "  bash:\n    \"*\": deny\n    git diff *: allow\n"
	if !strings.Contains(got, want) {
		t.Errorf("want scoped bash map %q in:\n%s", want, got)
	}
}

func TestEmit_Agent_XOpencodePermissionWinsForItsKeys(t *testing.T) {
	got := emitAgent(t, map[string]any{
		"description": "d",
		"tools":       []any{"Read", "Bash"},
		"x-opencode":  map[string]any{"permission": map[string]any{"bash": "ask"}},
	})
	for _, want := range []string{"\"*\": deny", "bash: ask", readAllowed} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "bash: allow") {
		t.Errorf("x-opencode.permission must win for bash:\n%s", got)
	}
}

func TestEmit_Agent_XOpencodeScalarPermissionWinsOutright(t *testing.T) {
	got := emitAgent(t, map[string]any{
		"description": "d",
		"tools":       []any{"Read"},
		"x-opencode":  map[string]any{"permission": "ask"},
	})
	if !strings.Contains(got, "permission: ask\n") || strings.Contains(got, "read:") {
		t.Errorf("scalar x-opencode.permission must win outright:\n%s", got)
	}
}

func TestEmit_Agent_UnknownToolPrintsCoverageNote(t *testing.T) {
	buf := swapNoteWarner(t)
	got := emitAgent(t, map[string]any{"description": "d", "tools": []any{"Read", "NotebookEdit"}})
	if !strings.Contains(got, readAllowed) {
		t.Errorf("known tool must still translate:\n%s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "opencode") || !strings.Contains(buf.String(), "tools") {
		t.Errorf("want a coverage note for the unmapped tool, got: %q", buf.String())
	}
}

func TestEmit_Agent_OnlyUnknownToolsKeepsDefaultPermissions(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "tools": []any{"NotebookEdit"}})
	if strings.Contains(got, "permission:") {
		t.Errorf("an untranslatable list must not lock the agent out:\n%s", got)
	}
}

func TestEmit_Agent_BareMCPServerAllowsEveryServerTool(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "can": []any{"read", "mcp:github"}})
	if !strings.Contains(got, "github_*: allow") {
		t.Errorf("want github_* allowed:\n%s", got)
	}
}

func TestEmit_Agent_RestrictedAgentKeepsMemoryStoreGrant(t *testing.T) {
	dir := testutil.TempCwd(t)
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cfg := &config.Config{
		Builtins:  []string{emit.MemoryBuiltin},
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: true},
	}
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "d", "tools": []any{"Read"}}, Body: "x"}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{agent}), cfg, false); err != nil {
		t.Fatal(err)
	}
	project := readFile(t, filepath.Join(dir, "opencode.json"))
	got := readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
	if !strings.Contains(project, "external_directory") {
		t.Fatalf("setup: opencode.json grants no store:\n%s", project)
	}
	var front struct {
		Permission map[string]any `yaml:"permission"`
	}
	if err := yaml.Unmarshal([]byte(strings.Split(got, "---")[1]), &front); err != nil {
		t.Fatal(err)
	}
	outside, _ := front.Permission["external_directory"].(map[string]any)
	store, _ := emit.PersonalMemoryDirFor(cfg, "opencode.json", target)
	if outside["*"] != "ask" || outside[filepath.ToSlash(store)+"/**"] != "allow" {
		t.Errorf("external_directory = %#v, want * ask and %s/** allow", outside, store)
	}
}

func TestEmit_Agent_RestrictedAgentKeepsSettingsOutsideDeny(t *testing.T) {
	dir := testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "settings", Path: "settings.yaml", Meta: map[string]any{
		"x-opencode": map[string]any{"permission": map[string]any{
			"external_directory": map[string]any{"/secrets/**": "deny"},
		}},
	}}
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "d", "tools": []any{"Read"}}, Body: "x"}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
	want := "  external_directory:\n    \"*\": ask\n    /secrets/**: deny\n"
	if !strings.Contains(got, want) {
		t.Errorf("want %q (settings deny after the agent's ask) in:\n%s", want, got)
	}
}

func TestEmit_Agent_BareSettingsOutsideDenyStaysDeny(t *testing.T) {
	dir := testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "settings", Path: "settings.yaml", Meta: map[string]any{
		"x-opencode": map[string]any{"permission": map[string]any{"external_directory": "deny"}},
	}}
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "d", "tools": []any{"Read"}}, Body: "x"}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
	if !strings.Contains(got, "  external_directory: deny\n") {
		t.Errorf("want external_directory: deny in:\n%s", got)
	}
}

func emitAgentWithSettings(t *testing.T, settingsMeta, agentMeta map[string]any) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	settings := spec.Entry{Kind: spec.KindSettings, Name: "settings", Path: "settings.yaml", Meta: settingsMeta}
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: agentMeta, Body: "x"}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	return readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
}

func TestEmit_Agent_ProjectAskStaysOnListedTool(t *testing.T) {
	got := emitAgentWithSettings(t,
		map[string]any{"permissions": map[string]any{"ask": []any{"Bash"}}},
		map[string]any{"description": "d", "tools": []any{"Read", "Bash"}})
	if !strings.Contains(got, "  bash: ask\n") {
		t.Errorf("project ask must win over the agent's allow:\n%s", got)
	}
}

func TestEmit_Agent_ProjectScopedDenySurvivesListedTool(t *testing.T) {
	got := emitAgentWithSettings(t,
		map[string]any{"permissions": map[string]any{"deny": []any{"Read(/secrets/**)"}}},
		map[string]any{"description": "d", "tools": []any{"Read"}})
	want := readAllowed + "    /secrets/**: deny\n"
	if !strings.Contains(got, want) {
		t.Errorf("want %q in:\n%s", want, got)
	}
}

func TestEmit_Agent_ProjectBareDenyKeepsToolDenied(t *testing.T) {
	got := emitAgentWithSettings(t,
		map[string]any{"permissions": map[string]any{"deny": []any{"Bash"}}},
		map[string]any{"description": "d", "tools": []any{"Bash"}})
	if !strings.Contains(got, "  bash: deny\n") {
		t.Errorf("project deny must win:\n%s", got)
	}
}

func TestEmit_Agent_TildePatternStaysAString(t *testing.T) {
	got := emitAgentWithSettings(t,
		map[string]any{"x-opencode": map[string]any{"permission": map[string]any{
			"external_directory": map[string]any{"~": "deny"},
		}}},
		map[string]any{"description": "d", "tools": []any{"Read"}})
	var front struct {
		Permission map[string]any `yaml:"permission"`
	}
	if err := yaml.Unmarshal([]byte(strings.Split(got, "---")[1]), &front); err != nil {
		t.Fatal(err)
	}
	outside, _ := front.Permission["external_directory"].(map[string]any)
	if outside["~"] != "deny" {
		t.Errorf("external_directory = %#v, want \"~\" deny:\n%s", outside, got)
	}
}

func TestEmit_Agent_DryRunKeepsHandWrittenProjectRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(`{"permission": {"bash": {"rm *": "deny"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	settings := spec.Entry{Kind: spec.KindSettings, Name: "s", Path: "s.yaml", Meta: map[string]any{
		"x-opencode": map[string]any{"permission": map[string]any{"edit": "ask"}},
	}}
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "d", "tools": []any{"Bash", "Edit"}}, Body: "x"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = w
	emitErr := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{settings, agent}), &config.Config{}, true)
	os.Stdout = stdout
	_ = w.Close()
	printed, _ := io.ReadAll(r)
	if emitErr != nil {
		t.Fatal(emitErr)
	}
	preview := string(printed)
	start := strings.Index(preview, filepath.FromSlash(".opencode/agents/reviewer.md")+" ---")
	if start < 0 {
		t.Fatalf("no agent preview in:\n%s", preview)
	}
	agentPreview := preview[start:]
	for _, want := range []string{"rm *: deny", "edit: ask"} {
		if !strings.Contains(agentPreview, want) {
			t.Errorf("dry run agent preview misses %q:\n%s", want, agentPreview)
		}
	}
}
