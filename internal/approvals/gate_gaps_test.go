package approvals

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecide_PathAskRulesCoverShellCommands(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	p.AlwaysAsk = []string{"Read(./private/**)"}
	for _, command := range []string{"cat private/x", "head private/x", "grep key private/x", "sed -n p private/x", "ls"} {
		wantDecision(t, Decide(bash(command), p), RuleAlwaysAsk)
	}
	p.AlwaysAsk = []string{"Edit(./Makefile)"}
	wantDecision(t, Decide(bash("go build -o Makefile ./sub"), p), RuleAlwaysAsk)
	p.AlwaysAsk = []string{"Write(src/**)"}
	wantDecision(t, Decide(bash("go test ./..."), p), RuleAlwaysAsk)

	p.AlwaysAsk = []string{"Read(*)", "Edit", "Bash(make deploy*)"}
	wantDecision(t, Decide(bash("go test ./..."), p), "")
}

func TestDecide_RtkPrefixKeepsBashAskRules(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	p.AlwaysAsk = []string{"Bash(git commit*)", "Bash(go test:*)"}
	wantDecision(t, Decide(bash("rtk git commit -m x"), p), RuleAlwaysAsk)
	wantDecision(t, Decide(bash("go vet ./... && rtk go test ./..."), p), RuleAlwaysAsk)
	wantDecision(t, Decide(bash("rtk go vet ./..."), p), "")
}

func TestDecide_HomeAsProjectAsks(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	p.ProjectRoot = testHome
	r := call("Write", map[string]any{"file_path": testHome + "/notes.txt"})
	r.Cwd = testHome
	wantDecision(t, Decide(r, p), RuleOutsideProject)
	p.ProjectRoot = "/"
	r.Cwd = "/"
	wantDecision(t, Decide(r, p), RuleOutsideProject)

	p = testPolicy()
	wantDecision(t, Decide(call("Write", map[string]any{"file_path": testRoot + "/.gitconfig"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Write", map[string]any{"file_path": testRoot + "/sub/.config/git/config"}), p), RuleProtectedPath)
}

func TestDecide_PathAskRulesFollowLinksAndCase(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if err := os.MkdirAll(filepath.Join(root, "private"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(root, "private"), filepath.Join(root, "pub"))
	p := testPolicy()
	p.ProjectRoot = root
	p.Resolve = ResolveLinks
	p.AlwaysAsk = []string{"Edit(./private/**)"}
	write := func(file string) Request {
		r := call("Write", map[string]any{"file_path": file})
		r.Cwd = root
		return r
	}
	wantDecision(t, Decide(write(filepath.Join(root, "pub", "x")), p), RuleAlwaysAsk)
	wantDecision(t, Decide(write(filepath.Join(root, "PRIVATE", "x")), p), RuleAlwaysAsk)
	wantDecision(t, Decide(write(filepath.Join(root, "public", "x")), p), "")
}

func TestDecide_SecretThroughALinkAsks(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(root, ".env"), filepath.Join(root, "docs", "config.txt"))
	p := testPolicy()
	p.ProjectRoot = root
	p.Resolve = ResolveLinks
	in := func(r Request) Request {
		r.Cwd = root
		return r
	}
	wantDecision(t, Decide(in(bash("cat docs/config.txt")), p), RuleSecret)
	wantDecision(t, Decide(in(call("Read", map[string]any{"file_path": filepath.Join(root, "docs", "config.txt")})), p), RuleSecret)
	wantDecision(t, Decide(in(call("Write", map[string]any{"file_path": filepath.Join(root, "docs", "config.txt")})), p), RuleSecret)
}

func TestDecide_ReadOfGitInternalsAsks(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	wantDecision(t, Decide(call("Read", map[string]any{"file_path": testRoot + "/.git/config"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Read", map[string]any{"file_path": testRoot + "/sub/.GIT/config"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Read", map[string]any{"file_path": testRoot + "/.github/x.yml"}), p), "")
}
