package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var stagedCheckCases = []string{"clean", "yaml-comment", "semantic-config", "spec-change", "deleted-output", "deleted-spec"}

func TestStagedCheckFixture_DoesNotStartAutomaticMaintenance(t *testing.T) {
	trace := filepath.Join(t.TempDir(), "git-trace.jsonl")
	t.Setenv("GIT_TRACE2_EVENT", trace)
	writeStagedCheckFixture(t, t.TempDir(), 4, 120)
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatalf("read Git trace: %v", err)
	}
	sawCommit := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Event string   `json:"event"`
			Name  string   `json:"name"`
			Argv  []string `json:"argv"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse Git trace: %v", err)
		}
		if event.Event == "cmd_name" && event.Name == "commit" {
			sawCommit = true
		}
		if event.Event == "child_start" && strings.Contains(strings.Join(event.Argv, " "), "maintenance run --auto") {
			t.Errorf("fixture started automatic maintenance: %v", event.Argv)
		}
	}
	if !sawCommit {
		t.Error("Git trace did not record the fixture commit")
	}
}

func TestStagedCheckBenchmarkControls(t *testing.T) {
	for _, scenario := range stagedCheckCases {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			writeStagedCheckFixture(t, root, 4, 120)
			applyStagedCheckCase(t, root, scenario)
			before := stagedCheckGit(t, root, "status", "--porcelain=v1")
			index := stagedCheckGit(t, root, "write-tree")
			err := runStagedCheckCommand(t, root, "sync", "--check", "--against", "index")
			checkStagedCheckResult(t, scenario, err)
			if got := stagedCheckGit(t, root, "write-tree"); got != index {
				t.Error("staged check changed the index")
			}
			if got := stagedCheckGit(t, root, "status", "--porcelain=v1"); got != before {
				t.Error("staged check changed the working tree status")
			}
		})
	}
}

func BenchmarkSyncCheckAgainstIndex(b *testing.B) {
	for _, specs := range []int{100, 500} {
		for _, scenario := range stagedCheckCases {
			b.Run(fmt.Sprintf("specs=%d/files=23000/%s", specs, scenario), func(b *testing.B) {
				root := b.TempDir()
				writeStagedCheckFixture(b, root, specs, 23000)
				applyStagedCheckCase(b, root, scenario)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					err := runStagedCheckCommand(b, root, "sync", "--check", "--against", "index")
					checkStagedCheckResult(b, scenario, err)
				}
			})
		}
	}
}

func checkStagedCheckResult(tb testing.TB, scenario string, err error) {
	tb.Helper()
	wantFailure := scenario != "clean" && scenario != "yaml-comment"
	if (err != nil) != wantFailure {
		tb.Fatalf("%s: expected failure=%t, got %v", scenario, wantFailure, err)
	}
	if err != nil && err.Error() != againstHint(againstIndex) {
		tb.Fatalf("%s: expected staged drift, got an unrelated error: %v", scenario, err)
	}
}

func TestWriteStagedCheckBenchFixture(t *testing.T) {
	root := os.Getenv("AGNOSTIC_AI_STAGED_BENCH_FIXTURE")
	if root == "" {
		t.Skip("AGNOSTIC_AI_STAGED_BENCH_FIXTURE not set")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("AGNOSTIC_AI_STAGED_BENCH_FIXTURE must be an absolute empty directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("fixture directory must exist and be empty: %s: %v", root, err)
	}
	specs := 100
	if value := os.Getenv("AGNOSTIC_AI_STAGED_BENCH_SPECS"); value != "" {
		specs, err = strconv.Atoi(value)
		if err != nil || (specs != 100 && specs != 500) {
			t.Fatal("AGNOSTIC_AI_STAGED_BENCH_SPECS must be 100 or 500")
		}
	}
	writeStagedCheckFixture(t, root, specs, 23000)
	scenario := os.Getenv("AGNOSTIC_AI_STAGED_BENCH_CASE")
	if scenario == "" {
		scenario = "clean"
	}
	applyStagedCheckCase(t, root, scenario)
	t.Logf("exported %d specs, 23000 tracked baseline files, case %s, HEAD %s", specs, scenario, stagedCheckGit(t, root, "rev-parse", "HEAD"))
}

func writeStagedCheckFixture(tb testing.TB, root string, specs, files int) {
	tb.Helper()
	for _, key := range []string{"HOME", "USERPROFILE", "CODEX_HOME", "AGNOSTIC_AI_HOME"} {
		tb.Setenv(key, tb.TempDir())
	}
	tb.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	gitConfig := filepath.Join(tb.TempDir(), "gitconfig")
	writeBenchFile(tb, gitConfig, "")
	tb.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	tb.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	stagedCheckGit(tb, root, "init", "-q")
	stagedCheckGit(tb, root, "config", "core.autocrlf", "false")
	stagedCheckGit(tb, root, "config", "core.symlinks", "true")
	writeBenchFile(tb, filepath.Join(root, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\ngitignore:\n  enabled: true\n  commit: [instructions, skills]\n")
	writeBenchFile(tb, filepath.Join(root, ".agnostic-ai", "AGNOSTIC_AI.md"), "# Benchmark project\n\nRead the nearest scoped instructions before editing.\n")
	for i := 0; i < specs/2; i++ {
		body := benchRule(i)
		if i > 0 && i%3 == 0 {
			body = strings.Replace(body, "---\n", fmt.Sprintf("---\nscope: packages/unit-%04d\n", i%20), 1)
		}
		writeBenchFile(tb, filepath.Join(root, ".agnostic-ai", "rules", fmt.Sprintf("rule-%04d.md", i)), body)
	}
	for i := 0; i < specs-specs/2; i++ {
		dir := filepath.Join(root, ".agnostic-ai", "skills", fmt.Sprintf("skill-%04d", i))
		writeBenchFile(tb, filepath.Join(dir, "SKILL.md"), benchSkill(i))
		writeBenchFile(tb, filepath.Join(dir, "assets", "example.md"), fmt.Sprintf("# Example %d\n\nDeterministic asset body.\n", i))
	}
	for i := 0; i < 20; i++ {
		writeBenchFile(tb, filepath.Join(root, "packages", fmt.Sprintf("unit-%04d", i), "src", "index.txt"), "Source fixture.\n")
	}
	if err := runStagedCheckCommand(tb, root, "sync"); err != nil {
		tb.Fatalf("sync benchmark fixture: %v", err)
	}
	stagedCheckGit(tb, root, "add", "-A")
	stagedCheckGit(tb, root, "ls-files", "--error-unmatch", ".claude/rules/rule-0000.md")
	checkStagedCheckSkillLink(tb, root)
	tracked := strings.Count(stagedCheckGit(tb, root, "ls-files", "-z"), "\x00")
	if tracked > files {
		tb.Fatalf("fixture has %d tracked files, exceeding requested %d", tracked, files)
	}
	for i := 0; i < files-tracked; i++ {
		writeBenchFile(tb, filepath.Join(root, "packages", fmt.Sprintf("unit-%04d", i/20), "src", fmt.Sprintf("file-%05d.txt", i)), fmt.Sprintf("Source %d.\n", i))
	}
	stagedCheckGit(tb, root, "add", "-A")
	if got := strings.Count(stagedCheckGit(tb, root, "ls-files", "-z"), "\x00"); got != files {
		tb.Fatalf("fixture tracked files: got %d, want %d", got, files)
	}
	stagedCheckGit(tb, root, "-c", "user.name=benchmark", "-c", "user.email=benchmark@localhost", "-c", "commit.gpgsign=false", "-c", "core.hooksPath="+os.DevNull, "commit", "-q", "--no-verify", "-m", "benchmark baseline")
}

func checkStagedCheckSkillLink(tb testing.TB, root string) {
	tb.Helper()
	policy := strings.TrimSpace(stagedCheckGit(tb, root, "config", "--local", "--get", "core.symlinks"))
	if policy != "true" {
		tb.Fatalf("fixture Git core.symlinks = %q, want true", policy)
	}
	link := ".claude/skills/skill-0000"
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(link)))
	if err != nil {
		tb.Fatalf("shared skill fixture %s: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		for _, rel := range []string{"SKILL.md", "assets/example.md"} {
			if _, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(link), filepath.FromSlash(rel))); err != nil {
				tb.Fatalf("shared skill copy %s/%s: %v", link, rel, err)
			}
		}
		tb.Logf("fixture Git core.symlinks=%s; %s uses the real-copy fallback", policy, link)
		return
	}
	target, err := os.Readlink(filepath.Join(root, filepath.FromSlash(link)))
	if err != nil {
		tb.Fatalf("read shared skill link %s: %v", link, err)
	}
	entry := strings.TrimSpace(stagedCheckGit(tb, root, "ls-files", "--stage", "--", link))
	if !strings.HasPrefix(entry, "120000 ") || !strings.HasSuffix(entry, "\t"+link) {
		tb.Fatalf("shared skill Git entry must be one symlink: %q", entry)
	}
	canonical := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(filepath.FromSlash(link)), target)))
	if canonical != ".agents/skills/skill-0000" {
		tb.Fatalf("shared skill link %s points to %q, want .agents/skills/skill-0000", link, target)
	}
	probe := tb.TempDir()
	stagedCheckGit(tb, root, "checkout-index", "--force", "--prefix="+probe+string(filepath.Separator), "--",
		canonical+"/SKILL.md", canonical+"/assets/example.md", link)
	got, err := os.Readlink(filepath.Join(probe, filepath.FromSlash(link)))
	if err != nil || filepath.Clean(got) != filepath.Clean(target) {
		tb.Fatalf("Git checkout link %s = %q, %v; want %q", link, got, err, target)
	}
	for _, rel := range []string{"SKILL.md", "assets/example.md"} {
		if _, err := os.ReadFile(filepath.Join(probe, filepath.FromSlash(link), filepath.FromSlash(rel))); err != nil {
			tb.Fatalf("Git checkout shared skill %s/%s: %v", link, rel, err)
		}
	}
	tb.Logf("fixture Git core.symlinks=%s; %s mode=120000 target=%q; Git checkout restored the link and its files", policy, link, target)
}

func applyStagedCheckCase(tb testing.TB, root, scenario string) {
	tb.Helper()
	switch scenario {
	case "clean":
	case "yaml-comment", "semantic-config":
		path := filepath.Join(root, "agnostic-ai.yaml")
		body, err := os.ReadFile(path)
		if err != nil {
			tb.Fatal(err)
		}
		addition := "# Staged comment changes no generated content.\n"
		if scenario == "semantic-config" {
			addition = "outputs:\n  claude:\n    rules-dir: .claude/project-rules\n"
		}
		writeBenchFile(tb, path, string(body)+addition)
		stagedCheckGit(tb, root, "add", "agnostic-ai.yaml")
	case "spec-change":
		path := filepath.Join(root, ".agnostic-ai", "rules", "rule-0000.md")
		writeBenchFile(tb, path, benchRule(0)+"\nStaged instruction changes.\n")
		stagedCheckGit(tb, root, "add", ".agnostic-ai/rules/rule-0000.md")
		if err := runStagedCheckCommand(tb, root, "sync"); err != nil {
			tb.Fatal(err)
		}
	case "deleted-output":
		stagedCheckGit(tb, root, "rm", "-q", "--cached", ".claude/rules/rule-0000.md")
	case "deleted-spec":
		stagedCheckGit(tb, root, "rm", "-q", ".agnostic-ai/rules/rule-0000.md")
	default:
		tb.Fatalf("unknown staged benchmark case %q", scenario)
	}
}

func runStagedCheckCommand(tb testing.TB, root string, args ...string) error {
	tb.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		tb.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		tb.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			tb.Fatal(err)
		}
	}()
	defer quiet()()
	cmd := NewRootCmd("benchmark")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func stagedCheckGit(tb testing.TB, root string, args ...string) string {
	tb.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		tb.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, data)
	}
	return string(data)
}
