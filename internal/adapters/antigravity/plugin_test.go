package antigravity

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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

// A path inside a component is not itself a component, and a name the
// vendor does not document is not one either.
func TestPluginRoots_OnlyExactDocumentedComponents(t *testing.T) {
	cases := map[string]string{
		".agents/plugins/team/skills":          ".agents/plugins/team",
		".agents/plugins/team/mcp_config.json": ".agents/plugins/team",
		".agents/plugins/team/hooks.json":      ".agents/plugins/team",
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
