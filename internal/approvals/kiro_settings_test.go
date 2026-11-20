package approvals_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestKiroAskRuleFiles_EveryScope(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	cwd := filepath.Join(root, "repo", "sub")
	for _, path := range []string{
		filepath.Join(home, ".kiro", "workspace-roots", "h1", "permissions.yaml"),
		filepath.Join(home, ".kiro", "agents", "a.json"),
		filepath.Join(root, "repo", ".kiro", "agents", "b.md"),
		filepath.Join(root, "repo", ".kiro", "agents", "notes.txt"),
	} {
		writeFile(t, path, "")
	}
	got := approvals.KiroAskRuleFiles(home, cwd)
	for _, want := range []string{
		filepath.Join(home, ".kiro", "settings", "permissions.yaml"),
		filepath.Join(home, ".kiro", "workspace-roots", "h1", "permissions.yaml"),
		filepath.Join(home, ".kiro", "agents", "a.json"),
		filepath.Join(root, "repo", ".kiro", "agents", "b.md"),
	} {
		if !slices.Contains(got, want) {
			t.Errorf("files %v miss %s", got, want)
		}
	}
	if slices.Contains(got, filepath.Join(root, "repo", ".kiro", "agents", "notes.txt")) {
		t.Errorf("files %v hold a text file", got)
	}
}

func TestKiroHasAskRule(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		file, body string
		ask, err   bool
	}{
		"allow only":       {"p.yaml", "rules:\n  - capability: shell\n    effect: allow\n", false, false},
		"ask rule":         {"p.yaml", "rules:\n  - capability: shell\n    effect: ASK\n", true, false},
		"preset":           {"p.yaml", "policies: [edit-workspace]\n", false, false},
		"profile ask":      {"a.json", `{"permissions":{"rules":[{"capability":"fs_write","effect":"ask"}]}}`, true, false},
		"markdown ask":     {"a.md", "---\npermissions:\n  rules:\n    - effect: ask\n---\n# Agent\n", true, false},
		"markdown no head": {"b.md", "# Agent\neffect: ask\n", false, false},
		"broken json":      {"c.json", `{"permissions":`, false, true},
		"approve for me":   {"p.yaml", "policies: [read-workspace, approve-for-me]\n", true, false},
		"evaluate rule":    {"p.yaml", "rules:\n  - capability: shell\n    effect: evaluate\n", true, false},
		"number key":       {"p.yaml", "1: x\nrules:\n  - capability: shell\n    effect: ask\n", true, false},
		"bool key in rule": {"p.yaml", "rules:\n  - true: 1\n    effect: ask\n", true, false},
		"second document":  {"p.yaml", "rules: []\n---\nrules:\n  - capability: shell\n    effect: ask\n", true, false},
		"broken yaml":      {"q.yaml", "rules: [\n", false, true},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name, tc.file)
			writeFile(t, path, tc.body)
			ask, err := approvals.KiroHasAskRule([]string{path})
			if ask != tc.ask || (err != nil) != tc.err {
				t.Errorf("ask %v err %v; want ask %v err %v", ask, err, tc.ask, tc.err)
			}
		})
	}
	if ask, err := approvals.KiroHasAskRule([]string{filepath.Join(dir, "missing.yaml")}); ask || err != nil {
		t.Errorf("missing file = %v, %v", ask, err)
	}
}
