package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestScaffold_Demo_SeedsHookScriptWhereSyncReadsIt(t *testing.T) {
	dir := t.TempDir()
	if err := scaffold(scaffoldOptions{Root: dir, Base: ".", Demo: true, Targets: []string{"claude"}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "scripts", "no-force-push.sh"))
	if err != nil {
		t.Fatalf("a root-level base must still seed the script under .agnostic-ai/scripts/: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script mode = %v, want executable since the hook runs it directly", info.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts")); !os.IsNotExist(err) {
		t.Errorf("script must not land under the base dir, stat err = %v", err)
	}
}

func TestDemoHook_BlocksForcePushOnClaudeAndCodex(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	silence(t)
	if err := scaffold(scaffoldOptions{Root: dir, Demo: true, Targets: []string{"claude", "codex"}}); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cases := []struct {
		command, expect string
	}{
		{"git push --force origin main", "block"},
		{"git push -f origin main", "block"},
		{"git fetch && git push origin main --force", "block"},
		{"git status", "allow"},
		{"git push --force-with-lease origin main", "allow"},
		{`git commit -m "drop -f" && git push`, "allow"},
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			out, err := runHookRun(t, "no-force-push", "--bash", c.command, "--expect", c.expect)
			if err != nil {
				t.Fatalf("err = %v\n%s", err, out)
			}
			for _, target := range []string{"claude: " + c.expect, "codex: " + c.expect} {
				if !strings.Contains(out, target) {
					t.Errorf("output misses %q:\n%s", target, out)
				}
			}
		})
	}
}
