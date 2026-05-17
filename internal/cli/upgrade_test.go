package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectInstallMethod_Homebrew(t *testing.T) {
	cases := []string{
		"/opt/homebrew/bin/agnostic-ai",
		"/opt/homebrew/Caskroom/agnostic-ai/0.22.0/agnostic-ai",
		"/usr/local/Cellar/agnostic-ai/0.22.0/bin/agnostic-ai",
		"/home/linuxbrew/.linuxbrew/bin/agnostic-ai",
	}
	for _, p := range cases {
		if got := detectInstallMethod(p); got != installHomebrew {
			t.Errorf("detectInstallMethod(%q) = %v, want homebrew", p, got)
		}
	}
}

func TestDetectInstallMethod_GoInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path semantics differ on windows")
	}
	tmp := t.TempDir()
	gobin := filepath.Join(tmp, "gobin")
	if err := os.MkdirAll(gobin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOBIN", gobin)
	t.Setenv("GOPATH", "")

	exe := filepath.Join(gobin, "agnostic-ai")
	if got := detectInstallMethod(exe); got != installGoInstall {
		t.Errorf("GOBIN install: got %v, want go install", got)
	}

	t.Setenv("GOBIN", "")
	gopath := filepath.Join(tmp, "gopath")
	if err := os.MkdirAll(filepath.Join(gopath, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOPATH", gopath)
	exe2 := filepath.Join(gopath, "bin", "agnostic-ai")
	if got := detectInstallMethod(exe2); got != installGoInstall {
		t.Errorf("GOPATH/bin install: got %v, want go install", got)
	}
}

func TestDetectInstallMethod_Binary(t *testing.T) {
	t.Setenv("GOBIN", "")
	t.Setenv("GOPATH", "/nowhere")
	// HOME guarded too: $HOME/go/bin should not match an unrelated path.
	t.Setenv("HOME", "/nowhere")
	if got := detectInstallMethod("/usr/local/bin/agnostic-ai"); got != installBinary {
		t.Errorf("raw binary: got %v, want binary", got)
	}
}

func TestUpgradeCommandFor(t *testing.T) {
	cases := map[installMethod]string{
		installHomebrew:  "brew update && brew upgrade Chemaclass/tap/agnostic-ai",
		installGoInstall: "go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@latest",
		installBinary:    "",
		installUnknown:   "",
	}
	for m, want := range cases {
		if got := upgradeCommandFor(m); got != want {
			t.Errorf("upgradeCommandFor(%v) = %q, want %q", m, got, want)
		}
	}
}

func TestVersionsEqual(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"0.22.0", "v0.22.0", true},
		{" 0.22.0 ", "0.22.0", true},
		{"0.22.0", "0.21.0", false},
		{"", "", true},
	}
	for _, c := range cases {
		if got := versionsEqual(c.a, c.b); got != c.want {
			t.Errorf("versionsEqual(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestOtherInstancesOnPATH_FindsShadow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH separator + exe extension differ on windows")
	}
	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "a")
	dirB := filepath.Join(tmp, "b")
	for _, d := range []string{dirA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	self := filepath.Join(dirA, "agnostic-ai")
	other := filepath.Join(dirB, "agnostic-ai")
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("y"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dirA+string(os.PathListSeparator)+dirB)

	got := otherInstancesOnPATH(self)
	if len(got) != 1 || got[0] != other {
		t.Errorf("shadows = %v, want [%s]", got, other)
	}
}

func TestOtherInstancesOnPATH_NoShadow(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH semantics differ on windows")
	}
	tmp := t.TempDir()
	self := filepath.Join(tmp, "agnostic-ai")
	if err := os.WriteFile(self, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	if got := otherInstancesOnPATH(self); len(got) != 0 {
		t.Errorf("expected no shadows, got %v", got)
	}
}

func TestPrintUpgradeInfo_RendersAllFields(t *testing.T) {
	var buf bytes.Buffer
	printUpgradeInfo(&buf, upgradeInfo{
		Path:    "/opt/homebrew/bin/agnostic-ai",
		Method:  installHomebrew,
		Version: "0.20.0",
		Latest:  "0.22.0",
		Shadows: []string{"/usr/local/bin/agnostic-ai"},
		Notes:   []string{"shadow detected"},
	})
	out := buf.String()
	for _, want := range []string{
		"Install method: homebrew",
		"Binary:         /opt/homebrew/bin/agnostic-ai",
		"Installed:      0.20.0",
		"Latest:         0.22.0",
		"PATH shadows",
		"/usr/local/bin/agnostic-ai",
		"shadow detected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestRunUpgrade_CheckOnlyPrintsAndReturns(t *testing.T) {
	var buf bytes.Buffer
	if err := runUpgrade(&buf, false, true, "0.22.0"); err != nil {
		t.Fatalf("check-only: %v", err)
	}
	if !strings.Contains(buf.String(), "Install method:") {
		t.Errorf("expected install method line, got:\n%s", buf.String())
	}
}

func TestUpgradeCmd_RegistersOnRoot(t *testing.T) {
	root := NewRootCmd("test")
	var found bool
	for _, c := range root.Commands() {
		if c.Name() == "upgrade" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("upgrade command not registered on root")
	}
}
