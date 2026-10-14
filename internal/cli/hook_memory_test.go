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

	"github.com/chemaclass/agnostic-ai/internal/adapters"
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

func TestHookMemory_TraeRepliesWithSessionStartContext(t *testing.T) {
	memoryHookProject(t, sampleIndex)

	var reply struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(runHookMemory(t, "--target", "trae")), &reply); err != nil {
		t.Fatalf("decode Trae context: %v", err)
	}
	if reply.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(reply.HookSpecificOutput.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("Trae context = %+v", reply.HookSpecificOutput)
	}
}

func TestHookMemory_TraeUsesProjectDirectoryWhenCwdDiffers(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	project, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TRAE_PROJECT_DIR", project)
	testutil.Chdir(t, t.TempDir())
	if got := runHookMemory(t, "--target", "trae"); !strings.Contains(got, "CI is Ubuntu only") {
		t.Errorf("Trae context from %s: %q", project, got)
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

// Copilot CLI reads the top-level key and VS Code the nested one.
func TestHookMemory_RepliesWithBothContextKeysForCopilot(t *testing.T) {
	memoryHookProject(t, "- [Quote \"x\" and \\ path](q.md): café\n")

	out := runHookMemory(t, "--target", "copilot")
	var reply struct {
		AdditionalContext  string `json:"additionalContext"`
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if !strings.Contains(reply.AdditionalContext, `Quote "x" and \ path`) || !strings.Contains(reply.AdditionalContext, "café") {
		t.Errorf("context lost text: %q", reply.AdditionalContext)
	}
	if reply.HookSpecificOutput.HookEventName != "SessionStart" {
		t.Errorf("hookEventName = %q, want SessionStart", reply.HookSpecificOutput.HookEventName)
	}
	if reply.HookSpecificOutput.AdditionalContext != reply.AdditionalContext {
		t.Errorf("nested context differs from the top-level one:\n%q\n%q", reply.HookSpecificOutput.AdditionalContext, reply.AdditionalContext)
	}
}

func TestHookMemory_PrintsNothingWithoutAStore(t *testing.T) {
	for _, target := range []string{"codex", "cursor", "copilot", "qoder", "factory", "claude"} {
		memoryHookProject(t, "")
		if got := runHookMemory(t, "--target", target); got != "" {
			t.Errorf("%s: got %q, want nothing", target, got)
		}
	}
}

func TestHookMemory_NamesTheRepoStoreBeforeTheFirstPersonalFact(t *testing.T) {
	parent := repoMemoryProject(t, true)

	var reply struct {
		AdditionalContext string `json:"additional_context"`
	}
	if err := json.Unmarshal([]byte(runHookMemory(t, "--target", "cursor")), &reply); err != nil {
		t.Fatalf("decode cursor context: %v", err)
	}
	store := repoStore(t, parent, reply.AdditionalContext)
	if !strings.Contains(reply.AdditionalContext, store+"/MEMORY.md") {
		t.Errorf("personal index missing from context: %s", reply.AdditionalContext)
	}
	if _, err := os.Stat(filepath.FromSlash(store + "/MEMORY.md")); !os.IsNotExist(err) {
		t.Errorf("fresh personal index: %v, want absent", err)
	}
}

func TestHookMemory_NamesTheEmptyRepoStoreAlongsideTheProjectIndex(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), sampleIndex)

	got := runHookMemory(t, "--target", "codex")
	store := repoStore(t, parent, got)
	if !strings.Contains(got, store+"/MEMORY.md") || !strings.Contains(got, "CI is Ubuntu only") {
		t.Errorf("hook context should name both scopes: %s", got)
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
	if !strings.Contains(got, "More facts are in .agnostic-ai/local/memory/MEMORY.md and .agnostic-ai/memory/MEMORY.md") {
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
		"file in the project": func(dir string) error {
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("token=hunter2\n"), 0o600); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "memory"), 0o755); err != nil {
				return err
			}
			return os.Symlink(filepath.Join("..", "..", ".env"), filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md"))
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

// Cursor runs user hooks from ~/.cursor/ and names the workspace in
// CURSOR_PROJECT_DIR.
func TestHookMemory_StartsFromTheWorkspaceTheTargetNames(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	project, _ := os.Getwd()
	t.Setenv("CURSOR_PROJECT_DIR", project)
	testutil.Chdir(t, t.TempDir())

	if got := runHookMemory(t, "--target", "cursor"); !strings.Contains(got, "CI is Ubuntu only") {
		t.Errorf("got %q", got)
	}
}

func TestHookMemory_RepliesWithHookSpecificContextForGemini(t *testing.T) {
	memoryHookProject(t, sampleIndex)

	var reply struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	out := runHookMemory(t, "--target", "gemini")
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if reply.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(reply.HookSpecificOutput.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("gemini reply = %+v", reply)
	}
}

func TestHookMemory_RepliesWithHookSpecificContextForQoder(t *testing.T) {
	memoryHookProject(t, sampleIndex)

	var reply struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	out := runHookMemory(t, "--target", "qoder")
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if reply.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(reply.HookSpecificOutput.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("qoder reply = %+v", reply)
	}
}

func TestHookMemory_AugmentReadsBoundedContextFromTheWorkingProject(t *testing.T) {
	memoryHookProject(t, strings.Repeat(sampleIndex, 200))
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Quote \"x\"](quote.md): café\n")
	t.Setenv("GEMINI_PROJECT_DIR", t.TempDir())
	var reply struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	out := runHookMemory(t, "--target", "augment")
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("Augment reply is not JSON: %v: %q", err, out)
	}
	context := reply.HookSpecificOutput.AdditionalContext
	if reply.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(context, "café") || !strings.Contains(context, "CI is Ubuntu only") {
		t.Errorf("Augment context lost the working project's indexes: %+v", reply)
	}
	if len(context) > memoryContextLimit || !strings.Contains(context, "More facts are in") {
		t.Errorf("Augment context is not bounded: %d bytes", len(context))
	}
}

func TestHookMemory_StartsFromTheGeminiProjectDir(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	project, _ := os.Getwd()
	t.Setenv("GEMINI_PROJECT_DIR", project)
	testutil.Chdir(t, t.TempDir())

	if got := runHookMemory(t, "--target", "gemini"); !strings.Contains(got, "CI is Ubuntu only") {
		t.Errorf("got %q", got)
	}
}

func TestHookMemory_PrintsThePersonalIndexToo(t *testing.T) {
	memoryHookProject(t, sampleIndex)
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Prefers tabs](tabs.md): the user indents with tabs\n")

	got := runHookMemory(t, "--target", "codex")
	for _, want := range []string{"Project memory, `.agnostic-ai/memory/MEMORY.md`", "CI is Ubuntu only", "Personal memory, `.agnostic-ai/local/memory/MEMORY.md`", "Prefers tabs"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestHookMemory_PrintsThePersonalIndexAlone(t *testing.T) {
	memoryHookProject(t, "")
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Prefers tabs](tabs.md): the user indents with tabs\n")

	if got := runHookMemory(t, "--target", "codex"); !strings.Contains(got, "Prefers tabs") || strings.Contains(got, "Project memory") {
		t.Errorf("got %q", got)
	}
}

func TestHookMemory_KeepsThePersonalIndexWhenTheProjectIndexIsLong(t *testing.T) {
	memoryHookProject(t, strings.Repeat("- [Fact](fact.md): "+strings.Repeat("x", 80)+"\n", 100))
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Prefers tabs](tabs.md): the user indents with tabs\n")

	if got := runHookMemory(t, "--target", "codex"); !strings.Contains(got, "Prefers tabs") {
		t.Errorf("personal index dropped:\n%s", got[:300])
	}
}

// claudeMemoryProject is a Claude Code project with both memory indexes,
// whose entry point at path imports the project index, and the personal
// one too when importsPersonal is set.
func claudeMemoryProject(t *testing.T, config, path string, importsPersonal bool) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", config)
	writeFile(t, filepath.Join(".agnostic-ai", "memory", "MEMORY.md"), sampleIndex)
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Prefers tabs](tabs.md): the user indents with tabs\n")
	personal := ""
	if importsPersonal {
		personal = adapters.PersonalMemoryIndexPath
	}
	writeFile(t, path, adapters.AppendMemoryBlock("# Project\n", adapters.RenderMemoryBlock(path, personal)))
}

const claudeMemoryConfig = "version: 1\ntargets: [claude]\nbuiltins: [memory]\n"

// Claude Code's own memory never reaches a subagent, and CLAUDE.md gives
// it the project index, so the subagent hook adds the personal one.
func TestHookMemory_GivesClaudeSubagentsThePersonalIndexAlone(t *testing.T) {
	claudeMemoryProject(t, claudeMemoryConfig, "CLAUDE.md", false)

	var reply struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	out := runHookMemory(t, "--target", "claude")
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	got := reply.HookSpecificOutput
	if got.HookEventName != "SubagentStart" || !strings.Contains(got.AdditionalContext, "Personal memory, `.agnostic-ai/local/memory/MEMORY.md`") ||
		!strings.Contains(got.AdditionalContext, "Prefers tabs") || strings.Contains(got.AdditionalContext, "CI is Ubuntu only") {
		t.Errorf("claude reply = %+v", got)
	}
}

// A CLAUDE.md that still imports the personal index gives it to every
// subagent already.
func TestHookMemory_AddsNothingForClaudeWhenTheEntryPointImportsThePersonalIndex(t *testing.T) {
	for name, c := range map[string]struct{ config, path string }{
		"root CLAUDE.md":    {claudeMemoryConfig, "CLAUDE.md"},
		"moved entry point": {claudeMemoryConfig + "outputs:\n  claude:\n    file: .claude/CLAUDE.md\n", filepath.Join(".claude", "CLAUDE.md")},
	} {
		t.Run(name, func(t *testing.T) {
			claudeMemoryProject(t, c.config, c.path, true)

			if got := runHookMemory(t, "--target", "claude"); got != "" {
				t.Errorf("got %q, want nothing", got)
			}
		})
	}
}

func TestHookMemory_ClaudeWorktreeReadsTheNativeMainCheckoutStore(t *testing.T) {
	main := setupGitRepo(t)
	writeFile(t, filepath.Join(main, "agnostic-ai.yaml"), claudeMemoryConfig)
	git(t, main, "add", ".")
	git(t, main, "commit", "-qm", "memory fixture")
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	store := filepath.Join(main, ".agnostic-ai", "local", "memory")
	value, err := json.Marshal(map[string]string{"autoMemoryDirectory": filepath.ToSlash(store)})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(linked, ".claude", "settings.local.json"), string(value))
	writeFile(t, filepath.Join(store, "MEMORY.md"), "Main checkout personal fact\n")
	writeFile(t, filepath.Join(linked, ".agnostic-ai", "local", "memory", "MEMORY.md"), "Wrong worktree fact\n")
	writeFile(t, filepath.Join(linked, "CLAUDE.md"), adapters.AppendMemoryBlock("# Project\n", adapters.RenderMemoryBlock("CLAUDE.md", "")))
	testutil.Chdir(t, linked)
	for _, cwd := range []string{linked, filepath.Join(linked, "nested")} {
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatal(err)
		}
		testutil.Chdir(t, cwd)
		out := runHookMemory(t, "--target", "claude")
		if !strings.Contains(out, "Main checkout personal fact") || strings.Contains(out, "Wrong worktree fact") {
			t.Errorf("hook from %s must use Claude's native store: %s", cwd, out)
		}
	}
}
