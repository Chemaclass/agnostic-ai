package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

// renderReuseGolden holds what the commands below printed and wrote
// before sync shared one render per target across its readers.
const renderReuseGolden = "testdata/sync-render-reuse.golden"

// warningAdapterOnPath installs an external adapter that writes one file
// and reports one warning on each render.
func warningAdapterOnPath(t *testing.T, target string) {
	t.Helper()
	bin := t.TempDir()
	script := filepath.Join(bin, "agnostic-ai-adapter-"+target)
	mustWriteFile(t, script, "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"protocol_version\":1,\"files\":[{\"path\":\""+target+".md\",\"content\":\"from "+target+"\\n\"}],\"warnings\":[\"heads up\"]}'\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// renderReuseProject is a git checkout whose syncs run every reader of
// the shared renders: collision detection, shared skills, a commit kind,
// personal built-ins, and an external adapter that warns.
func renderReuseProject(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires a POSIX shell")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	warningAdapterOnPath(t, "acme")
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, opencode, kilo, copilot, acme]\nbuiltins: []\ngitignore:\n  enabled: true\n  commit: [skills]\nsync:\n  shared-skills: true\n")
	mustWriteFile(t, "agnostic-ai.local.yaml", "builtins: [handoff]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Project instructions.\n")
	mustWriteFile(t, ".agnostic-ai/rules/r1.md", "---\nname: r1\n---\nrule body\n")
	mustWriteFile(t, ".agnostic-ai/skills/s1/SKILL.md", "---\nname: s1\ndescription: One skill.\n---\nskill body\n")
	mustWriteFile(t, ".agnostic-ai/agents/a1.md", "---\nname: a1\ndescription: One agent.\n---\nagent body\n")
	return root
}

// runCaptured runs the CLI and returns everything it printed, with the
// command's own output first and the process streams after it.
func runCaptured(t *testing.T, args ...string) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, log := os.Stdout, os.Stderr, logOut
	os.Stdout, os.Stderr, logOut = w, w, w
	adapters.SetWarner(w)
	done := make(chan struct{})
	var streams bytes.Buffer
	go func() {
		_, _ = io.Copy(&streams, r)
		close(done)
	}()
	out, runErr := runCLI(t, args...)
	os.Stdout, os.Stderr, logOut = stdout, stderr, log
	adapters.SetWarner(os.Stderr)
	_ = w.Close()
	<-done
	errText := "<nil>"
	if runErr != nil {
		errText = runErr.Error()
	}
	return "$ agnostic-ai " + strings.Join(args, " ") + "\n--- output\n" + out + "--- streams\n" + streams.String() + "--- error\n" + errText + "\n"
}

// projectTree lists every file under root but Git's own, the personal
// exclude file aside, with its mode and sum, and every link's target. The
// ignore files and the ledger are listed in full.
func projectTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() && rel != ".git" && rel != ".git/info" {
				return filepath.SkipDir
			}
			if rel != ".git/info/exclude" {
				return nil
			}
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			lines = append(lines, rel+" -> "+filepath.ToSlash(target)+"\n")
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		switch rel {
		case ".agnostic-ai/.command-lock":
			return nil
		case ".git/info/exclude":
			start := strings.Index(string(data), personalExcludeStart)
			if start < 0 {
				start = len(data)
			}
			lines = append(lines, "=== "+rel+" "+info.Mode().String()+"\n"+string(data[start:])+"\n")
			return nil
		}
		if rel == ".gitignore" || rel == filepath.ToSlash(stateFilePath(".")) {
			lines = append(lines, "=== "+rel+" "+info.Mode().String()+"\n"+string(data)+"\n")
			return nil
		}
		lines = append(lines, fmt.Sprintf("%s %s %x\n", rel, info.Mode(), sha256.Sum256(data)))
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(lines)
	return strings.Join(lines, "")
}

var (
	syncedAtField = regexp.MustCompile(`"synced_at": *"[^"]*"`)
	elapsedTime   = regexp.MustCompile(`\b\d+(\.\d+)?(ns|µs|ms|s)\b`)
	// Each render repeats an external adapter's warnings; the count is pinned by
	// TestSync_PrintsAnExternalAdapterWarningOncePerRender.
	repeatedWarning = regexp.MustCompile(`(! acme: heads up\n)+`)
)

func TestSync_OutputMatchesBeforeRenderReuse(t *testing.T) {
	pkg, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := renderReuseProject(t)
	var got strings.Builder
	for _, args := range [][]string{
		{"sync"},
		{"sync", "-t", "claude", "-t", "codex"},
		{"sync", "--json", "-t", "claude", "-t", "acme"},
		{"sync", "--check"},
		{"sync", "--check", "-t", "copilot"},
		{"sync", "--plan"},
	} {
		got.WriteString(runCaptured(t, args...))
	}
	got.WriteString("--- tree\n" + projectTree(t, root))
	text := got.String()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		text = strings.ReplaceAll(text, resolved, "<root>")
	}
	text = strings.ReplaceAll(text, root, "<root>")
	text = syncedAtField.ReplaceAllString(text, `"synced_at": "<time>"`)
	text = elapsedTime.ReplaceAllString(text, "<elapsed>")
	text = repeatedWarning.ReplaceAllString(text, "! acme: heads up\n")

	golden := filepath.Join(pkg, renderReuseGolden)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read %s: %v (run with UPDATE_GOLDEN=1 to create it)", golden, err)
	}
	if text != string(want) {
		t.Errorf("output differs from %s:\n%s", renderReuseGolden, unifiedDiff(renderReuseGolden, string(want), text, 80))
	}
}
