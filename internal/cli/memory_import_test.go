package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const (
	tabsFact = "---\nname: tabs\ndescription: user prefers tabs\nmetadata:\n  type: user\n---\n\nThe user prefers tabs.\n"
	ciFact   = "---\nname: ci-ubuntu\ndescription: CI runs on Ubuntu\nmetadata:\n  type: project\n---\n\nPR CI runs on Ubuntu.\n"
)

// importProject makes a Git project with the memory built-in and the
// claude target, and points Claude Code's config at a temp folder. It
// returns Claude's own memory store for the project, not yet created.
func importProject(t *testing.T) string {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_CODE_PROJECT_DIR_NAME", "")
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", home)
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nbuiltins: [memory]\n")
	silence(t)
	native, ok, err := adapters.NativeMemoryDir("claude", ".")
	if err != nil || !ok {
		t.Fatalf("NativeMemoryDir: %v %v", ok, err)
	}
	return native
}

func writeStore(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
}

func listing(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s %s %d\n", e.Name(), info.ModTime(), info.Size())
	}
	return b.String()
}

var personalStore = filepath.Join(".agnostic-ai", "local", "memory")

func TestMemoryImport_CopiesClaudesFactsIntoPersonalMemory(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{
		"MEMORY.md":    "- [Tabs please](tabs.md) — the user prefers tabs\n",
		"tabs.md":      tabsFact,
		"ci-ubuntu.md": ciFact,
	})
	before := listing(t, native)

	out, err := runRoot(t, "memory", "import", "--from", "claude")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if got := readText(t, filepath.Join(personalStore, "tabs.md")); got != tabsFact {
		t.Errorf("tabs.md = %q", got)
	}
	index := readText(t, filepath.Join(personalStore, "MEMORY.md"))
	if !strings.Contains(index, "- [Tabs please](tabs.md) — the user prefers tabs\n") {
		t.Errorf("source index line not kept:\n%s", index)
	}
	if !strings.Contains(index, "- [ci-ubuntu](ci-ubuntu.md): CI runs on Ubuntu\n") {
		t.Errorf("unlinked fact got no line from its frontmatter:\n%s", index)
	}
	if !strings.Contains(out, "Copied 2 fact(s)") {
		t.Errorf("output:\n%s", out)
	}
	if after := listing(t, native); after != before {
		t.Errorf("source changed:\nbefore %s\nafter %s", before, after)
	}
}

func TestMemoryImport_KeepsAFactPersonalMemoryAlreadyHas(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "ci-ubuntu.md": ciFact})
	writeStore(t, personalStore, map[string]string{
		"MEMORY.md":    "- [Tabs](tabs.md): mine\n",
		"tabs.md":      "my own tabs fact\n",
		"ci-ubuntu.md": ciFact,
	})

	out, err := runRoot(t, "memory", "import", "--from", "claude")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if got := readText(t, filepath.Join(personalStore, "tabs.md")); got != "my own tabs fact\n" {
		t.Errorf("personal fact replaced: %q", got)
	}
	if !strings.Contains(out, "Kept 1 fact(s)") || !strings.Contains(out, "  tabs.md") || strings.Contains(out, "ci-ubuntu.md") {
		t.Errorf("output:\n%s", out)
	}
	want := "- [Tabs](tabs.md): mine\n- [ci-ubuntu](ci-ubuntu.md): CI runs on Ubuntu\n"
	if got := readText(t, filepath.Join(personalStore, "MEMORY.md")); got != want {
		t.Errorf("index = %q, want the unlinked identical fact added: %q", got, want)
	}
}

func TestMemoryImport_SkipsSymlinksAndSecrets(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{
		"token.md": "deploy with ghp_" + strings.Repeat("a", 36) + "\n",
		"tabs.md":  tabsFact,
	})
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeFile(t, outside, "not a fact\n")
	if err := os.Symlink(outside, filepath.Join(native, "link.md")); err != nil {
		t.Skipf("symlink: %v", err)
	}

	out, err := runRoot(t, "memory", "import", "--from", "claude")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	for _, name := range []string{"token.md", "link.md"} {
		if _, err := os.Lstat(filepath.Join(personalStore, name)); err == nil {
			t.Errorf("%s copied", name)
		}
	}
	if !strings.Contains(out, "token.md: line 1 looks like a secret") || !strings.Contains(out, "link.md: not a regular file") {
		t.Errorf("output:\n%s", out)
	}
}

func TestMemoryImport_DryRunWritesNothing(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	out, err := runRoot(t, "memory", "import", "--from", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if !strings.Contains(out, "Would copy 1 fact(s)") {
		t.Errorf("output:\n%s", out)
	}
	if _, err := os.Stat(personalStore); err == nil {
		t.Error("dry run created the personal store")
	}
}

func TestMemoryImport_CopiesTheCheckoutStoreIntoTheRepoStore(t *testing.T) {
	importProject(t)
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
	writeStore(t, personalStore, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	out, err := runRoot(t, "memory", "import", "--from", "checkout")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	repo := memoryPaths(t)["personal"]
	if got := readText(t, filepath.Join(filepath.FromSlash(repo), "tabs.md")); got != tabsFact {
		t.Errorf("repo store tabs.md = %q", got)
	}
}

func TestMemoryImport_RefusesToCopyAStoreIntoItself(t *testing.T) {
	importProject(t)
	writeStore(t, personalStore, map[string]string{"tabs.md": tabsFact})

	if out, err := runRoot(t, "memory", "import", "--from", "checkout"); err == nil || !strings.Contains(err.Error(), "personal memory store itself") {
		t.Errorf("err = %v\n%s", err, out)
	}
}

func TestMemoryImport_NeedsExactlyOneSource(t *testing.T) {
	importProject(t)

	for _, args := range [][]string{{"memory", "import"}, {"memory", "import", "--from", "claude", "--path", "x"}} {
		if _, err := runRoot(t, args...); err == nil || !strings.Contains(err.Error(), "one of --from or --path") {
			t.Errorf("%v: err = %v", args, err)
		}
	}
}

func TestMemoryImport_SaysWhereItLookedForClaudesStore(t *testing.T) {
	native := importProject(t)

	_, err := runRoot(t, "memory", "import", "--from", "claude")

	if err == nil || !strings.Contains(err.Error(), filepath.ToSlash(native)) || !strings.Contains(err.Error(), "--path") {
		t.Errorf("err = %v", err)
	}
}

func TestMemoryImport_CopiesFromAnyFolderWithPath(t *testing.T) {
	importProject(t)
	other := filepath.Join(t.TempDir(), "gemini-memory")
	writeStore(t, other, map[string]string{"MEMORY.md": "- [CI](ci-ubuntu.md): CI\n", "ci-ubuntu.md": ciFact})

	if out, err := runRoot(t, "memory", "import", "--path", other); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if got := readText(t, filepath.Join(personalStore, "MEMORY.md")); got != "- [CI](ci-ubuntu.md): CI\n" {
		t.Errorf("index:\n%s", got)
	}
}

func TestSync_SeedsAnEmptyPersonalStoreFromClaudesOwnStore(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})
	before := listing(t, native)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	if got := readText(t, filepath.Join(personalStore, "tabs.md")); got != tabsFact {
		t.Errorf("tabs.md = %q", got)
	}
	if after := listing(t, native); after != before {
		t.Errorf("source changed")
	}
}

func TestSync_DryRunSeedsNothing(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	if err := runSyncOnce(".", nil, true, false, "", 1); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(personalStore); err == nil {
		t.Error("dry run seeded the personal store")
	}
}

func TestSync_LeavesAPersonalStoreWithFactsAlone(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})
	writeStore(t, personalStore, map[string]string{"MEMORY.md": "- [CI](ci-ubuntu.md): CI\n", "ci-ubuntu.md": ciFact})

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(personalStore, "tabs.md")); err == nil {
		t.Error("sync merged into a personal store that holds facts")
	}
}

func TestLintMemory_ReportsClaudesFactsThatNoLongerLoad(t *testing.T) {
	native := importProject(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "ci-ubuntu.md": ciFact})
	writeStore(t, personalStore, map[string]string{"ci-ubuntu.md": ciFact})

	out, err := runRoot(t, "memory", "lint")
	if err != nil && !strings.Contains(out, "LINT043") {
		t.Fatalf("memory lint: %v\n%s", err, out)
	}

	if !strings.Contains(out, "LINT043") || !strings.Contains(out, "1 fact(s) here no longer load") || !strings.Contains(out, "memory import --from claude") {
		t.Errorf("output:\n%s", out)
	}
}

func TestLintMemory_ReportsTheCheckoutStoreAfterSwitchingToRepoMode(t *testing.T) {
	importProject(t)
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
	writeStore(t, personalStore, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	out, _ := runRoot(t, "memory", "lint")

	if !strings.Contains(out, "LINT043") || !strings.Contains(out, "memory import --from checkout") {
		t.Errorf("output:\n%s", out)
	}
}

func TestLintMemory_StaysQuietWithoutAnotherStore(t *testing.T) {
	importProject(t)
	writeStore(t, personalStore, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	out, _ := runRoot(t, "memory", "lint")

	if strings.Contains(out, "LINT043") {
		t.Errorf("output:\n%s", out)
	}
}

func TestMemoryImport_KeepsTheCheckoutStoreOutOfGit(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if err := exec.Command("git", "check-ignore", "-q", filepath.Join(personalStore, "tabs.md")).Run(); err != nil {
		t.Errorf("personal store is not ignored after import: %v", err)
	}
}

func gitCommitAll(t *testing.T) {
	t.Helper()
	for _, args := range [][]string{{"add", "-A", "-f", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "x"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestMemoryImport_RefusesAStoreGitTracks(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})
	writeStore(t, personalStore, map[string]string{"MEMORY.md": "- [CI](ci-ubuntu.md): CI\n", "ci-ubuntu.md": ciFact})
	gitCommitAll(t)

	_, err := runRoot(t, "memory", "import", "--from", "claude")

	if err == nil || !strings.Contains(err.Error(), "Git tracks") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(personalStore, "tabs.md")); err == nil {
		t.Error("import wrote into a tracked store")
	}
}

func TestMemoryImport_FinishesAnImportThatStoppedPartWay(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})
	writeStore(t, personalStore, map[string]string{"tabs.md": tabsFact})

	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if got := readText(t, filepath.Join(personalStore, "MEMORY.md")); got != "- [Tabs](tabs.md): tabs\n" {
		t.Errorf("index:\n%s", got)
	}
}

func TestMemoryImport_KeepsNotesFromTheSourceIndex(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{
		"MEMORY.md": "# Memory\n\n- [Tabs](tabs.md): tabs\nAlways run the tests before a push.\n",
		"tabs.md":   tabsFact,
	})

	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	notes := readText(t, filepath.Join(personalStore, "claude-index-notes.md"))
	if !strings.Contains(notes, "# Memory\nAlways run the tests before a push.\n") || !strings.Contains(notes, "type: reference") {
		t.Errorf("notes fact:\n%s", notes)
	}
	if index := readText(t, filepath.Join(personalStore, "MEMORY.md")); !strings.Contains(index, "(claude-index-notes.md)") {
		t.Errorf("index:\n%s", index)
	}
}

func TestMemoryImport_SkipsAnIndexWithOnlyHeadings(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "# Memory\n- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(personalStore, "claude-index-notes.md")); err == nil {
		t.Error("headings alone became a notes fact")
	}
}

func TestLintMemory_CountsNotesInClaudesIndex(t *testing.T) {
	native := importProject(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeStore(t, native, map[string]string{"MEMORY.md": "Prefer small pull requests.\n"})
	writeStore(t, personalStore, map[string]string{"ci-ubuntu.md": ciFact})

	out, _ := runRoot(t, "memory", "lint")

	if !strings.Contains(out, "LINT043") || !strings.Contains(out, "1 fact(s) here no longer load") {
		t.Errorf("output:\n%s", out)
	}
}

func TestWriteNewFile_NeverReplacesAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tabs.md")
	writeFile(t, path, "mine\n")

	created, err := writeNewFile(path, []byte("theirs\n"))

	if err != nil || created {
		t.Fatalf("created = %v, err = %v", created, err)
	}
	if got := readText(t, path); got != "mine\n" {
		t.Errorf("file = %q", got)
	}
	if entries, _ := os.ReadDir(filepath.Dir(path)); len(entries) != 1 {
		t.Errorf("temporary file left behind: %v", entries)
	}
}

func TestLockMemoryStore_WaitsForTheImportHoldingIt(t *testing.T) {
	store := t.TempDir()
	unlock, err := lockMemoryStore(store)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(150 * time.Millisecond)
		close(released)
		unlock()
	}()

	second, err := lockMemoryStore(store)

	if err != nil {
		t.Fatal(err)
	}
	defer second()
	select {
	case <-released:
	default:
		t.Error("second lock taken while the first was held")
	}
}

func TestMemoryImport_BuildsAnIndexLineWhenTheSourceLineLooksLikeASecret(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{
		"MEMORY.md": "- [Deploy](tabs.md): deploy with ghp_" + strings.Repeat("a", 36) + "\n",
		"tabs.md":   tabsFact,
	})

	out, err := runRoot(t, "memory", "import", "--from", "claude")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	index := readText(t, filepath.Join(personalStore, "MEMORY.md"))
	if strings.Contains(index, "ghp_") || !strings.Contains(index, "- [tabs](tabs.md): user prefers tabs\n") {
		t.Errorf("index:\n%s", index)
	}
	if !strings.Contains(out, "Built a new index line for 1 fact(s)") {
		t.Errorf("output:\n%s", out)
	}
}

func TestLintMemory_ForgetsAFactTheUserDeletedAfterImport(t *testing.T) {
	native := importProject(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "ci-ubuntu.md": ciFact})
	writeStore(t, personalStore, map[string]string{"MEMORY.md": ""})
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(personalStore, "tabs.md")); err != nil {
		t.Fatal(err)
	}

	out, _ := runRoot(t, "memory", "lint")

	if strings.Contains(out, "LINT043") {
		t.Errorf("a fact deleted after import still counts:\n%s", out)
	}
}

func TestSync_CheckSeedsNothing(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	_, _ = runRoot(t, "sync", "--check")

	if _, err := os.Stat(personalStore); err == nil {
		t.Error("sync --check seeded the personal store")
	}
}

func TestSync_NamesTheSeedCountAndSource(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "ci-ubuntu.md": ciFact})
	var notes strings.Builder
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.SetWarner(os.Stderr) })

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	if !strings.Contains(notes.String(), "claude: copied 2 fact(s) from "+filepath.ToSlash(native)) {
		t.Errorf("notes:\n%s", notes.String())
	}
}

// repoStoreWithFacts writes facts into the repository store of the
// project in the working directory, then leaves repo mode.
func repoStoreWithFacts(t *testing.T, files map[string]string) string {
	t.Helper()
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
	repo := filepath.FromSlash(memoryPaths(t)["personal"])
	writeStore(t, repo, files)
	if err := os.Remove("agnostic-ai.local.yaml"); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestMemoryImport_CopiesTheRepoStoreIntoTheCheckout(t *testing.T) {
	importProject(t)
	repoStoreWithFacts(t, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	if out, err := runRoot(t, "memory", "import", "--from", "repo"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if got := readText(t, filepath.Join(personalStore, "tabs.md")); got != tabsFact {
		t.Errorf("tabs.md = %q", got)
	}
}

func TestLintMemory_ReportsTheRepoStoreInCheckoutMode(t *testing.T) {
	importProject(t)
	repoStoreWithFacts(t, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})

	out, _ := runRoot(t, "memory", "lint")

	if !strings.Contains(out, "LINT043") || !strings.Contains(out, "memory import --from repo") {
		t.Errorf("output:\n%s", out)
	}
}

func TestMemoryImport_NeverWritesThroughALinkedIndex(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "- [Tabs](tabs.md): tabs\n", "tabs.md": tabsFact})
	writeStore(t, personalStore, map[string]string{})
	if err := os.Symlink(filepath.Join(native, "MEMORY.md"), filepath.Join(personalStore, "MEMORY.md")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	before := readText(t, filepath.Join(native, "MEMORY.md"))

	_, err := runRoot(t, "memory", "import", "--from", "claude")

	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("err = %v", err)
	}
	if after := readText(t, filepath.Join(native, "MEMORY.md")); after != before {
		t.Errorf("source index changed:\n%s", after)
	}
}

func TestMemoryImport_IgnoresTheWholeStoreWhenOnlyMarkdownIsIgnored(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})
	writeFile(t, ".gitignore", "*.md\n")

	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	for _, name := range []string{".imported", ".lock"} {
		if err := exec.Command("git", "check-ignore", "-q", filepath.Join(personalStore, name)).Run(); err != nil {
			t.Errorf("%s is not ignored: %v", name, err)
		}
	}
}

func TestLintMemory_StillCountsAFactImportCouldNotRead(t *testing.T) {
	native := importProject(t)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "token.md": "deploy with ghp_" + strings.Repeat("a", 36) + "\n"})
	writeStore(t, personalStore, map[string]string{"MEMORY.md": ""})
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	out, _ := runRoot(t, "memory", "lint")

	if !strings.Contains(out, "LINT043") || !strings.Contains(out, "1 fact(s) here no longer load") {
		t.Errorf("the skipped fact should still count:\n%s", out)
	}
}

func TestMemoryImport_CopiesOnlyTheFactsYouPick(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact, "ci-ubuntu.md": ciFact})

	out, err := runRoot(t, "memory", "import", "--from", "claude", "--only", "tabs.md")
	if err != nil {
		t.Fatalf("memory import: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(personalStore, "ci-ubuntu.md")); err == nil {
		t.Error("a fact outside --only was copied")
	}
	if got := readText(t, filepath.Join(personalStore, "tabs.md")); got != tabsFact {
		t.Errorf("tabs.md = %q", got)
	}
}

func TestMemoryImport_RejectsAnOnlyNameTheSourceLacks(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	_, err := runRoot(t, "memory", "import", "--from", "claude", "--only", "nope.md")

	if err == nil || !strings.Contains(err.Error(), "--only nope.md") {
		t.Errorf("err = %v", err)
	}
}

func TestMemoryImport_SaysEveryToolReadsWhatItCopies(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})

	dry, err := runRoot(t, "memory", "import", "--from", "claude", "--dry-run")
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, dry)
	}
	real, err := runRoot(t, "memory", "import", "--from", "claude")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, real)
	}

	if !strings.Contains(dry, "tabs.md: user prefers tabs") || !strings.Contains(dry, "Every tool reads personal memory. Check each fact above") {
		t.Errorf("dry run:\n%s", dry)
	}
	if !strings.Contains(real, "these facts now guide all of them") {
		t.Errorf("import:\n%s", real)
	}
}

func TestMemoryImport_KeepsIndexNotesWhenASourceFactUsesTheirName(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "Prefer small pull requests.\n", "claude-index-notes.md": tabsFact})
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readText(t, filepath.Join(personalStore, "claude-index-notes.md")); got != tabsFact {
		t.Errorf("source fact = %q", got)
	}
	notes := readText(t, filepath.Join(personalStore, "claude-index-notes-2.md"))
	if !strings.Contains(notes, "Prefer small pull requests.") {
		t.Errorf("notes = %q", notes)
	}
	before := listing(t, personalStore)
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("retry: %v\n%s", err, out)
	}
	if after := listing(t, personalStore); after != before {
		t.Errorf("retry changed store:\n%s\n%s", before, after)
	}
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(personalStore, "claude-index-notes-2.md")); err != nil {
		t.Fatal(err)
	}
	out, _ := runRoot(t, "memory", "lint")
	if strings.Contains(out, "LINT043") {
		t.Errorf("handled notes still reported:\n%s", out)
	}
}

func TestMemoryImport_GuardsTheDestinationOfASymlinkedStore(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})
	writeStore(t, "public-notes", map[string]string{"tracked.md": ciFact})
	if out, err := exec.Command("git", "add", "public-notes/tracked.md").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v %s", err, out)
	}
	if err := os.MkdirAll(filepath.Dir(personalStore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "public-notes"), personalStore); err != nil {
		t.Skipf("symlink: %v", err)
	}
	writeFile(t, ".gitignore", ".agnostic-ai/local/\n")
	_, err := runRoot(t, "memory", "import", "--from", "claude")
	if err == nil || !strings.Contains(err.Error(), "Git tracks") {
		t.Errorf("import error = %v", err)
	}
	if _, err := os.Stat(filepath.Join("public-notes", "tabs.md")); !os.IsNotExist(err) {
		t.Errorf("private fact reached tracked destination: %v", err)
	}
}

func TestMemoryImport_ExcludesTheDestinationOfASymlinkedParent(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"tabs.md": tabsFact})
	if err := os.MkdirAll("public-notes", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "public-notes"), filepath.Join(".agnostic-ai", "local")); err != nil {
		t.Skipf("symlink: %v", err)
	}
	writeFile(t, ".gitignore", ".agnostic-ai/local/\n")
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if err := exec.Command("git", "check-ignore", "-q", "public-notes/memory/tabs.md").Run(); err != nil {
		t.Errorf("effective destination is committable: %v", err)
	}
}

func TestMemoryImport_KeepsIndexNotesWhenTheDestinationUsesTheirName(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"MEMORY.md": "Prefer small pull requests.\n"})
	writeStore(t, personalStore, map[string]string{"claude-index-notes.md": tabsFact})
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readText(t, filepath.Join(personalStore, "claude-index-notes.md")); got != tabsFact {
		t.Errorf("existing fact = %q", got)
	}
	if got := readText(t, filepath.Join(personalStore, "claude-index-notes-2.md")); !strings.Contains(got, "Prefer small pull requests.") {
		t.Errorf("notes = %q", got)
	}
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil || !strings.Contains(out, "Nothing to copy") {
		t.Errorf("retry: %v\n%s", err, out)
	}
}

func TestMemoryImport_KeepsNewIndexNotesAfterTheCollidingSourceFactIsRemoved(t *testing.T) {
	native := importProject(t)
	writeStore(t, native, map[string]string{"claude-index-notes.md": tabsFact})
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("initial import: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(native, "claude-index-notes.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(native, "MEMORY.md"), "Prefer small pull requests.\n")
	if out, err := runRoot(t, "memory", "import", "--from", "claude"); err != nil {
		t.Fatalf("notes import: %v\n%s", err, out)
	}
	if got := readText(t, filepath.Join(personalStore, "claude-index-notes.md")); got != tabsFact {
		t.Errorf("original fact = %q", got)
	}
	if got := readText(t, filepath.Join(personalStore, "claude-index-notes-2.md")); !strings.Contains(got, "Prefer small pull requests.") {
		t.Errorf("new prose lost: %q", got)
	}
}
