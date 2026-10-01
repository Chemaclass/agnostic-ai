package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
		!strings.HasSuffix(groups[0].Hooks[0].Command, "/.codex/hooks/"+protecthook.ScriptName+`"`) {
		t.Fatalf("PreToolUse = %+v", groups)
	}

	info, err := os.Stat(filepath.Join(dir, ".codex", "hooks", protecthook.ScriptName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script mode = %v, want executable", info.Mode())
	}
}

func TestEmit_NoProtectedPathsWritesNoProtectHook(t *testing.T) {
	dir := emitProtect(t, []spec.Entry{{Kind: spec.KindSettings, Name: "m", Path: "settings/m.yaml", Meta: map[string]any{"model": "gpt"}}})
	if _, err := os.Stat(filepath.Join(dir, ".codex", "hooks", protecthook.ScriptName)); !os.IsNotExist(err) {
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

// runProtectHook feeds the generated hook the JSON a Codex PreToolUse
// event sends for apply_patch, from cwd.
func runProtectHook(t *testing.T, root, cwd, patch string) hooktest.Result {
	t.Helper()
	return runProtectHookReporting(t, root, cwd, cwd, patch)
}

// runProtectHookReporting runs the hook from dir with a payload whose cwd
// field is reported, which Codex may spell through a symlink. It runs
// under every sh and awk it finds and fails when they disagree.
func runProtectHookReporting(t *testing.T, root, dir, reported, patch string) hooktest.Result {
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
	return hooktest.RunAll(t, protectScript(root), dir, payload)
}

func protectScript(root string) string {
	return filepath.Join(root, ".codex", "hooks", protecthook.ScriptName)
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
	t.Logf("stderr:\n%s", run.Stderr)

	if run.Code != 2 {
		t.Fatalf("exit = %d, want 2; stderr:\n%s", run.Code, run.Stderr)
	}
	for _, want := range []string{
		"dir with space/notes.txt", "CI changes only on purpose.", "Ask the user",
		".github/workflows/app.yml",
		"composer.lock", "Run composer instead.", "Do not edit",
	} {
		if !strings.Contains(run.Stderr, want) {
			t.Errorf("stderr misses %q:\n%s", want, run.Stderr)
		}
	}
	for _, unwanted := range []string{"docs/read me.md", "src/app.go"} {
		if strings.Contains(run.Stderr, unwanted) {
			t.Errorf("stderr names unprotected %q:\n%s", unwanted, run.Stderr)
		}
	}
}

func TestProtectHook_AllowsAPatchThatTouchesNoProtectedPath(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**", "composer.lock"}}))
	patch := "*** Begin Patch\n*** Update File: src/a b.go\n+*** Delete File: composer.lock\n-*** Move to: .github/ci.yml\n*** Add File: sub/composer.lock\n+y\n*** End Patch"

	run := runProtectHook(t, root, root, patch)

	if run.Code != 0 || run.Stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0 and no output", run.Code, run.Stderr)
	}
}

func TestProtectHook_ResolvesPathsFromASubdirectory(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	sub := filepath.Join(root, "pkg", "inner")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	run := runProtectHook(t, root, sub, "*** Begin Patch\n*** Update File: ../../composer.lock\n+x\n*** End Patch")

	if run.Code != 2 || !strings.Contains(run.Stderr, "composer.lock") {
		t.Fatalf("exit = %d, stderr = %q; want the root composer.lock blocked", run.Code, run.Stderr)
	}
}

// Codex trims a header line before it parses it, so blanks around one
// must not let the edit through.
func TestProtectHook_BlocksHeadersWithSurroundingBlanks(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock", ".github/**"}}))

	run := runProtectHook(t, root, root, "*** Begin Patch\n  *** Update File: composer.lock \n+x\n\t*** Add File: .github/a.yml\t\n+y\n*** End Patch")

	if run.Code != 2 || !strings.Contains(run.Stderr, "composer.lock is protected") || !strings.Contains(run.Stderr, ".github/a.yml is protected") {
		t.Fatalf("exit = %d, stderr = %q; want both headers blocked", run.Code, run.Stderr)
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

	if run.Code != 2 || !strings.Contains(run.Stderr, "composer.lock is protected") {
		t.Fatalf("exit = %d, stderr = %q; want composer.lock blocked", run.Code, run.Stderr)
	}
}

func TestProtectHook_CommandFindsTheScriptFromTheProjectRoot(t *testing.T) {
	meta := protectHook().Meta
	if command := meta["command"].(string); !strings.HasPrefix(command, `"$(git rev-parse --show-toplevel 2>/dev/null || pwd)/.codex/hooks/`) {
		t.Errorf("command = %s", command)
	}
	windows := meta["commandWindows"].(string)
	if !strings.HasPrefix(windows, "sh -c ") || !strings.Contains(windows, "git rev-parse --show-toplevel") || !strings.Contains(windows, "/.codex/hooks/"+protecthook.ScriptName) {
		t.Errorf("commandWindows = %s, want sh finding the script from the Git root", windows)
	}
}

// Codex trims a header line with Rust's str::trim, which strips every
// Unicode White_Space character (codex-rs apply-patch parser). The hook
// must not let one of them carry a protected header past it, and blocks
// when it cannot tell.
func TestProtectHook_BlocksHeadersWrappedInAnyWhitespace(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	for name, space := range map[string]string{
		"form feed":           "\f",
		"vertical tab":        "\v",
		"carriage return":     "\r",
		"next line":           "\u0085",
		"no-break space":      " ",
		"line separator":      " ",
		"ideographic space":   "　",
		"narrow no-break":     " ",
		"form feed and space": "\f ",
	} {
		t.Run(name, func(t *testing.T) {
			for _, header := range []string{
				space + "*** Update File: composer.lock",
				"*** Update File: composer.lock" + space,
			} {
				run := runProtectHook(t, root, root, "*** Begin Patch\n"+header+"\n+x\n*** End Patch")
				if run.Code != 2 {
					t.Errorf("%q: exit = %d, stderr = %q; want 2", header, run.Code, run.Stderr)
				}
			}
		})
	}
}

func TestProtectHook_MatchesPathsWithoutRegardToCase(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**"}}))
	sub := filepath.Join(root, "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	run := runProtectHook(t, root, sub, "*** Begin Patch\n*** Add File: ../.GITHUB/w.yml\n+x\n*** End Patch")

	if run.Code != 2 || !strings.Contains(run.Stderr, ".GITHUB/w.yml is protected") {
		t.Fatalf("exit = %d, stderr = %q; want .GITHUB/w.yml blocked", run.Code, run.Stderr)
	}
}

func TestProtectHook_ReadsBackslashesAsPathSeparators(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{".github/**"}}))

	run := runProtectHook(t, root, root, "*** Begin Patch\n*** Add File: .github\\workflows\\ci.yml\n+x\n*** End Patch")

	if run.Code != 2 || !strings.Contains(run.Stderr, ".github/workflows/ci.yml is protected") {
		t.Fatalf("exit = %d, stderr = %q; want the backslash path blocked", run.Code, run.Stderr)
	}
}

func TestProtectHook_BlocksWhenAwkIsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the generated hook needs a POSIX shell")
	}
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	for _, rt := range hooktest.Runtimes(t) {
		rt.Path = t.TempDir()
		run := hooktest.Run(t, rt, protectScript(root), root, []byte(`{"tool_input":{"command":"*** Begin Patch\n*** Update File: a.txt\n*** End Patch"}}`))
		if run.Code != 2 || !strings.Contains(run.Stderr, "awk") {
			t.Errorf("%s: exit = %d, stderr = %q; want 2 naming awk", rt.Name, run.Code, run.Stderr)
		}
	}
}

func TestProtectHook_BlocksAnInputAwkCannotRead(t *testing.T) {
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "awk"), []byte("#!/bin/sh\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, rt := range hooktest.Runtimes(t) {
		rt.Path = dir + ":/usr/bin:/bin"
		run := hooktest.Run(t, rt, protectScript(root), root, []byte(`{}`))
		if run.Code != 2 {
			t.Errorf("%s: exit = %d, stderr = %q; want 2 when awk fails", rt.Name, run.Code, run.Stderr)
		}
	}
}

// A timed-out hook lets the edit through, so decoding must stay linear
// in the patch size.
func TestProtectHook_ReadsALargePatchQuickly(t *testing.T) {
	if testing.Short() {
		t.Skip("large patch")
	}
	root := emitProtect(t, protectSettings(map[string]any{"paths": []any{"composer.lock"}}))
	var patch strings.Builder
	patch.WriteString("*** Begin Patch\n*** Add File: big.txt\n")
	for i := 0; i < 200000; i++ {
		fmt.Fprintf(&patch, "+line %d with a \"quote\", a \\ backslash, and a tab\t.\n", i)
	}
	patch.WriteString("*** Update File: composer.lock\n+x\n*** End Patch")

	start := time.Now()
	run := runProtectHook(t, root, root, patch.String())
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
