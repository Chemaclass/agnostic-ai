package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestZedUserInstructions_PlatformPaths(t *testing.T) {
	t.Setenv("FLATPAK_XDG_CONFIG_HOME", "")
	if err := os.Unsetenv("FLATPAK_XDG_CONFIG_HOME"); err != nil {
		t.Fatal(err)
	}
	home, appdata, xdg := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("APPDATA", appdata)
	t.Setenv("XDG_CONFIG_HOME", xdg)
	g := userTargets["zed"]
	for _, tc := range []struct{ goos, want string }{
		{"windows", filepath.Join(appdata, "Zed", "AGENTS.md")},
		{"darwin", filepath.Join(home, ".config", "zed", "AGENTS.md")},
		{"linux", filepath.Join(xdg, "zed", "AGENTS.md")},
		{"freebsd", filepath.Join(xdg, "zed", "AGENTS.md")},
	} {
		if got := g.pathForOS(home, g.instructions, tc.goos); got != tc.want {
			t.Errorf("%s instructions path = %q, want %q", tc.goos, got, tc.want)
		}
	}
	t.Setenv("APPDATA", "")
	if got, want := g.pathForOS(home, g.instructions, "windows"), filepath.Join(home, "AppData", "Roaming", "Zed", "AGENTS.md"); got != want {
		t.Errorf("Windows fallback = %q, want %q", got, want)
	}
	if got, want := g.pathForOS(home, g.skills, "windows"), filepath.Join(home, ".agents", "skills"); got != want {
		t.Errorf("Windows skills changed = %q, want %q", got, want)
	}
}

func TestZedUserInstructions_RejectsRelativeWindowsRoot(t *testing.T) {
	t.Setenv("APPDATA", "relative")
	g := userTargets["zed"]
	if err := g.instructionsRootError("windows"); err == nil || !strings.Contains(err.Error(), "APPDATA") {
		t.Errorf("relative Windows root accepted: %v", err)
	}
	if err := g.instructionsRootError("darwin"); err != nil {
		t.Errorf("Windows root affected macOS: %v", err)
	}
}

func TestZedUserInstructions_WriteExplainCheckAndRestore(t *testing.T) {
	for _, before := range []string{"absent", "private native file"} {
		t.Run(before, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			writeFile(t, filepath.Join(source, "agnostic-ai.yaml"), "version: 1\ntargets: [zed]\nbuiltins: []\n")
			g := userTargets["zed"]
			path := filepath.Join(home, ".config", "zed", "AGENTS.md")
			if runtime.GOOS == "windows" {
				path = filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md")
			}
			if got := g.path(home, g.instructions); got != path {
				t.Fatalf("native path %q, want %q", got, path)
			}
			original := "# Personal notes\r\nKeep this private"
			if before != "absent" {
				writeFile(t, path, original)
				if err := os.Chmod(path, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			rule := filepath.Join(source, "rules", "personal.md")
			writeFile(t, rule, "---\nname: personal\n---\nUse the portable marker.\n")
			if _, notes, err := runUserAgentTest("--only", "zed"); err != nil {
				t.Fatalf("sync: %v\n%s", err, notes)
			}
			added := readFile(t, path)
			if before == "absent" && runtime.GOOS != "windows" {
				if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
					t.Errorf("created default mode changed: %v, %v", info, err)
				}
			}
			if !strings.Contains(added, "Use the portable marker.") || before != "absent" && !strings.HasPrefix(added, original) {
				t.Fatalf("native instructions changed user bytes or miss rule: %q", added)
			}
			if _, notes, err := runUserAgentTest("--only", "zed", "--check"); err != nil {
				t.Fatalf("check: %v\n%s", err, notes)
			}
			explained, err := runRoot(t, "explain", "--scope", "user", "rules/personal.md")
			if err != nil || !strings.Contains(explained, filepath.ToSlash(path)) {
				t.Errorf("explain native path: %v\n%s", err, explained)
			}
			raw, err := runRoot(t, "explain", "--scope", "user", "rules/personal.md", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var view explainOutput
			if err := json.Unmarshal([]byte(raw), &view); err != nil {
				t.Fatal(err)
			}
			if !slices.ContainsFunc(view.Contributions, func(c contribution) bool {
				return c.Target == "zed" && c.Path == filepath.ToSlash(path) && c.Mode == "section"
			}) {
				t.Errorf("JSON contribution misses canonical native path: %s", raw)
			}
			if _, err := runRoot(t, "import", "--scope", "user", "zed"); err == nil || !strings.Contains(err.Error(), "has no user settings, MCP file, or hook directory") {
				t.Errorf("user instruction import contract changed: %v", err)
			}
			if got := readFile(t, path); got != added {
				t.Errorf("unsupported import changed native file: %q", got)
			}
			if err := os.Remove(rule); err != nil {
				t.Fatal(err)
			}
			if _, notes, err := runUserAgentTest("--only", "zed"); err != nil {
				t.Fatalf("remove: %v\n%s", err, notes)
			}
			if before == "absent" {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("created instructions remain: %v", err)
				}
			} else {
				if got := readFile(t, path); got != original {
					t.Errorf("restored bytes %q, want %q", got, original)
				}
				if info, err := os.Stat(path); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Errorf("private mode changed: %v, %v", info, err)
				}
			}
		})
	}
}

func TestZedUserInstructions_WindowsMigratesOnlyRecordedLegacyBlock(t *testing.T) {
	for _, before := range []string{"unowned", "owned with personal text", "sync created"} {
		t.Run(before, func(t *testing.T) {
			home, source := userAgentTestHome(t)
			g := userTargets["zed"]
			legacy := userLevelPath(home, g.instructions)
			personal := "# Personal notes\r\nKeep this"
			original := personal + "\n" + userStart + "\nOld portable text\n" + userEnd + "\n"
			if before == "sync created" {
				personal = ""
				original = userStart + "\nOld portable text\n" + userEnd + "\n"
			}
			writeFile(t, legacy, original)
			if err := os.Chmod(legacy, 0o600); err != nil {
				t.Fatal(err)
			}
			old := userState{Version: userStateVersion}
			if before != "unowned" {
				old.Files = []string{legacy}
				old.Sums = map[string]string{legacy: userSum([]byte(original))}
			}
			writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("New portable text"), spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
			if err != nil {
				t.Fatal(err)
			}
			var foundNew, foundOld bool
			for _, write := range writes {
				switch write.path {
				case filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md"):
					foundNew = true
					if !strings.Contains(string(write.data), "New portable text") {
						t.Errorf("new native file misses instructions: %q", write.data)
					}
				case legacy:
					foundOld = true
					if string(write.data) != personal {
						t.Errorf("legacy cleanup changed personal bytes: %q, want %q", write.data, personal)
					}
				}
			}
			if !foundNew || foundOld != (before == "owned with personal text") {
				t.Errorf("writes new=%t, legacy=%t for %s", foundNew, foundOld, before)
			}
			if removed := removedUserFiles(old.Files, next.Files); slices.Contains(removed, legacy) != (before == "sync created") {
				t.Errorf("wrong legacy removal: %v", removed)
			}
			if got := readFile(t, legacy); got != original {
				t.Errorf("planning changed native file: %q", got)
			}
			if _, err := applyUserChanges(writes, removedUserFiles(old.Files, next.Files, writes...), false, next.removalGuards); err != nil {
				t.Fatal(err)
			}
			if before == "sync created" {
				if _, err := os.Stat(legacy); !os.IsNotExist(err) {
					t.Errorf("created legacy file remains: %v", err)
				}
			} else {
				want := original
				if before != "unowned" {
					want = personal
				}
				if got := readFile(t, legacy); got != want {
					t.Errorf("migration changed legacy user bytes: %q, want %q", got, want)
				}
				if info, err := os.Stat(legacy); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
					t.Errorf("migration widened legacy mode: %v, %v", info, err)
				}
			}
		})
	}
}

func TestZedUserInstructions_StopsAfterNativeTextChanges(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	path := g.path(home, g.instructions)
	writeFile(t, path, "Personal text before planning.\n")
	writes, next, err := buildUserWrites(home, source, []string{"zed"}, []byte("Portable marker"), spec.Bundle{}, userState{}, nil, io.Discard, "warn")
	if err != nil {
		t.Fatal(err)
	}
	edited := "Personal text edited while sync ran.\n"
	writeFile(t, path, edited)
	if _, err := applyUserChanges(writes, nil, false, next.removalGuards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Errorf("changed native text was not guarded: %v", err)
	}
	if got := readFile(t, path); got != edited {
		t.Errorf("native edit lost: %q", got)
	}
}

func TestZedUserInstructions_RemovalStopsAfterNativeTextChanges(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	path := g.path(home, g.instructions)
	original := userStart + "\nPortable text\n" + userEnd + "\n"
	writeFile(t, path, original)
	old := userState{Version: userStateVersion, Files: []string{path}, Sums: map[string]string{path: userSum([]byte(original))}}
	writes, next, err := buildUserWrites(home, source, []string{"zed"}, nil, spec.Bundle{}, old, nil, io.Discard, "warn")
	if err != nil {
		t.Fatal(err)
	}
	edited := "Personal text edited while sync ran.\n" + original
	writeFile(t, path, edited)
	if _, err := applyUserChanges(writes, removedUserFiles(old.Files, next.Files, writes...), false, next.removalGuards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Errorf("changed native text was not guarded on removal: %v", err)
	}
	if got := readFile(t, path); got != edited {
		t.Errorf("native edit lost on removal: %q", got)
	}
}

func TestZedUserInstructions_LegacyAliasKeepsTheActiveFile(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	legacy := userLevelPath(home, g.instructions)
	native := filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(native), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(legacy)), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.DirectoryAlias(t, filepath.Dir(native), filepath.Dir(legacy))
	original := "Personal text.\n" + userStart + "\nOld portable text\n" + userEnd + "\n"
	writeFile(t, native, original)
	old := userState{Version: userStateVersion, Files: []string{legacy}, Sums: map[string]string{legacy: userSum([]byte(original))}}
	writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("New portable text"), spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	removals := removedUserFiles(old.Files, next.Files, writes...)
	if _, err := applyUserChanges(writes, removals, false, next.removalGuards); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, native); !strings.Contains(got, "New portable text") || !strings.HasPrefix(got, "Personal text.\n") {
		t.Errorf("active native instructions lost: %q", got)
	}
	if slices.Contains(next.Files, legacy) {
		t.Errorf("obsolete path remains owned: %v", next.Files)
	}
}

func TestZedUserInstructions_AbsentLegacyAliasKeepsRecreatedFile(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	legacy := userLevelPath(home, g.instructions)
	native := filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(native), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(legacy)), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.DirectoryAlias(t, filepath.Dir(native), filepath.Dir(legacy))
	old := userState{Version: userStateVersion, Files: []string{legacy}}
	writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("Portable marker"), spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	removals := removedUserFiles(old.Files, next.Files, writes...)
	if slices.Contains(removals, legacy) {
		t.Errorf("active destination was scheduled for removal: %v", removals)
	}
	if _, err := applyUserChanges(writes, removals, false, next.removalGuards); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, native); !strings.Contains(got, "Portable marker") {
		t.Errorf("native instructions = %q", got)
	}
	if slices.Contains(next.Files, legacy) || !slices.Contains(next.Files, native) {
		t.Errorf("ownership = %v", next.Files)
	}
}

func TestZedUserInstructions_AbsentAliasStopsWhenNativeAppears(t *testing.T) {
	for _, missingDir := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing directory", true: "dangling directory"}[missingDir], func(t *testing.T) {
			home, source := userAgentTestHome(t)
			g := userTargets["zed"]
			legacy := userLevelPath(home, g.instructions)
			native := filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md")
			dir := filepath.Dir(native)
			if missingDir {
				dir = filepath.Dir(dir)
			}
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(filepath.Dir(legacy)), 0o755); err != nil {
				t.Fatal(err)
			}
			testutil.DirectoryAlias(t, filepath.Dir(native), filepath.Dir(legacy))
			old := userState{Version: userStateVersion, Files: []string{legacy}}
			writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("Portable marker"), spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
			if err != nil {
				t.Fatal(err)
			}
			const personal = "User wrote this while sync ran.\n"
			writeFile(t, native, personal)
			if _, err := applyUserChanges(writes, removedUserFiles(old.Files, next.Files, writes...), false, next.removalGuards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
				t.Errorf("expected stale write refusal, got %v", err)
			}
			if got := readFile(t, native); got != personal {
				t.Errorf("user file changed: %q", got)
			}
		})
	}
}

func TestZedUserInstructions_DanglingLegacyAliasKeepsRecreatedFile(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	legacy := userLevelPath(home, g.instructions)
	native := filepath.Join(os.Getenv("APPDATA"), "Zed", "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(native)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Dir(legacy)), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.DirectoryAlias(t, filepath.Dir(native), filepath.Dir(legacy))
	old := userState{Version: userStateVersion, Files: []string{legacy}}
	writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, []byte("Portable marker"), spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	removals := removedUserFiles(old.Files, next.Files, writes...)
	if slices.Contains(removals, legacy) {
		t.Errorf("active destination was scheduled for removal: %v", removals)
	}
	if _, err := applyUserChanges(writes, removals, false, next.removalGuards); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, native); !strings.Contains(got, "Portable marker") {
		t.Errorf("native instructions = %q", got)
	}
}

func TestZedUserInstructions_AbsentLegacyRemovalKeepsNewUserFile(t *testing.T) {
	home, source := userAgentTestHome(t)
	g := userTargets["zed"]
	legacy := userLevelPath(home, g.instructions)
	old := userState{Version: userStateVersion, Files: []string{legacy}}
	writes, next, err := buildUserWritesForPlatform(home, source, []string{"zed"}, nil, spec.Bundle{}, old, nil, io.Discard, "warn", "windows")
	if err != nil {
		t.Fatal(err)
	}
	removals := removedUserFiles(old.Files, next.Files, writes...)
	if !slices.Contains(removals, legacy) {
		t.Fatalf("expected guarded stale removal, got %v", removals)
	}
	writeFile(t, legacy, "")
	if _, err := applyUserChanges(writes, removals, false, next.removalGuards); err == nil || !strings.Contains(err.Error(), "changed while sync ran") {
		t.Errorf("expected stale removal refusal, got %v", err)
	}
	if data, err := os.ReadFile(legacy); err != nil || len(data) != 0 {
		t.Errorf("new user file = %q, %v", data, err)
	}
}
