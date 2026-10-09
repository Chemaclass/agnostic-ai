package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

var hookReaders = memoryReaders{hook: []string{"codex"}}

// factIndex returns n index lines linking prefix-<i>.md, each with desc,
// and the fact files they link.
func factIndex(prefix string, n int, desc string) (string, map[string]string) {
	files := map[string]string{}
	var index strings.Builder
	for i := range n {
		file := fmt.Sprintf("%s-%d.md", prefix, i)
		files[file] = memoryFact(file, "project", "A fact")
		index.WriteString("- [" + prefix + " " + strconv.Itoa(i) + "](" + file + "): " + desc + "\n")
	}
	return index.String(), files
}

func withIndex(index string, files map[string]string) map[string]string {
	files["MEMORY.md"] = index
	return files
}

// sizedMemoryIndex returns facts and an index whose hook context, as
// `hook memory` prints it, is exactly bytes long.
func sizedMemoryIndex(t *testing.T, bytes int, facts ...string) map[string]string {
	t.Helper()
	files := map[string]string{}
	lines := make([]string, len(facts))
	for i, f := range facts {
		files[f+".md"] = memoryFact(f, "project", "A fact")
		lines[i] = "- [" + f + "](" + f + ".md): a fact\n"
	}
	project := memoryIndex{name: "Project memory", path: ".agnostic-ai/memory/MEMORY.md", text: strings.Join(lines, "")}
	scopes := []memoryIndex{{name: "Personal memory", path: ".agnostic-ai/local/memory/MEMORY.md"}, project}
	pad := bytes - len(memoryContext([]memoryIndex{project}, scopes))
	if pad < 8 {
		t.Fatalf("%d bytes is too small for %d facts", bytes, len(facts))
	}
	// The padding sits after the first fact, so the cut drops the rest.
	files["MEMORY.md"] = lines[0] + "<!--" + strings.Repeat("x", pad-8) + "-->\n" + strings.Join(lines[1:], "")
	return files
}

func TestLintMemory_WarnsWhenTheHookWouldDropFacts(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", sizedMemoryIndex(t, memoryContextLimit+1, "kept", "dropped"))

	findings, err := lintMemory(memoryReaders{hook: []string{"codex", "cursor"}, whole: []string{"claude"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 {
		t.Fatalf("want one finding, got %v", findings)
	}
	f := findings[0]
	if f.Code != "LINT039" || f.Severity != lintWarn || f.Path != filepath.Join(".agnostic-ai", "memory", "MEMORY.md") {
		t.Errorf("got %+v, want a LINT039 warning on the project index", f)
	}
	for _, want := range []string{"6000 bytes", "codex, cursor", "dropped.md"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("the finding should say %q: %s", want, f.Message)
		}
	}
	if strings.Contains(f.Message, "kept.md") || strings.Contains(f.Message, "claude") {
		t.Errorf("the finding should name only the dropped fact and the hook tools: %s", f.Message)
	}
}

func TestLintMemory_QuietWhenTheHookKeepsEveryFact(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", sizedMemoryIndex(t, memoryContextLimit, "kept", "last"))

	if got, err := lintMemory(hookReaders); err != nil || len(got) != 0 {
		t.Fatalf("an index the hook keeps whole should give no findings, got %v (err %v)", got, err)
	}
}

func TestLintMemory_ListsADroppedFactOnceAndSkipsOneAKeptLineLinks(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	index, files := factIndex("fact", 12, strings.Repeat("y", 500))
	index += "- [Again](fact-0.md): kept above\n- [Twice](fact-11.md): again\n"
	writeMemory(t, dir, ".agnostic-ai/memory", withIndex(index, files))

	findings, err := lintMemory(hookReaders)
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 {
		t.Fatalf("want one finding, got %v", findings)
	}
	msg := findings[0].Message
	if strings.Contains(msg, "fact-0.md") || strings.Count(msg, "fact-11.md") != 1 || !strings.Contains(msg, "fact-10.md") {
		t.Errorf("each dropped fact once, and none a kept line links: %s", msg)
	}
}

func TestLintMemory_CapsTheDroppedFactList(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	index, files := factIndex("fact", 100, strings.Repeat("z", 80))
	writeMemory(t, dir, ".agnostic-ai/memory", withIndex(index, files))

	findings, err := lintMemory(hookReaders)
	if err != nil {
		t.Fatal(err)
	}

	names, total := droppedInFinding(t, findings[0].Message)
	if len(names) != 10 || total <= 10 {
		t.Errorf("want the first 10 names and a larger total, got %v of %d: %s", names, total, findings[0].Message)
	}
}

func TestLintMemory_WarnsOnSizeWhenNoTargetUsesTheHook(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	index, files := factIndex("fact", 70, strings.Repeat("w", 80))
	writeMemory(t, dir, ".agnostic-ai/memory", withIndex(index, files))

	findings, err := lintMemory(memoryReaders{whole: []string{"claude", "opencode"}})
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 || findings[0].Code != "LINT039" {
		t.Fatalf("want one LINT039 finding, got %v", findings)
	}
	msg := findings[0].Message
	if !strings.Contains(msg, fmt.Sprintf("%d bytes", len(index))) || !strings.Contains(msg, "claude, opencode") || !strings.Contains(msg, "in full") || strings.Contains(msg, "never") {
		t.Errorf("want the size and the tools that load it in full, with no claim of dropped facts: %s", msg)
	}
}

func TestLintMemory_QuietOnSizeAtTheLimitWithoutTheHook(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{"MEMORY.md": strings.Repeat("x", memoryContextLimit-1) + "\n"})

	if got, err := lintMemory(memoryReaders{whole: []string{"claude"}}); err != nil || len(got) != 0 {
		t.Fatalf("indexes at the limit should give no findings, got %v (err %v)", got, err)
	}
}

func TestLintMemory_NoReadersNoSizeFinding(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	index, files := factIndex("fact", 70, strings.Repeat("w", 80))
	writeMemory(t, dir, ".agnostic-ai/memory", withIndex(index, files))

	if got, err := lintMemory(memoryReaders{}); err != nil || len(got) != 0 {
		t.Fatalf("with the memory built-in off no tool loads the indexes, got %v (err %v)", got, err)
	}
}

func TestProjectMemoryReaders_SplitsHookTargetsFromWholeReaders(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, opencode, cursor]\nbuiltins: [memory]\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}

	got := projectMemoryReaders(cfg, bundle)

	if strings.Join(got.hook, ",") != "codex,cursor" || strings.Join(got.whole, ",") != "claude,opencode" {
		t.Errorf("got hook %v, whole %v", got.hook, got.whole)
	}
	cfg.Builtins = nil
	if got := projectMemoryReaders(cfg, bundle); len(got.hook)+len(got.whole) != 0 {
		t.Errorf("without the memory built-in no tool loads memory, got %+v", got)
	}
}

var droppedList = regexp.MustCompile(`facts: (.*?)(?:, and (\d+) more)?; merge`)

// droppedInFinding returns the fact files a hook finding names and how
// many it says are dropped.
func droppedInFinding(t *testing.T, message string) ([]string, int) {
	t.Helper()
	m := droppedList.FindStringSubmatch(message)
	if m == nil {
		t.Fatalf("no fact list in %q", message)
	}
	names := strings.Split(m[1], ", ")
	more := 0
	if m[2] != "" {
		more, _ = strconv.Atoi(m[2])
	}
	return names, len(names) + more
}

// hookDropped returns, in index order, the facts whose link the hook
// output leaves out.
func hookDropped(output string, indexes ...string) []string {
	var out []string
	for _, index := range indexes {
		for _, m := range regexp.MustCompile(`\]\(([^)]+)\)`).FindAllStringSubmatch(index, -1) {
			if !strings.Contains(output, "]("+m[1]+")") && !slices.Contains(out, m[1]) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

func TestMemoryLint_NamesTheFactsTheHookDrops(t *testing.T) {
	multibyte := strings.Repeat("café ñandú 日本語 ", 6)
	cases := []struct {
		name               string
		personal, project  string
		personalN          int
		projectN           int
		findings           int
		repo, skipPersonal bool
	}{
		{name: "project cut", project: strings.Repeat("a", 90), projectN: 70, findings: 1},
		{name: "multibyte text", project: multibyte, projectN: 60, findings: 1},
		{name: "single oversized line", personal: strings.Repeat("p", 7000), personalN: 1, project: "short", projectN: 3, findings: 1},
		{name: "both indexes over", personal: strings.Repeat("q", 100), personalN: 70, project: "short", projectN: 3, findings: 2},
		{name: "repo-mode personal index", repo: true, personal: strings.Repeat("r", 90), personalN: 40, project: strings.Repeat("s", 90), projectN: 30, findings: 1},
		{name: "repo-mode personal index missing", repo: true, skipPersonal: true, project: strings.Repeat("t", 90), projectN: 70, findings: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			personalDir := filepath.Join(".agnostic-ai", "local", "memory")
			if tc.repo {
				repoMemoryProject(t, false)
				stores, err := memoryStoresAt(".")
				if err != nil {
					t.Fatal(err)
				}
				personalDir = stores[1].dir
			} else {
				memoryHookProject(t, "")
			}
			dir := "."
			var personal string
			if !tc.skipPersonal && tc.personalN > 0 {
				var files map[string]string
				personal, files = factIndex("me", tc.personalN, tc.personal)
				for name, content := range withIndex(personal, files) {
					mustWriteFile(t, filepath.Join(personalDir, name), content)
				}
			}
			project, files := factIndex("team", tc.projectN, tc.project)
			writeMemory(t, dir, ".agnostic-ai/memory", withIndex(project, files))

			want := hookDropped(runHookMemory(t, "--target", "codex"), personal, project)
			out, _ := runRoot(t, "memory", "lint", "--json")

			var got struct {
				Findings []struct{ Code, Message string }
			}
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("invalid JSON: %v\n%s", err, out)
			}
			var names []string
			total := 0
			for _, f := range got.Findings {
				if f.Code != "LINT039" {
					continue
				}
				n, count := droppedInFinding(t, f.Message)
				names, total = append(names, n...), total+count
			}
			if total != len(want) {
				t.Errorf("lint counts %d dropped facts, the hook drops %d: %v", total, len(want), want)
			}
			for _, name := range names {
				if !slices.Contains(want, name) {
					t.Errorf("lint names %s, which the hook keeps", name)
				}
			}
			if count := len(got.Findings); count != tc.findings {
				t.Errorf("want %d findings, got %d: %+v", tc.findings, count, got.Findings)
			}
		})
	}
}
