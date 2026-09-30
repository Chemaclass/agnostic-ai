package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/protecthook/hooktest"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func protectSettings(groups ...map[string]any) []spec.Entry {
	var entries []spec.Entry
	for i, g := range groups {
		name := "protected" + string(rune('a'+i))
		entries = append(entries, spec.Entry{Kind: spec.KindSettings, Name: name, Path: "settings/" + name + ".yaml", Meta: map[string]any{"protected": g}})
	}
	return entries
}

func emitProtect(t *testing.T, entries []spec.Entry) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	return dir
}

type protectHookDefinition struct {
	Matcher string `json:"matcher"`
	Hooks   []struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"hooks"`
}

func beforeToolHooks(t *testing.T, root string) []protectHookDefinition {
	t.Helper()
	var doc struct {
		Hooks map[string][]protectHookDefinition `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(root, ".gemini", "settings.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Hooks["BeforeTool"]
}

// geminiPayload is the JSON Gemini CLI writes to a BeforeTool hook's
// stdin: the base input from createBaseInput, then tool_name and the
// tool's own params as tool_input (hookEventHandler.ts).
func geminiPayload(t *testing.T, cwd, tool string, input map[string]any) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id":            "s",
		"transcript_path":       "",
		"cwd":                   cwd,
		"hook_event_name":       "BeforeTool",
		"timestamp":             "2026-09-30T00:00:00.000Z",
		"tool_name":             tool,
		"tool_input":            input,
		"original_request_name": tool,
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func writeFile(path string) map[string]any {
	return map[string]any{"file_path": path, "content": "x"}
}

func replaceIn(path string) map[string]any {
	return map[string]any{"file_path": path, "old_string": "a", "new_string": "b", "instruction": "change a"}
}

// runProtectHook runs the generated script from root, which is where
// Gemini CLI starts hooks, under every sh and awk it finds.
func runProtectHook(t *testing.T, root, tool string, input map[string]any) hooktest.Result {
	t.Helper()
	return runProtectHookReporting(t, root, root, tool, input)
}

func runProtectHookReporting(t *testing.T, root, reported, tool string, input map[string]any) hooktest.Result {
	t.Helper()
	run := hooktest.RunAll(t, protectScript(root), root, geminiPayload(t, reported, tool, input))
	if run.Stdout != "" {
		t.Errorf("stdout = %q; Gemini CLI reads stdout before stderr, so it must stay empty", run.Stdout)
	}
	return run
}

func protectScript(root string) string {
	return filepath.Join(root, ".gemini", "hooks", protecthook.ScriptName)
}

func touch(t *testing.T, root string, rels ...string) {
	t.Helper()
	for _, rel := range rels {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("a\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestEmit_ProtectedPathsWriteABeforeToolHook(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**"}}))

	groups := beforeToolHooks(t, root)
	if len(groups) != 1 || groups[0].Matcher != "^(write_file|replace)$" || len(groups[0].Hooks) != 1 ||
		groups[0].Hooks[0].Type != "command" || groups[0].Hooks[0].Command != "sh .gemini/hooks/"+protecthook.ScriptName {
		t.Fatalf("BeforeTool = %+v", groups)
	}
	if !IsProtectHookCommand(groups[0].Hooks[0].Command) {
		t.Errorf("IsProtectHookCommand(%q) = false", groups[0].Hooks[0].Command)
	}
	matcher := regexp.MustCompile(groups[0].Matcher)
	for tool, want := range map[string]bool{"write_file": true, "replace": true, "read_file": false, "mcp_fs_replace": false, "write_file_x": false} {
		if got := matcher.MatchString(tool); got != want {
			t.Errorf("matcher on %s = %v, want %v", tool, got, want)
		}
	}

	info, err := os.Stat(protectScript(root))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script mode = %v, want executable", info.Mode())
	}
}

func TestEmit_NoProtectedPathsWritesNoProtectHook(t *testing.T) {
	root := emitProtect(t, []spec.Entry{{Kind: spec.KindSettings, Name: "m", Path: "settings/m.yaml", Meta: map[string]any{"model": "gemini-2.5-pro"}}})
	if _, err := os.Stat(protectScript(root)); !os.IsNotExist(err) {
		t.Errorf("protect script written without protected paths: %v", err)
	}
	if groups := beforeToolHooks(t, root); len(groups) != 0 {
		t.Errorf("BeforeTool = %+v, want none", groups)
	}
}

// Gemini CLI spawns the handler's command with bash -c from the
// session cwd (hookRunner.ts), so the command as written must find and
// run the script and block with the reason on stderr.
func TestProtectHook_CommandBlocksWhenGeminiRunsItFromTheProjectRoot(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil || runtime.GOOS == "windows" {
		t.Skip("needs bash")
	}
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}, "decision": "deny", "reason": "Run composer instead."}))
	command := beforeToolHooks(t, root)[0].Hooks[0].Command

	cmd := exec.Command(bash, "-c", command)
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(string(geminiPayload(t, root, "write_file", writeFile("composer.lock"))))
	out, err := cmd.CombinedOutput()
	exit, _ := err.(*exec.ExitError)
	if exit == nil || exit.ExitCode() != 2 || !strings.Contains(string(out), "composer.lock is protected (composer.lock). Run composer instead. Do not edit it.") {
		t.Fatalf("err = %v, output = %q; want exit 2 with the reason", err, out)
	}
}

func TestProtectHook_BlocksWriteFileAndReplaceOnAProtectedPath(t *testing.T) {
	root := emitProtect(t, protectSettings(
		map[string]any{"paths": []any{".github/**", "dir with space/"}, "reason": "CI changes only on purpose."},
		map[string]any{"paths": []any{"composer.lock"}, "decision": "deny", "reason": "Run composer instead."},
	))
	touch(t, root, "composer.lock", "dir with space/notes.txt", ".github/workflows/ci.yml")

	for name, c := range map[string]struct {
		tool  string
		input map[string]any
		want  []string
	}{
		"write_file with spaces":   {"write_file", writeFile("dir with space/notes.txt"), []string{"dir with space/notes.txt is protected (dir with space/**).", "CI changes only on purpose.", "Ask the user"}},
		"replace with spaces":      {"replace", replaceIn("dir with space/notes.txt"), []string{"dir with space/notes.txt is protected"}},
		"write_file new file":      {"write_file", writeFile(".github/workflows/new.yml"), []string{".github/workflows/new.yml is protected (.github/**)."}},
		"replace deny":             {"replace", replaceIn("composer.lock"), []string{"composer.lock is protected (composer.lock). Run composer instead. Do not edit it."}},
		"write_file absolute":      {"write_file", writeFile(filepath.Join(root, "composer.lock")), []string{"composer.lock is protected"}},
		"replace absolute":         {"replace", replaceIn(filepath.Join(root, ".github", "workflows", "ci.yml")), []string{".github/workflows/ci.yml is protected"}},
		"write_file case variant":  {"write_file", writeFile(".GITHUB/Workflows/ci.yml"), []string{".GITHUB/Workflows/ci.yml is protected"}},
		"replace case variant":     {"replace", replaceIn("Composer.LOCK"), []string{"Composer.LOCK is protected"}},
		"write_file dot segments":  {"write_file", writeFile("src/../composer.lock"), []string{"composer.lock is protected"}},
		"write_file backslashes":   {"write_file", writeFile(`.github\workflows\ci.yml`), []string{".github/workflows/ci.yml is protected"}},
		"write_file at prefix":     {"write_file", writeFile("@composer.lock"), []string{"composer.lock is protected"}},
		"write_file NUL byte":      {"write_file", writeFile("composer\x00.lock"), []string{"composer.lock is protected"}},
		"write_file escaped quote": {"write_file", map[string]any{"content": `"file_path": "a"`, "file_path": "composer.lock"}, []string{"composer.lock is protected"}},
	} {
		t.Run(name, func(t *testing.T) {
			run := runProtectHook(t, root, c.tool, c.input)
			if run.Code != 2 {
				t.Fatalf("exit = %d, stderr = %q; want 2", run.Code, run.Stderr)
			}
			for _, want := range c.want {
				if !strings.Contains(run.Stderr, want) {
					t.Errorf("stderr misses %q:\n%s", want, run.Stderr)
				}
			}
		})
	}
}

func TestProtectHook_AllowsEditsOutsideProtectedPaths(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**", "composer.lock"}}))
	touch(t, root, "src/a b.go", "sub/composer.lock")

	for name, c := range map[string]struct {
		tool  string
		input map[string]any
	}{
		"write_file":            {"write_file", writeFile("docs/read me.md")},
		"write_file nested":     {"write_file", writeFile("sub/composer.lock")},
		"replace":               {"replace", replaceIn("src/a b.go")},
		"replace absolute":      {"replace", replaceIn(filepath.Join(root, "src", "a b.go"))},
		"outside the project":   {"write_file", writeFile(filepath.Join(t.TempDir(), "composer.lock"))},
		"key inside content":    {"write_file", map[string]any{"file_path": "notes.md", "content": "{\"file_path\": \"composer.lock\"}\n\"file_path\":\".github/x\""}},
		"key inside old_string": {"replace", map[string]any{"file_path": "src/a b.go", "old_string": `"file_path":"composer.lock"`, "new_string": "b"}},
	} {
		t.Run(name, func(t *testing.T) {
			run := runProtectHook(t, root, c.tool, c.input)
			if run.Code != 0 || run.Stderr != "" {
				t.Fatalf("exit = %d, stderr = %q; want 0 and no output", run.Code, run.Stderr)
			}
		})
	}
}

// replace looks up a relative path that does not exist from the project
// root by searching the workspace for a file whose path ends with it
// (pathCorrector.ts), so workflows/ci.yml can edit .github/workflows/ci.yml.
func TestProtectHook_BlocksAReplaceGeminiWouldSearchTheWorkspaceFor(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**"}}))
	touch(t, root, ".github/workflows/ci.yml")

	run := runProtectHook(t, root, "replace", replaceIn("workflows/ci.yml"))
	if run.Code != 2 || !strings.Contains(run.Stderr, "workflows/ci.yml does not exist from the project root") {
		t.Fatalf("exit = %d, stderr = %q; want the relative path blocked", run.Code, run.Stderr)
	}

	if run := runProtectHook(t, root, "write_file", writeFile("workflows/ci.yml")); run.Code != 0 {
		t.Errorf("write_file exit = %d, stderr = %q; write_file does not search, so want 0", run.Code, run.Stderr)
	}
}

// Gemini CLI resolves an absolute path as given, which can name the
// checkout through a symlink the hook's physical cwd does not show.
func TestProtectHook_BlocksAnAbsolutePathThroughASymlinkedCwd(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	link := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink: %v", err)
	}

	run := runProtectHookReporting(t, root, link, "write_file", writeFile(filepath.Join(link, "composer.lock")))
	if run.Code != 2 || !strings.Contains(run.Stderr, "composer.lock is protected") {
		t.Fatalf("exit = %d, stderr = %q; want composer.lock blocked", run.Code, run.Stderr)
	}
}

func TestProtectHook_BlocksWhenAwkIsMissing(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	for _, rt := range hooktest.Runtimes(t) {
		rt.Path = t.TempDir()
		run := hooktest.Run(t, rt, protectScript(root), root, geminiPayload(t, root, "write_file", writeFile("a.txt")))
		if run.Code != 2 || !strings.Contains(run.Stderr, "awk") {
			t.Errorf("%s: exit = %d, stderr = %q; want 2 naming awk", rt.Name, run.Code, run.Stderr)
		}
	}
}

func TestProtectHook_BlocksAFilePathTooLongToCheck(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))

	run := runProtectHook(t, root, "write_file", writeFile(strings.Repeat("a/", 3000)+"composer.lock"))
	if run.Code != 2 || !strings.Contains(run.Stderr, "longer than the 4096 bytes") {
		t.Fatalf("exit = %d, stderr = %q; want the long path blocked", run.Code, run.Stderr)
	}
}

// A timed-out hook lets the edit through, so decoding must stay linear
// in the content size.
func TestProtectHook_ReadsALargeWriteQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("large payload")
	}
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	var content strings.Builder
	for i := 0; i < 200000; i++ {
		fmt.Fprintf(&content, "line %d with a \"quote\", a \\ backslash, and a tab\t.\n", i)
	}

	start := time.Now()
	run := runProtectHook(t, root, "write_file", map[string]any{"content": content.String(), "file_path": "composer.lock"})
	elapsed := time.Since(start)

	if run.Code != 2 || !strings.Contains(run.Stderr, "composer.lock is protected") {
		t.Fatalf("exit = %d, stderr = %q; want composer.lock blocked", run.Code, run.Stderr)
	}
	runs := len(hooktest.Runtimes(t))
	if limit := time.Duration(runs) * 10 * time.Second; elapsed > limit {
		t.Errorf("%d runs took %s, over %s", runs, elapsed, limit)
	}
	t.Logf("%d runs took %s", runs, elapsed)
}

// The merge keeps a hooks key sync stops writing, and a handler whose
// script is gone makes sh exit 127, which Gemini CLI reads as a block on
// every edit. So the protect handler leaves with its block, and a
// handler the user wrote stays.
func TestEmit_RemovedProtectedPathsDropTheStaleHook(t *testing.T) {
	model := spec.Entry{Kind: spec.KindSettings, Name: "m", Path: "settings/m.yaml", Meta: map[string]any{"model": "gemini-2.5-pro"}}
	for name, c := range map[string]struct {
		existing string
		want     []string
		unwanted []string
	}{
		"only the protect hook": {
			existing: `{"hooks": {"BeforeTool": [{"matcher": "^(write_file|replace)$", "hooks": [{"type": "command", "command": "sh .gemini/hooks/agnostic-ai-protect.sh"}]}]}, "ui": {"theme": "Dracula"}}`,
			want:     []string{"Dracula", "gemini-2.5-pro"},
			unwanted: []string{"agnostic-ai-protect", `"hooks"`},
		},
		"beside a user hook": {
			existing: `{"hooks": {"BeforeTool": [{"matcher": "^(write_file|replace)$", "hooks": [{"type": "command", "command": "sh .gemini/hooks/agnostic-ai-protect.sh"}, {"type": "command", "command": "./mine.sh"}]}], "AfterTool": [{"hooks": [{"type": "command", "command": "echo done"}]}]}}`,
			want:     []string{"./mine.sh", "echo done", "BeforeTool"},
			unwanted: []string{"agnostic-ai-protect"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			settings := filepath.Join(dir, ".gemini", "settings.json")
			if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(settings, []byte(c.existing), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{model}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			body := readFile(t, settings)
			for _, want := range c.want {
				if !strings.Contains(body, want) {
					t.Errorf("settings miss %q:\n%s", want, body)
				}
			}
			for _, unwanted := range c.unwanted {
				if strings.Contains(body, unwanted) {
					t.Errorf("settings keep %q:\n%s", unwanted, body)
				}
			}
		})
	}
}
