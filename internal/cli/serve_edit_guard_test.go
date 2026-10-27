package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJustMe_PersonalWritesStayInTheLocalFolder(t *testing.T) {
	link := func(t *testing.T, target, name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, name); err != nil {
			t.Skip("symlinks unavailable:", err)
		}
	}
	field := `{"audience":"personal","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`
	for _, tc := range []struct {
		name    string
		global  bool
		setup   func(t *testing.T, root string)
		path    string
		body    string
		guarded string
		allowed string
	}{
		{name: "local settings linked to the team settings", setup: func(t *testing.T, _ string) {
			link(t, "../settings", ".agnostic-ai/local/settings")
		}, path: editFieldPath, body: field, guarded: ".agnostic-ai/settings/defaults.yaml"},
		{name: "local folder linked to a configured source", setup: func(t *testing.T, _ string) {
			mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: [memory]\nsources:\n  settings: shared/settings\n")
			mustWrite(t, "shared/settings/defaults.yaml", "name: defaults\ndescription: Team defaults.\nmodel: sonnet\n")
			link(t, "../shared", ".agnostic-ai/local")
		}, path: editFieldPath, body: `{"audience":"personal","path":"shared/settings/defaults.yaml","key":"model","value":"opus"}`, guarded: "shared/settings/defaults.yaml"},
		{name: "personal config linked to the team config", setup: func(t *testing.T, _ string) {
			link(t, "agnostic-ai.yaml", "agnostic-ai.local.yaml")
		}, path: editBuiltinPath, body: `{"audience":"personal","name":"handoff","enabled":true}`, guarded: "agnostic-ai.yaml"},
		{name: "global local folder linked to the source", global: true, setup: func(t *testing.T, root string) {
			link(t, ".", filepath.Join(root, "local"))
		}, path: editBuiltinPath, body: `{"audience":"personal","name":"handoff","enabled":true}`, guarded: "agnostic-ai.yaml"},
		{name: "tracked personal file", setup: func(t *testing.T, root string) {
			isolateGit(t)
			mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\nmodel: haiku\n")
			git(t, root, "init", "-q")
			git(t, root, "add", "-f", ".agnostic-ai/local/settings/defaults.yaml")
			git(t, root, "commit", "-qm", "tracked")
		}, path: editFieldPath, body: field, guarded: ".agnostic-ai/local/settings/defaults.yaml"},
		{name: "local folder linked outside the project", setup: func(t *testing.T, _ string) {
			link(t, t.TempDir(), ".agnostic-ai/local")
		}, path: editFieldPath, body: field, allowed: ".agnostic-ai/local/settings/defaults.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := scopeProject(t)
			if tc.global {
				root = editHome(t)
			}
			tc.setup(t, root)
			guarded := filepath.Join(root, cmpOrPath(tc.guarded, tc.allowed))
			before := readOptional(t, guarded)
			rec := postEdit(t, newEditHandler(root, tc.global), tc.path, tc.body, nil)
			if tc.allowed != "" {
				if rec.Code != 200 || readOptional(t, guarded) == before {
					t.Fatalf("save = %d %s, want it written through the link", rec.Code, rec.Body)
				}
				return
			}
			if rec.Code < 400 {
				t.Errorf("save = %d %s, want a refusal", rec.Code, rec.Body)
			}
			if got := readOptional(t, guarded); got != before {
				t.Errorf("%s changed:\n%s", tc.guarded, got)
			}
		})
	}
}

func cmpOrPath(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func TestGitTracks_MatchesATrackedPathInAnyCase(t *testing.T) {
	isolateGit(t)
	root := t.TempDir()
	git(t, root, "init", "-q")
	mustWrite(t, filepath.Join(root, "AGNOSTIC-AI.LOCAL.YAML"), "builtins: []\n")
	git(t, root, "add", "AGNOSTIC-AI.LOCAL.YAML")
	git(t, root, "commit", "-qm", "tracked")
	if err := os.Remove(filepath.Join(root, "AGNOSTIC-AI.LOCAL.YAML")); err != nil {
		t.Fatal(err)
	}
	if !gitTracks(resolvedAbs(filepath.Join(root, "agnostic-ai.local.yaml"))) {
		t.Error("lowercase name of a tracked file reads as untracked")
	}
	if gitTracks(resolvedAbs(filepath.Join(root, "other.yaml"))) {
		t.Error("an untracked file reads as tracked")
	}
}
