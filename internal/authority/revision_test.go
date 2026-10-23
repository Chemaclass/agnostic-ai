package authority

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRevision_IsStableAndChangesWhenAnInputChanges(t *testing.T) {
	root := writeProject(t, map[string]string{
		"agnostic-ai.yaml":                       "version: 1\ntargets: [claude]\n",
		".agnostic-ai/settings/permissions.yaml": "permissions:\n  allow: [read]\n",
	})
	opts := Options{Root: root, ConfigFiles: []string{"agnostic-ai.yaml", "agnostic-ai.local.yaml"}}
	revision := func() string {
		_, b := loadProject(t, root)
		return Revision(b, opts)
	}
	first := revision()
	if len(first) != 12 || first != revision() {
		t.Fatalf("revision %q is not a stable 12-character hash", first)
	}
	if v := buildView(t, root, opts); v.Snapshot.Revision != first {
		t.Errorf("the view's revision = %s, want %s", v.Snapshot.Revision, first)
	}

	write := func(path, body string) {
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".agnostic-ai/settings/permissions.yaml", "permissions:\n  allow: [read, edit]\n")
	edited := revision()
	if edited == first {
		t.Fatal("editing a spec kept the revision")
	}
	write("agnostic-ai.local.yaml", "targets: [claude, codex]\n")
	if revision() == edited {
		t.Error("adding a config file kept the revision")
	}
}

func TestNewSnapshot_DescribesItsInputs(t *testing.T) {
	cfg, b := loadProject(t, demoRoot)
	s := NewSnapshot(cfg, b, Options{Root: demoRoot, Now: fixedNow, ConfigFiles: []string{"agnostic-ai.yaml"}})

	if s.Project.Name != "demo-project" || s.Project.Root != demoRoot {
		t.Errorf("project = %+v", s.Project)
	}
	if s.GeneratedAt != "2026-10-17T09:00:00Z" || len(s.Specs) == 0 || s.Specs[0].Path != ".agnostic-ai/agents/coding-agent.md" {
		t.Errorf("snapshot = %+v", s)
	}
	if s.Evidence.Declared != "loaded" || s.Evidence.Installed != InstalledNotChecked || s.Evidence.Runtime != "not-observed" {
		t.Errorf("evidence = %+v", s.Evidence)
	}
}
