package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	bad := []string{"", ".", "..", "foo/bar", `foo\bar`, ".git", ".GIT", ".hidden"}
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

	src := filepath.Join(root, "shared", "go-rules")
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

func TestPacksUpdateAndRemove_RefuseALockedNameOutsideThePacksFolder(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "proj")
	victim := filepath.Join(base, "victim", "data")
	mustWrite(t, victim, "keep\n")
	mustWrite(t, filepath.Join(root, packsLockfile), "version: 1\npacks:\n  - name: ../../../victim\n    source: "+filepath.ToSlash(t.TempDir())+"\n")
	testutil.Chdir(t, root)

	var out bytes.Buffer
	updateErr := runPacksUpdate(root, "", false, &out)
	removeErr := runPacksRemove(root, "../../../victim", &out)

	for name, err := range map[string]error{"update": updateErr, "remove": removeErr} {
		if err == nil || !strings.Contains(err.Error(), packsLockfile) {
			t.Errorf("%s error = %v, want one naming %s", name, err, packsLockfile)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("a locked pack name removed a folder outside the project: %v", err)
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

func TestPacksUpdate_RefusesLockfileNameOutsidePacksDir(t *testing.T) {
	for _, name := range []string{"../victim", "a/b", "..", `a\b`, ""} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			victim := filepath.Join(parent, "victim")
			mustWrite(t, filepath.Join(victim, "keep.txt"), "keep")
			mustWrite(t, filepath.Join(root, "payload", "rules", "x.md"), "---\nname: x\n---\nb")
			testutil.Chdir(t, root)
			writeLock(t, root, packEntry{Name: name, Source: "./payload"})

			var out bytes.Buffer
			err := runPacksUpdate(root, "", false, &out)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", name)) {
				t.Fatalf("err=%v, want a refusal that names the entry %q", err, name)
			}
			if _, err := os.Stat(filepath.Join(victim, "keep.txt")); err != nil {
				t.Errorf("update removed a folder outside the project: %v", err)
			}
			if _, err := os.Stat(filepath.Join(parent, "rules")); !os.IsNotExist(err) {
				t.Errorf("update wrote outside the project, stat err=%v", err)
			}
		})
	}
}

func TestPacksUpdate_ChecksEveryEntryBeforeChangingAny(t *testing.T) {
	root := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(root, "good", "rules", "x.md"), "---\nname: x\n---\nnew")
	mustWrite(t, filepath.Join(root, packsDir, "a-good", "rules", "x.md"), "old")
	writeLock(t, root,
		packEntry{Name: "a-good", Source: "./good"},
		packEntry{Name: "z-bad/x", Source: "./good"},
	)

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err == nil {
		t.Fatal("expected update to refuse the lockfile")
	}
	got, _ := os.ReadFile(filepath.Join(root, packsDir, "a-good", "rules", "x.md"))
	if string(got) != "old" {
		t.Errorf("update changed a pack before refusing the lockfile, got %q", got)
	}
}

func TestPacksUpdate_RefusesSymlinkedPacksDir(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	outside := filepath.Join(parent, "outside")
	mustWrite(t, filepath.Join(outside, "p", "keep.txt"), "keep")
	mustWrite(t, filepath.Join(root, "payload", "rules", "x.md"), "b")
	if err := os.MkdirAll(filepath.Join(root, ".agnostic-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, packsDir)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	testutil.Chdir(t, root)
	writeLock(t, root, packEntry{Name: "p", Source: "./payload"})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err == nil {
		t.Fatal("expected update to refuse a packs folder that links outside the project")
	}
	if _, err := os.Stat(filepath.Join(outside, "p", "keep.txt")); err != nil {
		t.Errorf("update removed a folder outside the project: %v", err)
	}
}

func TestPacksUpdate_RefusesOptionLikeSourceBeforeGitRuns(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the upload-pack command needs a POSIX shell")
	}
	root := testutil.TempCwd(t)
	marker := filepath.Join(t.TempDir(), "ran")
	bare := filepath.Join(root, packsDir, "p.git")
	if err := exec.Command("git", "init", "--quiet", "--bare", bare).Run(); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}
	writeLock(t, root, packEntry{Name: "p", Source: "--upload-pack=touch " + marker + ";://"})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err == nil {
		t.Fatal("expected update to refuse a source that starts with -")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("git ran the command from the lockfile source, stat err=%v", err)
	}
}

func TestPacksUpdate_RefusesUnsupportedSources(t *testing.T) {
	for _, source := range []string{
		"-c",
		"ext::sh -c touch% /tmp/x",
		"http://example.com/pack",
		"git://example.com/pack",
		"fd::17",
		"git@/../../outside",
		"git@:foo/bar",
		"git@host/path",
	} {
		t.Run(source, func(t *testing.T) {
			if err := checkLockedSource(t.TempDir(), source); err == nil {
				t.Errorf("checkLockedSource(%q) accepted, expected error", source)
			}
		})
	}
}

func TestPacksUpdate_AcceptsSupportedSources(t *testing.T) {
	for _, source := range []string{
		"github.com/foo/bar",
		"https://github.com/foo/bar.git",
		"ssh://git@github.com/foo/bar.git",
		"git@github.com:foo/bar.git",
	} {
		if err := checkLockedSource(t.TempDir(), source); err != nil {
			t.Errorf("checkLockedSource(%q) = %v, want nil", source, err)
		}
	}
}

func TestPacksUpdate_RefusesWindowsNetworkPathBeforeOpeningIt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("network share paths are absolute only on Windows")
	}
	for _, source := range []string{`\\attacker\share\x`, "//attacker/share/x", "file:////attacker/share"} {
		if err := checkLockedSource(t.TempDir(), source); err == nil {
			t.Errorf("checkLockedSource(%q) accepted, expected error", source)
		}
	}
}

func TestPacksUpdate_RefusesLocalSourceHoldingPacksDir(t *testing.T) {
	root := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(root, packsDir, "keep.txt"), "x")
	for _, source := range []string{"./", "./.agnostic-ai", "./" + packsDir, "./.AGNOSTIC-AI"} {
		if err := checkLockedSource(root, source); err == nil {
			t.Errorf("checkLockedSource(%q) accepted a folder that holds the packs folder", source)
		}
	}
}

func TestPacksUpdate_AbsoluteLocalSourceInsideProjectWithDotRoot(t *testing.T) {
	root := testutil.TempCwd(t)
	src := filepath.Join(root, "shared", "p")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "v2")
	writeLock(t, ".", packEntry{Name: "p", Source: src}, packEntry{Name: "q", Source: "../" + filepath.Base(root) + "/shared/p"})

	var out bytes.Buffer
	if err := runPacksUpdate(".", "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	for _, name := range []string{"p", "q"} {
		if _, err := os.Stat(filepath.Join(root, packsDir, name, "rules", "x.md")); err != nil {
			t.Errorf("pack %s not updated: %v", name, err)
		}
	}
}

func TestPacksUpdate_SkipsOutsideLocalSourceAndUpdatesTheRest(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	mustWrite(t, filepath.Join(parent, "elsewhere", "rules", "x.md"), "b")
	mustWrite(t, filepath.Join(root, "inside", "rules", "x.md"), "new")
	testutil.Chdir(t, root)
	writeLock(t, root,
		packEntry{Name: "far", Source: "../elsewhere"},
		packEntry{Name: "near", Source: "./inside"},
	)

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out.String(), "packs add '../elsewhere' --name 'far'") {
		t.Errorf("expected a skip notice naming the packs add command, got %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, packsDir, "near", "rules", "x.md")); err != nil {
		t.Errorf("inside pack not updated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, packsDir, "far")); !os.IsNotExist(err) {
		t.Errorf("update copied a folder from outside the project, stat err=%v", err)
	}
}

func TestResolvePacksLayers_SkipsInvalidLockfileNames(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, packEntry{Name: "../other", Source: "./x"})
	if err := os.MkdirAll(filepath.Join(root, ".agnostic-ai", "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if layers := resolvePacksLayers(root); len(layers) != 0 {
		t.Errorf("resolvePacksLayers loaded a layer outside the packs folder: %+v", layers)
	}
}

func TestPacksUpdate_RefusesLocalSourceOutsideProject(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	mustWrite(t, filepath.Join(parent, "elsewhere", "rules", "x.md"), "b")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)
	for _, source := range []string{"../elsewhere", filepath.Join(parent, "elsewhere"), "file://" + filepath.Join(parent, "elsewhere")} {
		writeLock(t, root, packEntry{Name: "p", Source: source})
		var out bytes.Buffer
		err := runPacksUpdate(root, "p", false, &out)
		if err == nil || !strings.Contains(err.Error(), "--name 'p'") {
			t.Errorf("source %q: err=%v, want a refusal that points to packs add", source, err)
		}
		if _, statErr := os.Stat(filepath.Join(root, packsDir, "p")); !os.IsNotExist(statErr) {
			t.Errorf("source %q: update copied a folder from outside the project", source)
		}
	}
}

func TestPacksRemove_RefusesLockfileNameOutsidePacksDir(t *testing.T) {
	for _, name := range []string{"../victim", "a/b", "..", `a\b`, ""} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "project")
			victim := filepath.Join(parent, "victim")
			mustWrite(t, filepath.Join(victim, "keep.txt"), "keep")
			mustWrite(t, filepath.Join(root, packsDir, "other", "keep.txt"), "keep")
			testutil.Chdir(t, root)
			writeLock(t, root, packEntry{Name: name, Source: "./x"}, packEntry{Name: "other", Source: "./x"})

			var out bytes.Buffer
			err := runPacksRemove(root, name, &out)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("%q", name)) {
				t.Fatalf("err=%v, want a refusal that names the entry %q", err, name)
			}
			for _, kept := range []string{filepath.Join(victim, "keep.txt"), filepath.Join(root, packsDir, "other", "keep.txt")} {
				if _, err := os.Stat(kept); err != nil {
					t.Errorf("remove deleted %s: %v", kept, err)
				}
			}
		})
	}
}

func TestGitClone_RefusesOptionLikeSourceOrRef(t *testing.T) {
	for _, c := range []struct{ source, ref string }{
		{"--upload-pack=x", ""},
		{"https://example.com/p", "--upload-pack=x"},
	} {
		if _, err := gitClone(c.source, c.ref, filepath.Join(t.TempDir(), "dest")); err == nil || !strings.Contains(err.Error(), "must not start with -") {
			t.Errorf("gitClone(%q, %q) err=%v, want a refusal before git runs", c.source, c.ref, err)
		}
	}
}

func TestGitCloneArgs_DisablesExtTransport(t *testing.T) {
	args := gitCloneArgs("https://example.com/p", "", "dest")
	if len(args) < 3 || args[0] != "-c" || args[1] != "protocol.ext.allow=never" || args[2] != "clone" {
		t.Errorf("gitCloneArgs = %q, want -c protocol.ext.allow=never before clone", args)
	}
}

func TestGitCloneArgs_EndOptionsBeforeSource(t *testing.T) {
	cases := []struct{ ref string }{{""}, {"v1.2.0"}}
	for _, c := range cases {
		args := gitCloneArgs("https://example.com/p", c.ref, "dest")
		n := len(args)
		if n < 3 || args[n-3] != "--" || args[n-2] != "https://example.com/p" || args[n-1] != "dest" {
			t.Errorf("gitCloneArgs(ref=%q) = %q, want -- before source and dest", c.ref, args)
		}
	}
}

func TestPacksUpdate_SkipNoticeQuotesLockfileValues(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	mustWrite(t, filepath.Join(parent, "elsewhere", "rules", "x.md"), "b")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, root)
	writeLock(t, root, packEntry{Name: "x$(touch pwned)", Source: "../elsewhere"})

	var out bytes.Buffer
	if err := runPacksUpdate(root, "", false, &out); err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out.String(), "--name 'x$(touch pwned)'") {
		t.Errorf("skip notice must quote the lockfile name, got %q", out.String())
	}
}

func TestPacksUpdate_DotNameErrorPointsToPacksAdd(t *testing.T) {
	root := testutil.TempCwd(t)
	writeLock(t, root, packEntry{Name: ".github", Source: "github.com/org/.github"})

	var out bytes.Buffer
	err := runPacksUpdate(root, "", false, &out)
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Errorf("err=%v, want a refusal that names packs add --name", err)
	}
}

func TestPacksAdd_RefusesGitSourcesUpdateRefuses(t *testing.T) {
	root := testutil.TempCwd(t)
	for _, source := range []string{"http://example.com/pack", "git://example.com/pack", "-c"} {
		var out bytes.Buffer
		if err := runPacksAdd(root, source, "pack", &out); err == nil {
			t.Errorf("packs add %q accepted, expected error", source)
		}
	}
}
