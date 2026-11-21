package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestZedUserInstructions_FlatpakRootPrecedesXDG(t *testing.T) {
	home, xdg, flatpak := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("FLATPAK_XDG_CONFIG_HOME", flatpak)
	g := userTargets["zed"]
	for _, goos := range []string{"linux", "freebsd"} {
		if got, want := g.pathForOS(home, g.instructions, goos), filepath.Join(flatpak, "zed", "AGENTS.md"); got != want {
			t.Errorf("%s path=%q, want %q", goos, got, want)
		}
	}
	if got, want := g.pathForOS(home, g.instructions, "darwin"), filepath.Join(home, ".config", "zed", "AGENTS.md"); got != want {
		t.Errorf("macOS path=%q, want %q", got, want)
	}
	if got := userTargets["goose"].pathForOS(home, userTargets["goose"].instructions, "linux"); !strings.HasPrefix(got, xdg+string(filepath.Separator)) {
		t.Errorf("other target moved: %q", got)
	}
}

func TestZedUserInstructions_NativeRootMigratesRecordedBlock(t *testing.T) {
	for _, goos := range []string{"darwin", "linux", "freebsd"} {
		t.Run(goos, func(t *testing.T) {
			for _, owned := range []bool{false, true} {
				t.Run(map[bool]string{false: "unowned", true: "owned"}[owned], func(t *testing.T) {
					home, source := userAgentTestHome(t)
					xdg, flatpak := t.TempDir(), t.TempDir()
					t.Setenv("XDG_CONFIG_HOME", xdg)
					t.Setenv("FLATPAK_XDG_CONFIG_HOME", flatpak)
					legacy := filepath.Join(xdg, "zed", "AGENTS.md")
					native := filepath.Join(flatpak, "zed", "AGENTS.md")
					if goos == "darwin" {
						native = filepath.Join(home, ".config", "zed", "AGENTS.md")
					}
					personal := "# Private\r\nKeep these bytes"
					original := personal + "\n" + userStart + "\nOld portable text\n" + userEnd + "\n"
					writeFile(t, legacy, original)
					if err := os.Chmod(legacy, 0o600); err != nil {
						t.Fatal(err)
					}
					writeFile(t, native, personal)
					if err := os.Chmod(native, 0o600); err != nil {
						t.Fatal(err)
					}
					old := userState{Version: userStateVersion}
					if owned {
						old.Files = []string{legacy}
						old.Sums = map[string]string{legacy: userSum([]byte(original))}
					}
					writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("New portable text"), spec.Bundle{}, old, nil, io.Discard, "warn", goos)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = applyUserChanges(writes, removedUserFiles(old.Files, next.Files, writes...), false, next.removalGuards); err != nil {
						t.Fatal(err)
					}
					want := original
					if owned {
						want = personal
					}
					if got := readFile(t, legacy); got != want {
						t.Errorf("legacy=%q, want %q", got, want)
					}
					if got := readFile(t, native); !strings.HasPrefix(got, personal) || !strings.Contains(got, "New portable text") {
						t.Errorf("native=%q", got)
					}
					writes, removed, err := buildUserWritesForPlatform(home, source, []string{"zed"}, nil, spec.Bundle{}, next, nil, io.Discard, "warn", goos)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = applyUserChanges(writes, removedUserFiles(next.Files, removed.Files, writes...), false, removed.removalGuards); err != nil {
						t.Fatal(err)
					}
					if got := readFile(t, native); got != personal {
						t.Errorf("restored native=%q", got)
					}
					if info, err := os.Stat(native); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
						t.Errorf("native private mode: %v, %v", info, err)
					}
					if !slices.Contains(next.Files, native) {
						t.Errorf("native ownership missing: %v", next.Files)
					}
				})
			}
		})
	}
}

func TestZedUserInstructions_RejectsUnsafeFlatpakRoot(t *testing.T) {
	for _, root := range []string{"", "relative", "../outside"} {
		t.Run(root, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			t.Setenv("FLATPAK_XDG_CONFIG_HOME", root)
			for _, goos := range []string{"linux", "freebsd"} {
				if _, _, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("Portable text"), spec.Bundle{}, userState{}, nil, io.Discard, "warn", goos); err == nil || !strings.Contains(err.Error(), "FLATPAK_XDG_CONFIG_HOME") {
					t.Errorf("%s unsafe root accepted: %v", goos, err)
				}
			}
			for _, goos := range []string{"darwin", "windows"} {
				if err := userTargets["zed"].instructionsRootError(goos); err != nil {
					t.Errorf("Flatpak root affected %s: %v", goos, err)
				}
			}
		})
	}
}

func TestZedUserInstructions_FlatpakOwnershipRoot(t *testing.T) {
	home, flatpak := t.TempDir(), t.TempDir()
	t.Setenv("FLATPAK_XDG_CONFIG_HOME", flatpak)
	want := filepath.Join(flatpak, "zed")
	for _, goos := range []string{"linux", "freebsd"} {
		if !slices.Contains(userRootsForOS(home, goos), want) {
			t.Errorf("%s native root not recognized", goos)
		}
	}
	if slices.Contains(userRootsForOS(home, "darwin"), want) {
		t.Error("Flatpak root accepted on macOS")
	}
}
