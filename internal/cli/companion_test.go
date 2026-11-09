package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// builtTools are tools on a machine that has what the build needs. The
// build writes the program where the Go command would.
func builtTools(t *testing.T, lines ...string) companionTools {
	t.Helper()
	return companionTools{
		succeeds: func(context.Context, string, ...string) bool { return true },
		build: func(_ context.Context, _ string, env []string, line func(string), args ...string) error {
			for _, text := range lines {
				line(text)
			}
			for _, entry := range env {
				if into, ok := strings.CutPrefix(entry, "GOBIN="); ok {
					return os.WriteFile(filepath.Join(into, "companion"), []byte("built from "+args[len(args)-1]), 0o700)
				}
			}
			return errors.New("no GOBIN in the environment")
		},
		start:    func(string, []string) error { return nil },
		builtFor: func(context.Context, string) string { return "1.2.3" },
	}
}

func TestInstallCompanion_BuildsTheCommitThisBinaryCameFromAndSaysHowFarItIs(t *testing.T) {
	dir := t.TempDir()
	tools := builtTools(t, "go: downloading github.com/wailsapp/wails/v3 v3.1.0", "go: downloading golang.org/x/sys v0.49.0", "internal/abi", "runtime", "github.com/chemaclass/agnostic-ai/companion")
	var out bytes.Buffer

	err := installCompanion(context.Background(), &out, false, tools, dir, companionBuild{goos: "darwin", version: "1.2.3", revision: "abc123"})

	if err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if raw, _ := os.ReadFile(installed); string(raw) != "built from github.com/chemaclass/agnostic-ai/companion@abc123" {
		t.Errorf("installed program = %q", raw)
	}
	want := "Building the companion for agnostic-ai 1.2.3 on this machine.\n" +
		"✓ Go found\n" +
		"✓ the Xcode Command Line Tools found\n" +
		"→ Fetching and compiling. The first build takes a few minutes.\n" +
		"  2 modules fetched, 3 packages compiled\n" +
		"✓ Installed " + installed + "\n" +
		"Start it with: agnostic-ai companion\n"
	if out.String() != want {
		t.Errorf("output:\n%s\nwant:\n%s", out.String(), want)
	}
	if command, _ := os.ReadFile(filepath.Join(dir, companionCommandFile)); strings.TrimSpace(string(command)) == "" {
		t.Error("the path of this agnostic-ai was not written down for the companion")
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".companion-build-*")); len(left) != 0 {
		t.Errorf("the build folder was left behind: %v", left)
	}
}

func TestInstallCompanion_NamesWhatTheMachineLacks(t *testing.T) {
	for name, tc := range map[string]struct {
		goos    string
		missing string
		want    string
	}{
		"no Go":              {"darwin", "go", "the companion needs Go to build. Install Go from https://go.dev/dl/, then run this again"},
		"no Xcode tools":     {"darwin", "xcode-select", "the companion needs the Xcode Command Line Tools to build. Run: xcode-select --install, then run this again"},
		"no Linux libraries": {"linux", "pkg-config", "the companion needs GTK 4, WebKitGTK 6.0 and X11 for developers to build. Install them (Debian and Ubuntu: sudo apt install pkg-config libgtk-4-dev libwebkitgtk-6.0-dev libx11-dev), then run this again"},
	} {
		t.Run(name, func(t *testing.T) {
			tools := builtTools(t)
			tools.succeeds = func(_ context.Context, program string, _ ...string) bool { return program != tc.missing }
			tools.build = func(context.Context, string, []string, func(string), ...string) error {
				t.Error("the build started on a machine that lacks something")
				return nil
			}

			err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, t.TempDir(), companionBuild{goos: tc.goos, version: "1.2.3", revision: "abc123"})

			if err == nil || err.Error() != tc.want {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestInstallCompanion_KeepsTheInstalledCompanionWhenTheBuildFails(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("the one that works"), 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	tools.build = func(_ context.Context, _ string, _ []string, line func(string), _ ...string) error {
		line("# github.com/chemaclass/agnostic-ai/companion")
		line("./main.go:12:2: undefined: nothing")
		return errors.New("exit status 1")
	}

	err := installCompanion(context.Background(), &bytes.Buffer{}, false, tools, dir, companionBuild{goos: "windows", version: "1.2.3", revision: "abc123"})

	want := "the build failed (exit status 1):\n# github.com/chemaclass/agnostic-ai/companion\n./main.go:12:2: undefined: nothing"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
	if raw, _ := os.ReadFile(installed); string(raw) != "the one that works" {
		t.Errorf("the installed companion changed to %q", raw)
	}
}

func TestInstallCompanion_AsksForASourceWhenTheBuildRecordsNoCommit(t *testing.T) {
	err := installCompanion(context.Background(), &bytes.Buffer{}, false, builtTools(t), t.TempDir(), companionBuild{goos: "darwin", version: "1.2.3"})

	want := "this agnostic-ai does not record the commit it was built from, so it cannot fetch the companion's source; pass --source with the companion folder of an agnostic-ai checkout"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v", err)
	}
}

func TestCompanionBuild_BuildsFromACheckoutInThatFolder(t *testing.T) {
	build := companionBuild{goos: "windows", version: "1.2.3", source: "/code/agnostic-ai/companion", into: "/staging/companion"}

	args, dir := build.goArgs()

	want := "build -v -tags production private_mac_apis -trimpath -ldflags -s -w -X main.version=1.2.3 -H windowsgui -o " + filepath.Join("/staging/companion", "agnostic-ai-companion.exe") + " ."
	if strings.Join(args, " ") != want || dir != "/code/agnostic-ai/companion" {
		t.Errorf("args = %q in %q", strings.Join(args, " "), dir)
	}
	if env := build.goEnv(nil); len(env) != 0 {
		t.Errorf("Windows needs no C and a checkout build no GOBIN, got %v", env)
	}
}

func TestRunCompanion_SaysHowToInstallAMissingCompanion(t *testing.T) {
	tools := builtTools(t)
	tools.start = func(string, []string) error {
		t.Error("something was started")
		return nil
	}

	err := runCompanion(context.Background(), &bytes.Buffer{}, tools, t.TempDir(), "darwin", "1.2.3")

	if err == nil || err.Error() != "the companion is not installed yet. Build it for this machine with: agnostic-ai companion install" {
		t.Errorf("err = %v", err)
	}
}

func TestRunCompanion_StartsItAndSaysWhenItWasBuiltForAnotherRelease(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "companion", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(installed), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	tools := builtTools(t)
	var started string
	var pointsBack bool
	tools.start = func(path string, env []string) error {
		started = path
		for _, entry := range env {
			if strings.HasPrefix(entry, "AGNOSTIC_AI_BIN=") {
				pointsBack = true
			}
		}
		return nil
	}
	var out bytes.Buffer

	err := runCompanion(context.Background(), &out, tools, dir, "darwin", "1.3.0")

	if err != nil || started != installed || !pointsBack {
		t.Fatalf("err %v, started %q, told where agnostic-ai is: %t", err, started, pointsBack)
	}
	want := "The companion is on your screen. Its ⋯ menu hides it, and its shortcut brings it back.\n" +
		"! It was built for agnostic-ai \"1.2.3\" and this is 1.3.0. Build it again with: agnostic-ai companion install\n"
	if out.String() != want {
		t.Errorf("output:\n%s", out.String())
	}
}
