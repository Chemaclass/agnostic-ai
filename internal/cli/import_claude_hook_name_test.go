package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// A spec a person wrote at the readable name belongs to another hook, so
// the import keeps it and adds the hook under the hashed name. A rerun
// finds its own file there and updates it in place.
func TestImportClaudeHooks_KeepsASpecAtTheReadableName(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"Notification":[{"hooks":[{"type":"command","command":"notify.sh"}]}]}}`)
	dst := filepath.Join(root, "hooks")
	own := "name: notification-notify-sh\nevent: Notification\ncommand: say done\n"
	writeFile(t, filepath.Join(dst, "notification-notify-sh.yaml"), own)

	for run := 1; run <= 2; run++ {
		if _, err := importClaudeHooks(root, dst); err != nil {
			t.Fatalf("import run %d: %v", run, err)
		}
	}

	if got := readFileString(t, filepath.Join(dst, "notification-notify-sh.yaml")); got != own {
		t.Errorf("hand-written spec changed:\n%s", got)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	var imported []string
	for _, e := range entries {
		if e.Name() != "notification-notify-sh.yaml" {
			imported = append(imported, e.Name())
		}
	}
	if len(imported) != 1 || !strings.HasPrefix(imported[0], "notification-notify-sh-") {
		t.Fatalf("imported specs = %v, want one notification-notify-sh-<hash>.yaml", imported)
	}
	if !strings.Contains(readFileString(t, filepath.Join(dst, imported[0])), "notify.sh") {
		t.Errorf("%s does not hold the imported hook", imported[0])
	}
}

// A rerun updates the readable file the first import wrote.
func TestImportClaudeHooks_RerunUpdatesItsReadableSpec(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard.sh"}]}],
		  "Stop":[{"hooks":[{"type":"http","url":"https://hooks.example.com/stop"}]}]}}`)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	for run := 1; run <= 2; run++ {
		if _, err := importClaudeHooks(root, dst); err != nil {
			t.Fatalf("import run %d: %v", run, err)
		}
	}

	entries, err := os.ReadDir(dst)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"pretooluse-bash-guard-sh.yaml", "stop-hooks-example-com.yaml"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("specs after rerun = %v, want %v", names, want)
	}
}

// A rerun finds the readable spec it wrote in the portable form and
// updates it instead of adding a hashed copy.
func TestImportClaudeHooks_RerunUpdatesItsPortableSpec(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"guard.sh"}]}]}}`)
	dst := filepath.Join(root, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := importHookConfig
	importHookConfig = &config.Config{Targets: []string{"claude"}}
	t.Cleanup(func() { importHookConfig = prior })

	for run := 1; run <= 2; run++ {
		if _, err := importClaudeHooks(root, dst); err != nil {
			t.Fatalf("import run %d: %v", run, err)
		}
	}

	names := readDirNames(t, dst)
	if strings.Join(names, ",") != "pretooluse-bash-guard-sh.yaml" {
		t.Errorf("specs after rerun = %v, want only pretooluse-bash-guard-sh.yaml", names)
	}
	if got := readFileString(t, filepath.Join(dst, "pretooluse-bash-guard-sh.yaml")); !strings.Contains(got, "on: before-tool\n") {
		t.Errorf("want the portable form:\n%s", got)
	}
}

// Identical hook groups keep one spec each, on the first import and on
// every rerun.
func TestImportClaudeHooks_IdenticalGroupsStayApartOnRerun(t *testing.T) {
	for _, hook := range []string{
		`{"type":"command","command":"notify.sh"}`,
		`{"type":"prompt","prompt":"Check the result."}`,
	} {
		root := t.TempDir()
		group := `{"hooks":[` + hook + `]}`
		writeFile(t, filepath.Join(root, ".claude", "settings.json"),
			`{"hooks":{"Stop":[`+group+`,`+group+`,`+group+`]}}`)
		dst := filepath.Join(root, "hooks")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatal(err)
		}
		for run := 1; run <= 2; run++ {
			n, err := importClaudeHooks(root, dst)
			if err != nil {
				t.Fatalf("import run %d: %v", run, err)
			}
			entries, err := os.ReadDir(dst)
			if err != nil {
				t.Fatal(err)
			}
			if n != 3 || len(entries) != 3 {
				t.Errorf("%s run %d: imported %d hooks into %d specs, want 3 and 3", hook, run, n, len(entries))
			}
		}
	}
}
