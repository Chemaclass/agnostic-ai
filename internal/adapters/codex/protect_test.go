package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
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

func TestEmit_ProtectedPathsWriteAPreToolUseHook(t *testing.T) {
	dir := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**"}}))

	raw, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	groups := doc.Hooks["PreToolUse"]
	if len(groups) != 1 || groups[0].Matcher != "apply_patch" || len(groups[0].Hooks) != 1 ||
		!strings.HasSuffix(groups[0].Hooks[0].Command, "/.codex/hooks/"+protectScriptName+`"`) {
		t.Fatalf("PreToolUse = %+v", groups)
	}

	info, err := os.Stat(filepath.Join(dir, ".codex", "hooks", protectScriptName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script mode = %v, want executable", info.Mode())
	}
}

func TestEmit_NoProtectedPathsWritesNoProtectHook(t *testing.T) {
	dir := emitProtect(t, []spec.Entry{{Kind: spec.KindSettings, Name: "m", Path: "settings/m.yaml", Meta: map[string]any{"model": "gpt"}}})
	if _, err := os.Stat(filepath.Join(dir, ".codex", "hooks", protectScriptName)); !os.IsNotExist(err) {
		t.Errorf("protect script written without protected paths: %v", err)
	}
}

func TestProtectScript_NeedsOnlyPOSIXShellAndAwk(t *testing.T) {
	script := renderProtectScript([]spec.ProtectGroup{{Paths: []string{"a"}, Decision: spec.ProtectAsk, Reason: `it's "fine" \ here`}})
	if !strings.HasPrefix(script, "#!/bin/sh\n") {
		t.Errorf("script does not start with a POSIX sh shebang:\n%s", script)
	}
	for _, tool := range []string{"jq", "python", "bash", "agnostic-ai hook", "node", "perl"} {
		if strings.Contains(script, tool+" ") {
			t.Errorf("script calls %s:\n%s", tool, script)
		}
	}
}

type hookRun struct {
	code   int
	stderr string
}

// runProtectHook feeds the generated hook the JSON a Codex PreToolUse
// event sends for apply_patch, from cwd.
func runProtectHook(t *testing.T, root, cwd, patch string) hookRun {
	t.Helper()
	return runProtectHookReporting(t, root, cwd, cwd, patch)
}

// runProtectHookReporting runs the hook from dir with a payload whose cwd
// field is reported, which Codex may spell through a symlink.
func runProtectHookReporting(t *testing.T, root, dir, reported, patch string) hookRun {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the generated hook needs a POSIX shell")
	}
	var shells []string
	for _, name := range []string{"sh", "dash"} {
		if path, err := exec.LookPath(name); err == nil {
			shells = append(shells, path)
		}
	}
	if len(shells) == 0 {
		t.Skip("no sh on PATH")
	}
	first := runProtectHookWith(t, shells[0], root, dir, reported, patch)
	for _, sh := range shells[1:] {
		if got := runProtectHookWith(t, sh, root, dir, reported, patch); got != first {
			t.Errorf("%s: %+v, but %s: %+v", sh, got, shells[0], first)
		}
	}
	return first
}

func runProtectHookWith(t *testing.T, sh, root, dir, reported, patch string) hookRun {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"session_id":      "s",
		"hook_event_name": "PreToolUse",
		"cwd":             reported,
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"command": patch},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, filepath.Join(root, ".codex", "hooks", protectScriptName))
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return hookRun{code: 0, stderr: stderr.String()}
	case errors.As(err, &exit):
		return hookRun{code: exit.ExitCode(), stderr: stderr.String()}
	}
	t.Fatalf("run hook: %v", err)
	return hookRun{}
}

func TestProtectHook_BlocksEveryProtectedFileInAMultiFilePatch(t *testing.T) {
	root := emitProtect(t, protectSettings(
		map[string]any{"paths": []any{".github/**", "dir with space/"}, "reason": "CI changes only on purpose."},
		map[string]any{"paths": []any{"composer.lock"}, "decision": "deny", "reason": "Run composer instead."},
	))
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: docs/read me.md",
		"+*** Update File: composer.lock",
		"+a \"quoted\" line with a \\ backslash",
		"*** Update File: dir with space/notes.txt",
		"@@",
		"-old",
		"+new",
		"*** Update File: src/app.go",
		"*** Move to: .github/workflows/app.yml",
		"@@",
		"-x",
		"+y",
		"*** Delete File: " + filepath.Join(root, "composer.lock"),
		"*** End Patch",
	}, "\n")

	run := runProtectHook(t, root, root, patch)
	t.Logf("stderr:\n%s", run.stderr)

	if run.code != 2 {
		t.Fatalf("exit = %d, want 2; stderr:\n%s", run.code, run.stderr)
	}
	for _, want := range []string{
		"dir with space/notes.txt", "CI changes only on purpose.", "Ask the user",
		".github/workflows/app.yml",
		"composer.lock", "Run composer instead.", "Do not edit",
	} {
		if !strings.Contains(run.stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, run.stderr)
		}
	}
	for _, unwanted := range []string{"docs/read me.md", "src/app.go"} {
		if strings.Contains(run.stderr, unwanted) {
			t.Errorf("stderr names unprotected %q:\n%s", unwanted, run.stderr)
		}
	}
}

func TestProtectHook_AllowsAPatchThatTouchesNoProtectedPath(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**", "composer.lock"}}))
	patch := "*** Begin Patch\n*** Update File: src/a b.go\n+*** Delete File: composer.lock\n-*** Move to: .github/ci.yml\n*** Add File: sub/composer.lock\n+y\n*** End Patch"

	run := runProtectHook(t, root, root, patch)

	if run.code != 0 || run.stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0 and no output", run.code, run.stderr)
	}
}

func TestProtectHook_ResolvesPathsFromASubdirectory(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	sub := filepath.Join(root, "pkg", "inner")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	run := runProtectHook(t, root, sub, "*** Begin Patch\n*** Update File: ../../composer.lock\n+x\n*** End Patch")

	if run.code != 2 || !strings.Contains(run.stderr, "composer.lock") {
		t.Fatalf("exit = %d, stderr = %q; want the root composer.lock blocked", run.code, run.stderr)
	}
}

// Codex trims a header line before it parses it, so blanks around one
// must not let the edit through.
func TestProtectHook_BlocksHeadersWithSurroundingBlanks(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock", ".github/**"}}))

	run := runProtectHook(t, root, root, "*** Begin Patch\n  *** Update File: composer.lock \n+x\n\t*** Add File: .github/a.yml\t\n+y\n*** End Patch")

	if run.code != 2 || !strings.Contains(run.stderr, "composer.lock is protected") || !strings.Contains(run.stderr, ".github/a.yml is protected") {
		t.Fatalf("exit = %d, stderr = %q; want both headers blocked", run.code, run.stderr)
	}
}

// Codex builds absolute paths from its own cwd, which can name the
// checkout through a symlink the hook's physical cwd does not show.
func TestProtectHook_BlocksAnAbsolutePathThroughASymlinkedCwd(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	link := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlink: %v", err)
	}

	run := runProtectHookReporting(t, root, root, link, "*** Begin Patch\n*** Delete File: "+link+"/composer.lock\n*** End Patch")

	if run.code != 2 || !strings.Contains(run.stderr, "composer.lock is protected") {
		t.Fatalf("exit = %d, stderr = %q; want composer.lock blocked", run.code, run.stderr)
	}
}

func TestProtectHook_CommandFindsTheScriptFromTheProjectRoot(t *testing.T) {
	meta := protectHook().Meta
	if command := meta["command"].(string); !strings.HasPrefix(command, `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/`) {
		t.Errorf("command = %s", command)
	}
	if windows := meta["commandWindows"].(string); windows != "sh .codex/hooks/"+protectScriptName {
		t.Errorf("commandWindows = %s", windows)
	}
}
