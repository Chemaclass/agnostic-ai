package emit

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRewriteWindowsNeutralHookPath_ExecutesFromDirectoryWithSpaces(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join(t.TempDir(), "user home", "hooks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const marker = "copied Windows hook\n"
	if err := os.WriteFile(filepath.Join(dir, "guard.txt"), []byte(marker), 0o644); err != nil {
		t.Fatal(err)
	}
	command := RewriteWindowsNeutralHookPath("type .agnostic-ai/scripts/guard.txt", filepath.ToSlash(dir))
	cmd := exec.Command("cmd.exe", "/C")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(cmd.Path) + ` /C "` + command + `"`}
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("cmd.exe /C %q: %v, stdout %q", command, err, out)
	}
	if string(out) != marker {
		t.Errorf("stdout = %q, want %q", out, marker)
	}
}
