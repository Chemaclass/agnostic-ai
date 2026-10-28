package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSplitSourceRef(t *testing.T) {
	cases := []struct {
		in, src, ref string
	}{
		{"github.com/foo/bar", "github.com/foo/bar", ""},
		{"github.com/foo/bar@v1.2.0", "github.com/foo/bar", "v1.2.0"},
		{"git@host:foo/bar@v1", "git@host:foo/bar", "v1"},
		{"./local/pack", "./local/pack", ""},
	}
	for _, c := range cases {
		gotSrc, gotRef := splitSourceRef(c.in)
		if gotSrc != c.src || gotRef != c.ref {
			t.Errorf("splitSourceRef(%q)=(%q,%q), want (%q,%q)", c.in, gotSrc, gotRef, c.src, c.ref)
		}
	}
}

func TestDerivePackName(t *testing.T) {
	cases := map[string]string{
		"github.com/chemaclass/go-rules":     "go-rules",
		"https://github.com/foo/bar.git":     "bar",
		"git@github.com:foo/baz":             "baz",
		"./local/pack":                       "pack",
		"file:///tmp/sample-pack/":           "sample-pack",
		"https://example.com/team/security/": "security",
	}
	for in, want := range cases {
		if got := derivePackName(in); got != want {
			t.Errorf("derivePackName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestValidatePackName(t *testing.T) {
	good := []string{"foo", "go-rules", "team_security", "v1"}
	for _, n := range good {
		if err := validatePackName(n); err != nil {
			t.Errorf("validatePackName(%q) unexpected err: %v", n, err)
		}
	}
	bad := []string{"", ".", "..", "foo/bar", `foo\bar`}
	for _, n := range bad {
		if err := validatePackName(n); err == nil {
			t.Errorf("validatePackName(%q) accepted, expected error", n)
		}
	}
}

func TestPacksLifecycle_LocalSource(t *testing.T) {
	root := testutil.TempCwd(t)

	src := filepath.Join(t.TempDir(), "go-rules")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nbody")
	mustWrite(t, filepath.Join(src, "agents", "y.md"), "---\nname: y\n---\nbody")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	if !strings.Contains(out.String(), "added go-rules") {
		t.Errorf("add output: %q", out.String())
	}

	dest := filepath.Join(root, packsDir, "go-rules", "rules", "x.md")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("expected copied rule at %s: %v", dest, err)
	}

	lock, err := readPacksLock(root)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if len(lock.Packs) != 1 || lock.Packs[0].Name != "go-rules" {
		t.Fatalf("lock state: %+v", lock.Packs)
	}

	out.Reset()
	if err := runPacksList(root, &out); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out.String(), "go-rules") {
		t.Errorf("list output: %q", out.String())
	}

	out.Reset()
	if err := runPacksRemove(root, "go-rules", &out); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, packsDir, "go-rules")); !os.IsNotExist(err) {
		t.Errorf("expected pack dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, packsLockfile)); !os.IsNotExist(err) {
		t.Errorf("expected empty lock removed, stat err=%v", err)
	}
}

func TestPacksRemove_NotInstalled(t *testing.T) {
	root := testutil.TempCwd(t)
	var out bytes.Buffer
	if err := runPacksRemove(root, "missing", &out); err == nil {
		t.Fatal("expected error removing missing pack")
	}
}

func TestPacksUpdate_LocalSource(t *testing.T) {
	root := testutil.TempCwd(t)

	src := filepath.Join(t.TempDir(), "go-rules")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nv1")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nv2")

	out.Reset()
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, packsDir, "go-rules", "rules", "x.md"))
	if !strings.Contains(string(got), "v2") {
		t.Errorf("expected v2 after update, got %q", got)
	}
}

func TestResolveLayers_PacksLayerInsertedBeforeProject(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(t.TempDir(), "p1")
	mustWrite(t, filepath.Join(src, "rules", "r.md"), "---\nname: r\n---\nb")

	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}

	cfg := &config.Config{Sources: defaultLayerSources()}
	layers, err := resolveLayers(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers (pack + project), got %d (%+v)", len(layers), layers)
	}
	if !strings.HasPrefix(layers[0].Name, layerNamePackPrefix) {
		t.Errorf("layer[0]=%q, want pack:* (lower precedence than project)", layers[0].Name)
	}
	if layers[1].Name != layerNameProject {
		t.Errorf("layer[1]=%q, want %q", layers[1].Name, layerNameProject)
	}
}

func TestResolveLayers_PackRuleFoldersUseConsumingProjectDirectories(t *testing.T) {
	root := testutil.TempCwd(t)
	src := filepath.Join(t.TempDir(), "module-rules")
	mustWrite(t, filepath.Join(src, "rules", "modules", "grouped.md"), "---\nname: grouped\n---\nGrouped.\n")
	mustWrite(t, filepath.Join(src, "modules", "module.go"), "package modules\n")
	mustWrite(t, filepath.Join(src, "rules", "backend", "auth.md"), "---\nname: auth\n---\nAuth.\n")
	mustWrite(t, filepath.Join(src, "reviews", "review.md"), "---\nname: review\n---\n@missing.md\n")
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runPacksAdd(root, src, "", &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	layers, err := resolveLayers(root, &config.Config{Sources: defaultLayerSources()})
	if err != nil {
		t.Fatal(err)
	}
	b, err := spec.LoadLayered(layers)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Rules) != 2 {
		t.Fatalf("expected two rules, got %d", len(b.Rules))
	}
	for _, r := range b.Rules {
		want := ""
		if r.Name == "auth" {
			want = "backend"
		}
		if got, err := spec.RuleScope(r); err != nil || got != want {
			t.Errorf("%s: RuleScope() = %q, %v; want %q", r.Name, got, err, want)
		}
	}
	if len(b.Reviews) != 1 || b.Reviews[0].Body != "@missing.md\n" {
		t.Errorf("pack review includes must remain literal, got %+v", b.Reviews)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const gitPackSource = "example.test/pack"

// gitPackRemote is a local repository that a pack source of gitPackSource
// reaches through an isolated Git config.
type gitPackRemote struct {
	t   *testing.T
	dir string
}

func newGitPackRemote(t *testing.T, extraConfig ...string) *gitPackRemote {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := filepath.Join(t.TempDir(), "remote")
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	url := "file://" + filepath.ToSlash(dir)
	if !strings.HasPrefix(filepath.ToSlash(dir), "/") {
		url = "file:///" + filepath.ToSlash(dir)
	}
	mustWrite(t, cfg, "[url \""+url+"\"]\n\tinsteadOf = https://"+gitPackSource+"\n"+strings.Join(extraConfig, "\n"))
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "test")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "test@example.test")
	}
	r := &gitPackRemote{t: t, dir: dir}
	r.git("init", "--quiet", "-b", "main", dir)
	return r
}

func (r *gitPackRemote) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	if args[0] != "init" {
		cmd.Dir = r.dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *gitPackRemote) commit(body string) string {
	r.t.Helper()
	mustWrite(r.t, filepath.Join(r.dir, "rules", "x.md"), body)
	r.git("add", "-A")
	r.git("commit", "--quiet", "-m", body)
	return r.git("rev-parse", "HEAD")
}

func installedRule(t *testing.T, root string) string {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, packsDir, "p", "rules", "x.md"))
	if err != nil {
		t.Fatalf("read installed pack: %v", err)
	}
	return string(got)
}

func lockedSha(t *testing.T, root string) string {
	t.Helper()
	lock, err := readPacksLock(root)
	if err != nil || len(lock.Packs) != 1 {
		t.Fatalf("read lock: %v %+v", err, lock)
	}
	return lock.Packs[0].Sha
}

func TestPacksUpdate_InstallsRecordedCommitAfterTagMoves(t *testing.T) {
	remote := newGitPackRemote(t)
	pinned := remote.commit("v1")
	remote.git("tag", "--no-sign", "v1")
	remote.commit("moved")
	remote.git("tag", "--no-sign", "-f", "v1")

	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "v1", Sha: pinned})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := installedRule(t, root); got != "v1" {
		t.Errorf("installed %q, want the recorded commit's content v1", got)
	}
	if got := lockedSha(t, root); got != pinned {
		t.Errorf("lock sha = %s, want %s", got, pinned)
	}
}

func TestPacksUpdate_ForcePushedBranch(t *testing.T) {
	t.Run("installs the recorded commit while the remote still has it", func(t *testing.T) {
		remote := newGitPackRemote(t)
		pinned := remote.commit("pinned")
		remote.git("update-ref", "refs/keep/pinned", pinned)
		remote.git("checkout", "--quiet", "--orphan", "rewrite")
		remote.commit("rewritten")
		remote.git("branch", "--quiet", "-M", "main")

		root := testutil.TempCwd(t)
		writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "main", Sha: pinned})

		var out bytes.Buffer
		if err := runPacksUpdate(root, "", false, &out); err != nil {
			t.Fatalf("update: %v", err)
		}
		if got := installedRule(t, root); got != "pinned" {
			t.Errorf("installed %q, want pinned", got)
		}
	})

	t.Run("fails and keeps the lockfile when the commit is gone", func(t *testing.T) {
		remote := newGitPackRemote(t)
		pinned := remote.commit("pinned")
		remote.git("checkout", "--quiet", "--orphan", "rewrite")
		remote.commit("rewritten")
		remote.git("branch", "--quiet", "-M", "main")
		remote.git("reflog", "expire", "--expire=now", "--all")
		remote.git("gc", "--quiet", "--prune=now")

		root := testutil.TempCwd(t)
		writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "main", Sha: pinned})
		before, _ := os.ReadFile(filepath.Join(root, packsLockfile))

		var out bytes.Buffer
		err := runPacksUpdate(root, "", false, &out)
		if err == nil || !strings.Contains(err.Error(), short(pinned)) || !strings.Contains(err.Error(), "--latest") {
			t.Fatalf("err=%v, want a failure that names the recorded commit and --latest", err)
		}
		after, _ := os.ReadFile(filepath.Join(root, packsLockfile))
		if !bytes.Equal(before, after) {
			t.Errorf("lockfile changed after a failed restore:\n%s", after)
		}
	})
}

func TestPacksUpdate_RestoresAnnotatedTagOverProtocolV0(t *testing.T) {
	remote := newGitPackRemote(t, "[protocol]\n\tversion = 0\n")
	pinned := remote.commit("v1")
	remote.git("tag", "--no-sign", "-a", "-m", "v1", "v1")
	remote.commit("later")

	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "v1", Sha: pinned})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := installedRule(t, root); got != "v1" {
		t.Errorf("installed %q, want v1", got)
	}
}

func TestPacksUpdate_FailedRestoreLeavesNoEmptyPack(t *testing.T) {
	remote := newGitPackRemote(t)
	remote.commit("tip")
	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "main", Sha: strings.Repeat("0", 40)})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err == nil {
		t.Fatal("expected update to fail for a commit the remote does not have")
	}
	if _, err := os.Stat(filepath.Join(root, packsDir, "p")); !os.IsNotExist(err) {
		t.Errorf("failed restore left a pack folder behind, stat err=%v", err)
	}
}

func TestPacksUpdate_LatestMovesToTheRefTipAndShowsBothCommits(t *testing.T) {
	remote := newGitPackRemote(t)
	pinned := remote.commit("v1")
	remote.git("tag", "--no-sign", "v1")
	moved := remote.commit("moved")
	remote.git("tag", "--no-sign", "-f", "v1")

	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Ref: "v1", Sha: pinned})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", true, &out); err != nil {
		t.Fatalf("update --latest: %v", err)
	}
	if got := installedRule(t, root); got != "moved" {
		t.Errorf("installed %q, want moved", got)
	}
	if got := lockedSha(t, root); got != moved {
		t.Errorf("lock sha = %s, want %s", got, moved)
	}
	if !strings.Contains(out.String(), short(pinned)) || !strings.Contains(out.String(), short(moved)) {
		t.Errorf("output %q must show the old and new commits", out.String())
	}
}

func TestPacksUpdate_RecordsCommitWhenLockfileHasNone(t *testing.T) {
	remote := newGitPackRemote(t)
	tip := remote.commit("tip")

	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := lockedSha(t, root); got != tip {
		t.Errorf("lock sha = %s, want %s", got, tip)
	}
}

func TestPacksUpdate_RefusesRecordedShaThatIsNotACommitHash(t *testing.T) {
	remote := newGitPackRemote(t)
	remote.commit("tip")
	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: "p", Source: gitPackSource, Sha: "+refs/*:refs/*"})

	var out bytes.Buffer
	err := runPacksUpdate(root, "", false, &out)
	if err == nil || !strings.Contains(err.Error(), "not a commit hash") {
		t.Fatalf("err=%v, want a refusal of a sha that is not a commit hash", err)
	}
}

func writeLock(t *testing.T, root string, entries ...packEntry) {
	t.Helper()
	if err := writePacksLock(root, &packsLock{Version: 1, Packs: entries}); err != nil {
		t.Fatal(err)
	}
}
