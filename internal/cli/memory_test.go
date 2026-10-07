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

func TestMemoryIndex_RepairsConflictedIndexKeepingOrder(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	conflicted := strings.Join([]string{
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
	want := "- [CI runs on Ubuntu](ci.md): PR CI is Ubuntu only\n" +
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
