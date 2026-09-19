package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_SettingsWritesProjectModelAndPreservesKeys(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, defaultSettingsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"respectGitignore":true,"model":"old"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gpt-5.4"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gpt-5.4" {
		t.Errorf("model = %#v, want gpt-5.4", got["model"])
	}
	if got["respectGitignore"] != true {
		t.Errorf("unrelated key lost: %#v", got)
	}
}

func TestEmit_NoSettingsFileWithoutPortableModel(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultSettingsFile)); !os.IsNotExist(err) {
		t.Errorf("settings file should not be written without a model: %v", err)
	}
}

// docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference,
// under "Repository settings (`.github/copilot/settings.json`)":
// "`disabledMcpServers` | `string[]` | Union—repository can add
// entries, never remove | MCP servers configured but not started."
// A `disabled: true` MCP spec used to start anyway under Copilot CLI
// while the coverage note claimed no file-based route existed (#888).
func TestEmit_SettingsWritesDisabledMcpServers(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "zeta", Meta: map[string]any{"command": "npx", "disabled": true}},
		{Kind: spec.KindMCP, Name: "alpha", Meta: map[string]any{"command": "npx", "disabled": true}},
		{Kind: spec.KindMCP, Name: "live", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, defaultSettingsFile))), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[disabledMcpServersKey], []any{"alpha", "zeta"}) {
		t.Errorf("%s = %#v, want the two disabled names sorted", disabledMcpServersKey, got[disabledMcpServersKey])
	}
}

// The list rides the same MergeJSONFile as `model`, so the two land
// together and nothing else in the file is touched.
func TestEmit_SettingsMergesModelAndDisabledTogether(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, defaultSettingsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"respectGitignore":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "defaults", Meta: map[string]any{"model": "gpt-5.4"}},
		{Kind: spec.KindMCP, Name: "alpha", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "gpt-5.4" || got["respectGitignore"] != true || got[disabledMcpServersKey] == nil {
		t.Errorf("settings merge dropped something: %#v", got)
	}
}

// No disabled server and no model means no settings file at all.
func TestEmit_SettingsStaysUnwrittenWithoutDisabledOrModel(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "live", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, defaultSettingsFile)); !os.IsNotExist(err) {
		t.Errorf("settings file written with nothing to say: %v", err)
	}
}

// The MCP files themselves still carry no `disabled` key: no schema
// here has one. The coverage note now names the VS Code reader only,
// since the CLI half is covered by disabledMcpServers above.
func TestEmit_MCPFilesStillCarryNoDisabledKey(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "alpha", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".vscode/mcp.json", ".github/mcp.json"} {
		if got := readFile(t, filepath.Join(dir, rel)); strings.Contains(got, "disabled") {
			t.Errorf("%s must carry no disabled key, got:\n%s", rel, got)
		}
	}
}

// docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers:
// "Next to Tools, specify which tools from the server should be
// available. Enter `*` to include all tools, or provide a
// comma-separated list of tool names (no quotes needed). The default
// is `*`." The same page's configuration-file example carries
// `"tools": ["*"]` on both the local and the http server (#888).
func TestEmit_MCPToolsAllowlistOnCopilotCLIFiles(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{
		"copilot": {RootMCPFile: ".mcp.json"},
	}}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "gh", Meta: map[string]any{
			"command": "npx",
			"tools":   []any{"list_issues", "create_issue"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".github/mcp.json", ".mcp.json"} {
		got := readFile(t, filepath.Join(dir, rel))
		if !strings.Contains(got, "list_issues") || !strings.Contains(got, "create_issue") {
			t.Errorf("%s lost the tools allowlist:\n%s", rel, got)
		}
	}
	// VS Code's own MCP reference documents no per-server `tools` key,
	// and `.vscode/mcp.json` is its file.
	if got := readFile(t, filepath.Join(dir, ".vscode/mcp.json")); strings.Contains(got, "list_issues") {
		t.Errorf("tools must not reach the VS Code file:\n%s", got)
	}
}

// The command-hook field table: "`cwd` | string | No | Working
// directory for the command (relative to repository root or
// absolute)." and "`env` | object | No | Environment variables to set
// (supports variable expansion)." Neither had any route into the
// emitted JSON before #888, and the file is wholly managed, so a hand
// edit was wiped on the next sync.
func TestEmit_HookCarriesCwdAndEnv(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "build", Meta: map[string]any{
			"event":   "PostToolUse",
			"command": "make build",
			"cwd":     "packages/web",
			"env":     map[string]any{"CI": "1", "NODE_ENV": "test"},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{`"cwd": "packages/web"`, `"NODE_ENV": "test"`, `"CI": "1"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// `x-copilot.cwd` and `x-copilot.env` reach the same fields, so a
// cross-target hook spec can keep them out of every other target's
// frontmatter.
func TestEmit_HookCwdAndEnvFromXCopilot(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "build", Meta: map[string]any{
			"event":   "PostToolUse",
			"command": "make build",
			"x-copilot": map[string]any{
				"cwd": "packages/api",
				"env": map[string]any{"TOKEN": "x"},
			},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	for _, want := range []string{`"cwd": "packages/api"`, `"TOKEN": "x"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// A hook that sets neither stays byte-identical to before: both keys
// are omitempty, so no existing emitted file gains a key.
func TestEmit_HookWithoutCwdOrEnvWritesNeitherKey(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{
			"event": "PostToolUse", "command": "gofmt -w",
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".github/hooks/agnostic-ai.json"))
	if strings.Contains(got, `"cwd"`) || strings.Contains(got, `"env"`) {
		t.Errorf("unset fields must stay out of the file:\n%s", got)
	}
}

// TestEmit_SettingsNotesPermissionsAreMDMOnly covers the gap #917
// filed: a portable permission policy reached nothing here and said
// nothing either, so a user's allow and deny lists vanished in silence.
//
// Copilot's repository-settings table is closed and names no
// `permissions` key. Its deny/ask/allow grammar exists, but only in
// device-level MDM managed settings, which no project-tier tool can
// write. The note has to say that rather than imply the vendor has no
// such feature at all.
func TestEmit_SettingsNotesPermissionsAreMDMOnly(t *testing.T) {
	testutil.TempCwd(t)
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindSettings, Name: "base", Meta: map[string]any{"permissions": map[string]any{
			"allow": []any{"Read(**)"},
			"deny":  []any{"Bash(rm:*)"},
		}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()

	note := buf.String()
	for _, want := range []string{"`permissions`", "copilot", "MDM"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
	// A permissions-only spec must not create the settings file: there
	// is no key to put the policy under.
	if _, err := os.Stat(filepath.Join(".github", "copilot", "settings.json")); err == nil {
		t.Error("a permissions-only settings spec wrote a settings file")
	}
}
