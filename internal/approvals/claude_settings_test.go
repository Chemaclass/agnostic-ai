package approvals_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeAskRules_ReadsEveryScope(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "home", ".claude", "settings.json")
	managed := filepath.Join(root, "managed")
	project := filepath.Join(root, "repo")
	cwd := filepath.Join(project, "sub")
	writeFile(t, user, `{"permissions":{"ask":["Bash(user*)"]}}`)
	writeFile(t, filepath.Join(managed, "managed-settings.json"), `{"permissions":{"ask":["Bash(managed*)"]}}`)
	writeFile(t, filepath.Join(managed, "managed-settings.d", "10-team.json"), `{"permissions":{"ask":["Bash(dropin*)"]}}`)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(project*)"],"allow":["Bash(ls)"]}}`)
	writeFile(t, filepath.Join(project, ".claude", "settings.local.json"), `{"permissions":{"ask":["Bash(local*)"]}}`)
	writeFile(t, filepath.Join(cwd, ".claude", "settings.json"), `{"model":"x"}`)

	files := approvals.ClaudeSettingsFiles(cwd, project, user, managed)
	got, err := approvals.ClaudeAskRules(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Bash(user*)", "Bash(managed*)", "Bash(dropin*)", "Bash(project*)", "Bash(local*)"} {
		if !slices.Contains(got, want) {
			t.Errorf("ask rules %v miss %s", got, want)
		}
	}
	if slices.Contains(got, "Bash(ls)") {
		t.Errorf("ask rules %v hold an allow rule", got)
	}
}

func TestClaudeAskRules_LinkedWorktreeReadsMainCheckout(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main")
	linked := filepath.Join(root, "linked")
	gitdir := filepath.Join(main, ".git", "worktrees", "linked")
	writeFile(t, filepath.Join(gitdir, "commondir"), "../..\n")
	writeFile(t, filepath.Join(linked, ".git"), "gitdir: "+gitdir+"\n")
	writeFile(t, filepath.Join(main, ".claude", "settings.local.json"), `{"permissions":{"ask":["Bash(echo main*)"]}}`)

	got, err := approvals.ClaudeAskRules(approvals.ClaudeSettingsFiles(linked, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "Bash(echo main*)") {
		t.Errorf("ask rules %v miss the main checkout's local rule", got)
	}
}

func TestClaudeAskRules_BrokenFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"not-json":     `{"permissions":`,
		"ask-not-list": `{"permissions":{"ask":"Bash(x)"}}`,
		"empty":        ``,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			writeFile(t, path, body)
			if _, err := approvals.ClaudeAskRules([]string{path}); err == nil {
				t.Error("want an error, so the hook asks")
			}
		})
	}
	if got, err := approvals.ClaudeAskRules([]string{filepath.Join(dir, "missing.json")}); err != nil || len(got) != 0 {
		t.Errorf("missing file = %v, %v; want nothing", got, err)
	}
}

func TestClaudeManagedDir(t *testing.T) {
	if approvals.ClaudeManagedDir("darwin") != "/Library/Application Support/ClaudeCode" || approvals.ClaudeManagedDir("linux") != "/etc/claude-code" || approvals.ClaudeManagedDir("windows") != "" {
		t.Error("managed settings folders do not match code.claude.com/docs/en/managed-settings.md")
	}
}
