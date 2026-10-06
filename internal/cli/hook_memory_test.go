package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runHookMemory(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append([]string{"hook", "memory"}, args...))
	if err := root.Execute(); err != nil {
		t.Fatalf("hook memory: %v\n%s", err, out.String())
	}
	return out.String()
}

func memoryHookProject(t *testing.T, index string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\n")
	if index != "" {
		writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), index)
	}
}

const sampleIndex = "- [CI is Ubuntu only](ci-ubuntu.md): PR CI runs on Ubuntu alone\n"

func TestHookMemory_PrintsTheIndexAsPlainTextForCodex(t *testing.T) {
	memoryHookProject(t, sampleIndex)

	got := runHookMemory(t, "--target", "codex")
	if !strings.Contains(got, "CI is Ubuntu only") || !strings.Contains(got, ".agnostic-ai/memory/") {
		t.Errorf("got %q", got)
	}
	if strings.HasPrefix(strings.TrimSpace(got), "{") {
		t.Errorf("codex gets plain text, got JSON: %q", got)
	}
}

func TestHookMemory_ReadsTheTargetFromTheHookEnvironment(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	t.Setenv("AGNOSTIC_AI_TARGET", "cursor")

	var reply struct {
		AdditionalContext string `json:"additional_context"`
	}
	if err := json.Unmarshal([]byte(runHookMemory(t)), &reply); err != nil || !strings.Contains(reply.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("cursor reply = %+v, err %v", reply, err)
	}
}

func TestHookMemory_RepliesWithAdditionalContextForCopilot(t *testing.T) {
	memoryHookProject(t, "- [Quote \"x\" and \\ path](q.md): café\n")

	out := runHookMemory(t, "--target", "copilot")
	var reply struct {
		AdditionalContext string `json:"additionalContext"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if !strings.Contains(reply.AdditionalContext, `Quote "x" and \ path`) || !strings.Contains(reply.AdditionalContext, "café") {
		t.Errorf("context lost text: %q", reply.AdditionalContext)
	}
}

func TestHookMemory_PrintsNothingWithoutAStore(t *testing.T) {
	for _, target := range []string{"codex", "cursor", "copilot", "qoder", "factory"} {
		memoryHookProject(t, "")
		if got := runHookMemory(t, "--target", target); got != "" {
			t.Errorf("%s: got %q, want nothing", target, got)
		}
	}
}

func TestHookMemory_PrintsNothingOutsideAProject(t *testing.T) {
	testutil.TempCwd(t)

	if got := runHookMemory(t, "--target", "codex"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestHookMemory_CutsALongIndexAtAWholeLine(t *testing.T) {
	line := "- [Fact](fact.md): " + strings.Repeat("x", 80) + "\n"
	memoryHookProject(t, strings.Repeat(line, 500))

	got := runHookMemory(t, "--target", "codex")
	if len(got) > memoryContextLimit {
		t.Errorf("output is %d characters, over %d", len(got), memoryContextLimit)
	}
	if !strings.Contains(got, "index continues in .agnostic-ai/memory/MEMORY.md") {
		t.Errorf("no note naming the cut index:\n%s", got[len(got)-200:])
	}
	for _, l := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(l, "- [Fact]") && l != strings.TrimSuffix(line, "\n") {
			t.Fatalf("a line was cut in half: %q", l)
		}
	}
}

func TestHookMemory_CutsOneLongLineAtACharacter(t *testing.T) {
	memoryHookProject(t, "- [Fact](fact.md): "+strings.Repeat("é", 6000)+"\n")

	got := runHookMemory(t, "--target", "codex")
	if len(got) > memoryContextLimit || !utf8.ValidString(got) {
		t.Fatalf("output is %d bytes, valid UTF-8 %v", len(got), utf8.ValidString(got))
	}
	if !strings.Contains(got, "- [Fact](fact.md): éé") {
		t.Errorf("the index body was dropped:\n%s", got)
	}
}

// A global install reaches checkouts with no project config of their own.
func TestHookMemory_FallsBackToTheGitCheckout(t *testing.T) {
	dir := testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), sampleIndex)
	if err := os.MkdirAll(filepath.Join(dir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, filepath.Join(dir, "src"))

	if got := runHookMemory(t, "--target", "codex"); !strings.Contains(got, "CI is Ubuntu only") {
		t.Errorf("got %q", got)
	}
}

// The global source root holds a config but is never a project.
func TestHookMemory_SkipsTheGlobalSourceRoot(t *testing.T) {
	dir := testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\n")
	writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), sampleIndex)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))

	if got := runHookMemory(t, "--target", "codex"); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

func TestHookMemory_SkipsAnIndexThatLeavesTheProject(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(secret, []byte("token=hunter2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, link := range map[string]func(dir string) error{
		"file": func(dir string) error {
			if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "memory"), 0o755); err != nil {
				return err
			}
			return os.Symlink(secret, filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md"))
		},
		"folder": func(dir string) error {
			if err := os.Rename(secret, filepath.Join(filepath.Dir(secret), "MEMORY.md")); err != nil {
				return err
			}
			secret = filepath.Join(filepath.Dir(secret), "MEMORY.md")
			if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai"), 0o755); err != nil {
				return err
			}
			return os.Symlink(filepath.Dir(secret), filepath.Join(dir, ".agnostic-ai", "memory"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			memoryHookProject(t, "")
			dir, _ := os.Getwd()
			if err := link(dir); err != nil {
				t.Skipf("symlink: %v", err)
			}
			if got := runHookMemory(t, "--target", "codex"); strings.Contains(got, "hunter2") {
				t.Errorf("hook leaked a file outside the project: %q", got)
			}
		})
	}
}
