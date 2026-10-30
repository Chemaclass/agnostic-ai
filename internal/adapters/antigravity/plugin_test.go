package antigravity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A plugin directory is only a plugin once it carries a manifest:
// "Every plugin requires a `plugin.json` file at its root to identify
// the directory as a plugin and define its metadata"
// (antigravity.google/docs/plugins). Routing components into
// `.agents/plugins/<name>/` without one emits a tree Antigravity never
// activates.
func TestEmit_WritesOneManifestPerPluginRoot(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{target: {
		SkillsDir: ".agents/plugins/team/skills",
		AgentsDir: ".agents/plugins/team/agents",
	}}}
	files := emitPluginCapture(t, cfg, []spec.Entry{
		{Kind: spec.KindSkill, Name: "probe", Path: "skills/probe/SKILL.md", Body: "b"},
		{Kind: spec.KindAgent, Name: "helper", Path: "agents/helper.md", Body: "b"},
	})

	manifest, ok := files[".agents/plugins/team/plugin.json"]
	if !ok {
		t.Fatalf("no plugin.json for the bundle root; wrote %v", sortedKeys(files))
	}
	if !strings.Contains(manifest, `"name": "team"`) {
		t.Errorf("manifest does not name the plugin after its directory:\n%s", manifest)
	}
	// Two components of one plugin share a single manifest.
	if n := countManifests(files); n != 1 {
		t.Errorf("wrote %d manifests for one plugin root, want 1", n)
	}
}

// The default tree is not a plugin, so it must not gain a manifest.
func TestEmit_NoManifestOutsideThePluginsRoot(t *testing.T) {
	files := emitPluginCapture(t, &config.Config{}, []spec.Entry{
		{Kind: spec.KindSkill, Name: "probe", Path: "skills/probe/SKILL.md", Body: "b"},
	})
	if n := countManifests(files); n != 0 {
		t.Errorf("wrote %d manifests for a default-tree sync, want 0", n)
	}
}

func TestPluginRoots_OnlyExactSupportedComponents(t *testing.T) {
	cases := map[string]string{
		".agents/plugins/team/skills":          ".agents/plugins/team",
		".agents/plugins/team/mcp.json":        ".agents/plugins/team",
		".agents/plugins/team/hooks.json":      ".agents/plugins/team",
		".agents/plugins/team/mcp_config.json": ".agents/plugins/team",
		".agents/plugins/team/skills/extra":    "",
		".agents/plugins/team/notes":           "",
		".agents/plugins/team":                 "",
		".agents/skills":                       "",
	}
	for in, want := range cases {
		if got := pluginRoots(in)[0]; got != want {
			t.Errorf("pluginRoots(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEmit_LegacyPluginMCPKeepsManifestAndNotesMove(t *testing.T) {
	dir := testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	cfg := &config.Config{Outputs: map[string]config.Output{target: {
		MCPFile: ".agents/plugins/team/mcp_config.json",
	}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(mcpGoldenEntries()), cfg, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".agents/plugins/team/plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"name": "team"`) {
		t.Errorf("unexpected manifest: %s", body)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{"Antigravity documents plugin MCP in mcp.json", "outputs.antigravity.mcp-file", ".agents/plugins/team/mcp.json"} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("missing %q in notes: %s", want, notes)
		}
	}
}

func TestEmit_PluginMCPKeepsUnownedManifest(t *testing.T) {
	dir := testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	path := filepath.Join(dir, ".agents/plugins/team/plugin.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{\n  \"name\": \"custom\", \"version\": \"2.0.0\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Outputs: map[string]config.Output{target: {MCPFile: ".agents/plugins/team/mcp.json"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(mcpGoldenEntries()), cfg, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("unowned manifest changed: %s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), ".agents/plugins/team/plugin.json") || !strings.Contains(notes.String(), "kept") {
		t.Errorf("missing preservation note: %s", notes)
	}
}

func emitPluginCapture(t *testing.T, cfg *config.Config, entries []spec.Entry) map[string]string {
	t.Helper()
	sess := emit.NewSession()
	sess.StartCapture()
	if err := (Adapter{}).Emit(sess, spec.NewBundle(entries), cfg, true); err != nil {
		sess.StopCapture()
		t.Fatalf("emit: %v", err)
	}
	out := map[string]string{}
	for _, f := range sess.StopCapture() {
		out[filepath.ToSlash(f.Path)] = f.Content
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func countManifests(files map[string]string) int {
	n := 0
	for path := range files {
		if filepath.Base(path) == "plugin.json" {
			n++
		}
	}
	return n
}
