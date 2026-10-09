package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func memoryFact(name, typ, description string) string {
	return "---\nname: " + name + "\ndescription: " + description + "\nmetadata:\n  type: " + typ + "\n---\n\nThe fact.\n"
}

func writeMemory(t *testing.T, dir, store string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(store), name), content)
	}
}

func TestLintMemory_Findings(t *testing.T) {
	longIndex := "- [Kept](kept.md): a fact\n" + strings.Repeat("- [Kept](kept.md): again\n", 100)
	index := filepath.Join(".agnostic-ai", "memory", "MEMORY.md")
	cases := []struct {
		name     string
		files    map[string]string
		code     string
		severity lintSeverity
		path     string
		message  string
	}{
		{
			name:     "index over the line cap",
			files:    map[string]string{"MEMORY.md": longIndex, "kept.md": memoryFact("kept", "project", "A fact")},
			code:     "LINT039",
			severity: lintWarn,
			path:     index,
			message:  "101 lines",
		},
		{
			name:     "dead index link",
			files:    map[string]string{"MEMORY.md": "- [Gone](gone.md): removed fact\n"},
			code:     "LINT040",
			severity: lintError,
			path:     index,
			message:  "line 1 links gone.md",
		},
		{
			name: "orphan topic file",
			files: map[string]string{
				"MEMORY.md": "- [Kept](kept.md): a fact\n",
				"kept.md":   memoryFact("kept", "project", "A fact"),
				"stray.md":  memoryFact("stray", "project", "Not indexed"),
			},
			code:     "LINT041",
			severity: lintWarn,
			path:     filepath.Join(".agnostic-ai", "memory", "stray.md"),
			message:  "no line in MEMORY.md",
		},
		{
			name: "secret in a topic file",
			files: map[string]string{
				"MEMORY.md": "- [Token](token.md): the CI token\n",
				"token.md":  memoryFact("token", "reference", "CI token") + "Use ghp_AAAAbbbbCCCC1111 to push.\n",
			},
			code:     "LINT042",
			severity: lintError,
			path:     filepath.Join(".agnostic-ai", "memory", "token.md"),
			message:  "line 9",
		},
		{
			name: "bearer token in the index",
			files: map[string]string{
				"MEMORY.md": "- [API](api.md): call it with `Authorization: Bearer abcd1234efgh5678ijkl`\n",
				"api.md":    memoryFact("api", "reference", "The API"),
			},
			code:     "LINT042",
			severity: lintError,
			path:     index,
			message:  "line 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			writeMemory(t, dir, ".agnostic-ai/memory", tc.files)

			findings, err := lintMemory()
			if err != nil {
				t.Fatal(err)
			}

			if len(findings) != 1 {
				t.Fatalf("want one finding, got %v", findings)
			}
			f := findings[0]
			if f.Code != tc.code || f.Severity != tc.severity || f.Path != tc.path || !strings.Contains(f.Message, tc.message) {
				t.Errorf("got %+v, want %s %s at %s with %q", f, tc.code, tc.severity, tc.path, tc.message)
			}
			if strings.Contains(f.Message, "ghp_") {
				t.Errorf("a finding must never print the secret: %s", f.Message)
			}
		})
	}
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
	index := strings.Join(lines, "")
	scope := []memoryIndex{{name: "Project memory", path: ".agnostic-ai/memory/MEMORY.md", text: index}}
	pad := bytes - len(memoryContext(scope, scope))
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

	findings, err := lintMemory()
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
	if !strings.Contains(f.Message, "6000 bytes") || !strings.Contains(f.Message, "dropped.md") || strings.Contains(f.Message, "kept.md") {
		t.Errorf("the finding should name the limit and only the dropped fact: %s", f.Message)
	}
}

func TestLintMemory_QuietWhenTheHookKeepsEveryFact(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", sizedMemoryIndex(t, memoryContextLimit, "kept", "last"))

	if got, err := lintMemory(); err != nil || len(got) != 0 {
		t.Fatalf("an index the hook keeps whole should give no findings, got %v (err %v)", got, err)
	}
}

func TestLintMemory_PersonalIndexCountsTowardTheHookLimit(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/local/memory", map[string]string{
		"MEMORY.md": "- [Tabs](tabs.md): " + strings.Repeat("prefers tabs ", 300) + "\n",
		"tabs.md":   memoryFact("tabs", "user", "Prefers tabs"),
	})
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": "- [Late](late.md): " + strings.Repeat("a late fact ", 200) + "\n",
		"late.md":   memoryFact("late", "project", "A late fact"),
	})

	findings, err := lintMemory()
	if err != nil {
		t.Fatal(err)
	}

	if len(findings) != 1 || findings[0].Path != filepath.Join(".agnostic-ai", "memory", "MEMORY.md") || !strings.Contains(findings[0].Message, "late.md") {
		t.Fatalf("the hook loads personal memory first, so the project fact drops: %v", findings)
	}
}

func TestLintMemory_CleanAndMissingStoresHaveNoFindings(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if got, err := lintMemory(); err != nil || len(got) != 0 {
		t.Fatalf("no store should give no findings, got %v", got)
	}
	writeMemory(t, dir, ".agnostic-ai/local/memory", map[string]string{
		"MEMORY.md": "- [Tabs](tabs.md): prefers tabs\n",
		"tabs.md": memoryFact("tabs", "user", "Prefers tabs") +
			"The `##KEY=value##` writer and `['config-key'=>'config-value']` are prose, and so is bearer authentication.\n",
	})
	if got, err := lintMemory(); err != nil || len(got) != 0 {
		t.Fatalf("a clean store should give no findings, got %v", got)
	}
}

func TestLintAndDoctor_ReportMemoryFindings(t *testing.T) {
	setupDoctorLintProject(t, map[string]string{
		"memory/MEMORY.md": "- [Gone](gone.md): removed fact\n",
	})
	want := "LINT040 [error] " + filepath.Join(".agnostic-ai", "memory", "MEMORY.md")

	lintOut, lintErr := runRoot(t, "lint")
	doctorOut, doctorErr := runDoctor(t)

	if lintErr == nil || !strings.Contains(lintOut, want) {
		t.Errorf("lint should fail on a dead memory link (err %v):\n%s", lintErr, lintOut)
	}
	if doctorErr == nil || !strings.Contains(doctorOut, want) {
		t.Errorf("doctor should fail on a dead memory link (err %v):\n%s", doctorErr, doctorOut)
	}
}

func TestMemoryLint_ExitsOnErrorsOnly(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": "",
		"stray.md":  memoryFact("stray", "project", "Not indexed"),
	})

	out, err := runRoot(t, "memory", "lint")
	if err != nil || !strings.Contains(out, "LINT041 [warn]") {
		t.Fatalf("a warning must not fail memory lint (err %v):\n%s", err, out)
	}
	if _, err := runRoot(t, "memory", "lint", "--strict"); err == nil {
		t.Errorf("--strict should fail on a warning")
	}

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md"), "- [Gone](gone.md): removed\n")
	out, err = runRoot(t, "memory", "lint", "--json")
	if err == nil {
		t.Fatalf("an error must fail memory lint:\n%s", out)
	}
	var got struct {
		Version, Command string
		Findings         []struct{ Code, Severity, Path, Message string }
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.Version != "1" || got.Command != "memory lint" || len(got.Findings) != 2 || got.Findings[0].Severity != "error" {
		t.Errorf("unexpected JSON: %+v", got)
	}
}

func TestMemoryLint_CleanStore(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": "- [Kept](kept.md): a fact\n",
		"kept.md":   memoryFact("kept", "project", "A fact"),
	})

	out, err := runRoot(t, "memory", "lint")

	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("a clean store should pass (err %v):\n%s", err, out)
	}
}

func TestMemoryLint_SaysWhenNoStoreExists(t *testing.T) {
	testutil.Chdir(t, t.TempDir())

	out, err := runRoot(t, "memory", "lint")

	if err != nil || !strings.Contains(out, "No memory store found") || strings.Contains(out, "ok") {
		t.Fatalf("want a no-store notice (err %v):\n%s", err, out)
	}
}

func TestMemoryList_ReadsTopLevelTypeWhenMetadataTypeIsMissing(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": "- [Old style](old.md): auto memory fact\n",
		"old.md":    "---\nname: old\ndescription: auto memory fact\ntype: feedback\n---\n\nThe fact.\n",
	})

	out, err := runRoot(t, "memory", "list")

	if err != nil || !strings.Contains(out, "feedback") {
		t.Fatalf("want the top-level type (err %v):\n%s", err, out)
	}
}

func TestMemoryIndex_RepairsConflictedIndexKeepingOrder(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	conflicted := strings.Join([]string{
		"# Team memory",
		"",
		"Facts the team confirmed.",
		"- [CI runs on Ubuntu](ci.md): PR CI is Ubuntu only",
		"<<<<<<< HEAD",
		"- [Zola pin](zola.md): pin 0.23.6",
		"=======",
		"- [Gone](gone.md): deleted fact",
		"- [Zola pin](zola.md): pin 0.23.6",
		">>>>>>> other",
		"",
	}, "\n")
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": conflicted,
		"ci.md":     memoryFact("ci", "project", "PR CI is Ubuntu only"),
		"zola.md":   memoryFact("zola", "project", "pin 0.23.6"),
		"alpha.md":  memoryFact("alpha-fact", "reference", "Where alpha lives"),
	})

	if out, err := runRoot(t, "memory", "index"); err != nil {
		t.Fatalf("memory index: %v\n%s", err, out)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Team memory\n\nFacts the team confirmed.\n" +
		"- [CI runs on Ubuntu](ci.md): PR CI is Ubuntu only\n" +
		"- [Zola pin](zola.md): pin 0.23.6\n" +
		"- [alpha-fact](alpha.md): Where alpha lives\n"
	if string(data) != want {
		t.Errorf("index:\n%s\nwant:\n%s", data, want)
	}
	if got, err := lintMemory(); err != nil || len(got) != 0 {
		t.Errorf("a rebuilt index should lint clean, got %v", got)
	}
}

func TestMemoryIndex_SkipsMissingStore(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if out, err := runRoot(t, "memory", "index"); err != nil {
		t.Fatalf("memory index: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai")); !os.IsNotExist(err) {
		t.Errorf("memory index must not create a missing store: %v", err)
	}
}

func TestMemoryList_PrintsScopeTypeAndTitle(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"MEMORY.md": "- [CI runs on Ubuntu](ci.md): PR CI is Ubuntu only\n",
		"ci.md":     memoryFact("ci", "project", "PR CI is Ubuntu only"),
	})
	writeMemory(t, dir, ".agnostic-ai/local/memory", map[string]string{
		"MEMORY.md": "",
		"tabs.md":   memoryFact("prefers-tabs", "user", "Prefers tabs"),
	})

	out, err := runRoot(t, "memory", "list")

	if err != nil {
		t.Fatalf("memory list: %v\n%s", err, out)
	}
	for _, want := range []string{"personal", "user", "prefers-tabs", "project", "CI runs on Ubuntu"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Index(out, "prefers-tabs") < strings.Index(out, "CI runs on Ubuntu") {
		t.Errorf("project facts should come first:\n%s", out)
	}
}

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}
