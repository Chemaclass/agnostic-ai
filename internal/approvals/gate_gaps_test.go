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

func TestDecide_RtkEvasionsKeepBashAskRules(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	p.AlwaysAsk = []string{"Bash(go test:*)", "Bash(git log*)"}
	for _, command := range []string{
		" rtk go test ./...", "\trtk go test ./...", `"rtk" go test ./...`, `'rtk' go test ./...`,
		`r\tk go test ./...`, `rtk "go" test ./...`, `rtk g\o test ./...`, `"rtk" git log`,
	} {
		if got := Decide(bash(command), p); got.Outcome != Asked {
			t.Errorf("%q: got %+v, want asked", command, got)
		}
	}
}

func TestDecide_ToolFoldersUnderHomeAsk(t *testing.T) {
	t.Parallel()
	for root, file := range map[string]string{
		testHome + "/.claude":       "settings.json",
		testHome + "/.codex":        "config.toml",
		testHome + "/.config":       "git/config",
		testHome + "/.config/fish":  "config.fish",
		testHome + "/Library":       "LaunchAgents/x.plist",
		testHome + "/go":            "bin/agnostic-ai",
		testHome + "/.local":        "bin/git",
		testHome + "/.cargo":        "bin/x",
		testHome + "/.agnostic-ai2": "notes.md",
	} {
		p := testPolicy()
		p.ProjectRoot = root
		r := call("Write", map[string]any{"file_path": root + "/" + file})
		r.Cwd = root
		want := RuleProtectedPath
		if file == "notes.md" {
			want = ""
		}
		wantDecision(t, Decide(r, p), want)
	}
}

func TestDecide_PathRulesThroughALinkedRoot(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "private"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(real, "private"), filepath.Join(real, "pub"))
	link := filepath.Join(base, "link")
	mustSymlink(t, real, link)
	p := testPolicy()
	p.ProjectRoot = link
	p.Resolve = ResolveLinks
	p.AlwaysAsk = []string{"Edit(./private/**)"}
	for _, file := range []string{filepath.Join(link, "pub", "x"), filepath.Join(real, "private", "x"), filepath.Join(link, "private", "x")} {
		r := call("Write", map[string]any{"file_path": file})
		r.Cwd = link
		wantDecision(t, Decide(r, p), RuleAlwaysAsk)
	}
}

func TestDecide_HomeRuleMatchesInAnyCase(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	p := testPolicy()
	p.Home = home
	p.ProjectRoot = filepath.Join(home, "proj")
	p.AlwaysAsk = []string{"Edit(~/proj/private/**)"}
	r := call("Write", map[string]any{"file_path": filepath.Join(home, "proj", "PRIVATE", "x")})
	r.Cwd = p.ProjectRoot
	wantDecision(t, Decide(r, p), RuleAlwaysAsk)
}

func TestDecide_LinkedHomeAsProjectAsks(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	real := filepath.Join(base, "realhome")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, real, filepath.Join(base, "home"))
	p := testPolicy()
	p.Home = filepath.Join(base, "home")
	p.ProjectRoot = real
	p.Resolve = ResolveLinks
	r := call("Write", map[string]any{"file_path": filepath.Join(real, "notes.txt")})
	r.Cwd = real
	wantDecision(t, Decide(r, p), RuleOutsideProject)
}

func TestDecide_GitConfigReadsAndGlobsAsk(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	wantDecision(t, Decide(call("Read", map[string]any{"file_path": testRoot + "/.gitconfig"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Read", map[string]any{"file_path": testRoot + "/.config/git/config"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Grep", map[string]any{"pattern": "x", "glob": ".git/config"}), p), RuleProtectedPath)
	wantDecision(t, Decide(call("Grep", map[string]any{"pattern": "x", "glob": "**/*.go"}), p), "")
}

func TestDotEnvSetsPolicy_DotenvForms(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		"AGNOSTIC_AI_HOME=/x", "export\tAGNOSTIC_AI_HOME=/x", "AGNOSTIC_AI_HOME: /x", "A=1\rAGNOSTIC_AI_HOME=/x",
		"\uFEFFAGNOSTIC_AI_HOME=/x", "  AGNOSTIC_AI_HOME = /x", "agnostic_ai_home=/x", "HOME=/x", "USERPROFILE=/x",
	} {
		if !dotEnvSetsPolicy(text) {
			t.Errorf("%q: not caught", text)
		}
	}
	for _, text := range []string{"# AGNOSTIC_AI_HOME=/x", "FOO=AGNOSTIC_AI_HOME", "MY_HOME=/x"} {
		if dotEnvSetsPolicy(text) {
			t.Errorf("%q: caught, want not", text)
		}
	}
}

func TestDecide_ToolFoldersInsideAProjectDoNotProtectIt(t *testing.T) {
	t.Parallel()
	for _, root := range []string{
		testHome + "/Code/proj/.claude/worktrees/x", testHome + "/Code/.idea/p",
		testHome + "/.codex/worktrees/ab12/proj", testHome + "/.cursor/worktrees/proj/x1",
		testHome + "/Code/bin", testHome + "/go/src/github.com/a/bin",
	} {
		p := testPolicy()
		p.ProjectRoot = root
		r := call("Write", map[string]any{"file_path": root + "/main.go"})
		r.Cwd = root
		wantDecision(t, Decide(r, p), "")
	}
}

func TestDotEnvSetsPolicy_UnicodeBlanks(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		" AGNOSTIC_AI_HOME=/x", "\vAGNOSTIC_AI_HOME=/x", " AGNOSTIC_AI_HOME=/x", "#c AGNOSTIC_AI_HOME=/x",
		"export AGNOSTIC_AI_HOME=/x", "AGNOSTIC_AI_HOME =/x", "AGNOSTIC_AI_HOME: /x",
	} {
		if !dotEnvSetsPolicy(text) {
			t.Errorf("%q: not caught", text)
		}
	}
}

func TestDecide_GitGlobWildcardsAsk(t *testing.T) {
	t.Parallel()
	p := testPolicy()
	for _, glob := range []string{".gi?/config", ".git*/config", ".[g]it/config", "*.gitconfig", "**/.git/**",
		"sub/.gi?/config", "a/b/.gi?/config", "sub/.gitconfi?", "sub/.config/gi?/config", ".gi?/FETCH_HEAD",
		".gi?/packed-refs", ".gi?/modules/*/config"} {
		if got := Decide(call("Grep", map[string]any{"pattern": "x", "glob": glob}), p); got.Outcome != Asked {
			t.Errorf("%q: got %+v, want asked", glob, got)
		}
	}
	wantDecision(t, Decide(call("Grep", map[string]any{"pattern": "x", "glob": "src/*.go"}), p), "")
}
