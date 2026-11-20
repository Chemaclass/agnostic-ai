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

	settings, err := approvals.ReadClaudeSettings(approvals.ClaudeSettingsFiles(cwd, project, user, managed))
	if err != nil {
		t.Fatal(err)
	}
	got := settings.Ask
	for _, want := range []string{"Bash(user*)", "Bash(managed*)", "Bash(dropin*)", "Bash(project*)", "Bash(local*)"} {
		if !slices.Contains(got, want) {
			t.Errorf("ask rules %v miss %s", got, want)
		}
	}
	if slices.Contains(got, "Bash(ls)") {
		t.Errorf("ask rules %v hold an allow rule", got)
	}
	if !slices.Contains(settings.RuleRoots, project) || slices.Contains(settings.RuleRoots, cwd) {
		t.Errorf("rule roots = %v, want the project that holds ask rules", settings.RuleRoots)
	}
}

func TestReadClaudeSettings_SymlinkedCwdFindsTheRealProject(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "repo")
	writeFile(t, filepath.Join(project, "pkg", "a.txt"), "")
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go test*)"]}}`)
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join(project, "pkg"), link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	settings, err := approvals.ReadClaudeSettings(approvals.ClaudeSettingsFiles(link, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(settings.Ask, "Bash(go test*)") {
		t.Errorf("ask rules %v miss the real project's rule", settings.Ask)
	}
}

func TestReadClaudeSettings_ProjectEnvThatMovesThePolicy(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "home", ".claude", "settings.json")
	project := filepath.Join(root, "repo")
	writeFile(t, user, `{"env":{"AGNOSTIC_AI_HOME":"/home/me/.agnostic-ai","AGNOSTIC_AI_TARGET":"claude"}}`)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"env":{"AGNOSTIC_AI_HOME":"/tmp/fake","FOO":"1"}}`)
	settings, err := approvals.ReadClaudeSettings(approvals.ClaudeSettingsFiles(project, "", user, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(settings.MovedEnv, []string{"AGNOSTIC_AI_HOME"}) {
		t.Errorf("moved env = %v, want only the project's AGNOSTIC_AI_HOME", settings.MovedEnv)
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

	got, err := approvals.ReadClaudeSettings(approvals.ClaudeSettingsFiles(linked, "", "", ""))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Ask, "Bash(echo main*)") {
		t.Errorf("ask rules %v miss the main checkout's local rule", got.Ask)
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
			if _, err := approvals.ReadClaudeSettings([]approvals.ClaudeSettingsFile{{Path: path}}); err == nil {
				t.Error("want an error, so the hook asks")
			}
		})
	}
	if got, err := approvals.ReadClaudeSettings([]approvals.ClaudeSettingsFile{{Path: filepath.Join(dir, "missing.json")}}); err != nil || len(got.Ask) != 0 {
		t.Errorf("missing file = %v, %v; want nothing", got, err)
	}
}

func TestClaudeManagedDir(t *testing.T) {
	if approvals.ClaudeManagedDir("darwin") != "/Library/Application Support/ClaudeCode" || approvals.ClaudeManagedDir("linux") != "/etc/claude-code" || approvals.ClaudeManagedDir("windows") != "" {
		t.Error("managed settings folders do not match code.claude.com/docs/en/managed-settings.md")
	}
}
