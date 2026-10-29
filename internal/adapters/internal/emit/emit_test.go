package emit

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"gopkg.in/yaml.v3"
)

// WriteFile refuses to write a project-relative path that climbs above the
// project root, the last line of defense against a path traversal from a
// crafted spec name/scope.
func TestWriteFile_RefusesProjectRootEscape(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	testutilChdir(t, dir)

	for _, bad := range []string{"../escape.md", ".opencode/commands/../../../escape.md", ".."} {
		if err := sess.WriteFile(bad, "x", false); err == nil {
			t.Errorf("path %q: expected escape rejection, got nil", bad)
		}
		// Confirm nothing landed outside the project dir.
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.md")); err == nil {
			t.Errorf("path %q wrote a file outside the project root", bad)
		}
	}

	// A normal in-project path still writes.
	if err := sess.WriteFile(".cursor/commands/deploy.md", "x", false); err != nil {
		t.Errorf("safe in-project path rejected: %v", err)
	}
}

// A drive-relative or rooted path names a place outside the project on
// Windows, so WriteFile refuses it with the same message as a ".." path.
func TestWriteFile_RefusesWindowsVolumeAndRootedPathsLikeAParentPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative and rooted relative paths exist only on Windows")
	}
	testutilChdir(t, t.TempDir())
	sess := NewSession()

	parentErr := sess.WriteFile(`..\escape.md`, "x", false)
	if parentErr == nil {
		t.Fatal(`..\escape.md: expected escape rejection, got nil`)
	}
	for _, bad := range []string{`C:escape.md`, `\escape.md`, `/escape.md`} {
		err := sess.WriteFile(bad, "x", false)
		want := strings.Replace(parentErr.Error(), `..\escape.md`, bad, 1)
		if err == nil || err.Error() != want {
			t.Errorf("path %q: error = %v, want %q", bad, err, want)
		}
	}

	sess.SetProjectRoot(".")
	volumePath := `Z:\escape.md`
	want := "refusing a path outside the project or inside .git: " + volumePath
	if err := sess.WriteFile(volumePath, "x", false); err == nil || err.Error() != want {
		t.Errorf("path %q: error = %v, want %q", volumePath, err, want)
	}
}

// A hand-edited file named by a drive-relative path is kept as it is: no
// backup is written for a path that resolves outside the project.
func TestRemoveOwned_KeepsAHandEditOnADriveRelativePathWithoutABackup(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths exist only on Windows")
	}
	dir := t.TempDir()
	volume := filepath.VolumeName(dir)
	if len(volume) != 2 {
		t.Skip("the temporary folder has no drive letter")
	}
	testutilChdir(t, dir)
	path := volume + "edited.md"
	if err := os.WriteFile("edited.md", []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{path: ContentSum("generated\n")})

	removed, err := sess.RemoveOwned(path, ContentSum("hand edit\n"), false)

	if err != nil || removed {
		t.Errorf("RemoveOwned(%q) = %v, %v, want false, nil", path, removed, err)
	}
	if _, err := os.Lstat("edited.md.bak"); err == nil {
		t.Error("a backup was written for a drive-relative path")
	}
	if data, _ := os.ReadFile("edited.md"); string(data) != "hand edit\n" {
		t.Errorf("edited.md = %q, want the hand edit kept", data)
	}
}

// testutilChdir changes into dir for the test and restores cwd after, kept
// local to avoid importing internal/testutil from the emit package.
func testutilChdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestWriteFile_CreatesNestedDirs(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.txt")
	if err := sess.WriteFile(path, "hello", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello\n" {
		t.Errorf("expected 'hello\\n' (normalized trailing newline), got %q", got)
	}
}

func TestWriteFile_NormalizesTrailingNewlines(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	cases := []struct{ name, in, want string }{
		{"no-newline", "hello", "hello\n"},
		{"single-newline", "hello\n", "hello\n"},
		{"double-newline", "hello\n\n", "hello\n"},
		{"many-newlines", "hello\n\n\n\n", "hello\n"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".txt")
			if err := sess.WriteFile(path, tc.in, false); err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				got, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != 0 {
					t.Errorf("empty in -> want empty file, got %q", got)
				}
				return
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("in=%q want=%q got=%q", tc.in, tc.want, got)
			}
		})
	}
}

func TestWriteExecutableFile_SetsExecutableMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix executable mode bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), "setup")

	if err := sess.WriteExecutableFile(path, "#!/bin/sh\nexit 0", false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755", info.Mode().Perm())
	}
}

func TestWriteExecutableFile_CorrectsExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix executable mode bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), "setup")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess.StartDetailedRecording()
	if err := sess.WriteExecutableFile(path, "#!/bin/sh", false); err != nil {
		t.Fatal(err)
	}
	sess.StopDetailedRecording()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755", info.Mode().Perm())
	}
}

func TestWriteFile_PreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not support Unix mode bits")
	}
	t.Parallel()
	sess := NewSession()
	path := filepath.Join(t.TempDir(), "private.toml")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := sess.WriteFile(path, "new", false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want existing mode 600", info.Mode().Perm())
	}
}

func TestWriteFile_DryRun(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "should-not-exist.txt")
	if err := sess.WriteFile(path, "hello", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("dry-run wrote a file")
	}
}

func TestRemoveGenerated_DeletesAgnosticAuthored(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "orphan.toml")
	body := "# Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`.\nkey = 1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.RemoveGenerated(path, false); err != nil {
		t.Fatalf("RemoveGenerated: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed, stat err = %v", err)
	}
}

func TestRemoveGenerated_PreservesUserAuthored(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "user.toml")
	body := "# user comment\nkey = 1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.RemoveGenerated(path, false); err != nil {
		t.Fatalf("RemoveGenerated: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file unexpectedly removed: %v", err)
	}
	if string(got) != body {
		t.Errorf("body changed: got %q", got)
	}
}

func TestRemoveGenerated_MissingFileIsNoOp(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.toml")
	if err := sess.RemoveGenerated(path, false); err != nil {
		t.Errorf("absent file should be no-op, got %v", err)
	}
}

func TestRemoveGenerated_DryRunLeavesFile(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.toml")
	body := "# Generated by agnostic-ai\nkey = 1\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.RemoveGenerated(path, true); err != nil {
		t.Fatalf("RemoveGenerated dryRun: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dry-run removed the file: %v", err)
	}
}

func TestRemoveGeneratedTree_SweepsMarkerFilesAndEmptyDirs(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	tree := filepath.Join(dir, "legacy", "agents")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Generated by agnostic-ai\nkey = 1\n"
	for _, name := range []string{"a.toml", "b.toml"} {
		if err := os.WriteFile(filepath.Join(tree, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.RemoveGeneratedTree(filepath.Join(dir, "legacy"), false); err != nil {
		t.Fatalf("RemoveGeneratedTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "legacy")); !os.IsNotExist(err) {
		t.Errorf("expected legacy dir gone, stat err=%v", err)
	}
}

func TestRemoveGeneratedTree_PreservesUserFilesAndTheirDirs(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	tree := filepath.Join(dir, "legacy", "agents")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	generated := "# Generated by agnostic-ai\nkey = 1\n"
	user := "# user keeps this\nkey = 2\n"
	if err := os.WriteFile(filepath.Join(tree, "managed.toml"), []byte(generated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "mine.toml"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sess.RemoveGeneratedTree(filepath.Join(dir, "legacy"), false); err != nil {
		t.Fatalf("RemoveGeneratedTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tree, "managed.toml")); !os.IsNotExist(err) {
		t.Errorf("expected managed file removed, stat err=%v", err)
	}
	got, err := os.ReadFile(filepath.Join(tree, "mine.toml"))
	if err != nil {
		t.Fatalf("user file unexpectedly removed: %v", err)
	}
	if string(got) != user {
		t.Errorf("user file modified: %q", got)
	}
	if _, err := os.Stat(tree); err != nil {
		t.Errorf("expected dir kept because user file remains: %v", err)
	}
}

// sync --jobs runs one target's empty-directory prune alongside another
// target's write into the same shared directory (codex sweeping its
// legacy `.agents/agents/*.toml` next to Antigravity's nested profiles).
// The write used to fail outright on the pruned parent.
func TestWriteFile_SurvivesAConcurrentPruneOfItsParent(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	shared := filepath.Join(dir, "agents", "agents")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	// Stand in for the prune landing between mkdirAll and the write.
	if err := os.Remove(shared); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(shared, "agent-0.md")
	if err := sess.WriteFile(path, "body\n", false); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "body\n" {
		t.Errorf("content = %q, want %q", got, "body\n")
	}
}

func TestRemoveGeneratedTree_MissingDirIsNoOp(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	if err := sess.RemoveGeneratedTree(filepath.Join(dir, "does-not-exist"), false); err != nil {
		t.Errorf("missing dir should be no-op, got %v", err)
	}
}

func TestFrontmatter_Empty(t *testing.T) {
	if got := Frontmatter(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
	if got := Frontmatter(map[string]any{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestFrontmatter_WithFields(t *testing.T) {
	got := Frontmatter(map[string]any{"name": "foo"})
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("expected leading ---, got %q", got)
	}
	if !strings.Contains(got, "name: foo") {
		t.Errorf("expected 'name: foo' in %q", got)
	}
	if !strings.HasSuffix(got, "---\n") {
		t.Errorf("expected trailing ---, got %q", got)
	}
}

func TestFrontmatterOrdered_PreservesSourceKeyOrder(t *testing.T) {
	meta := map[string]any{
		"name":          "changelog-keeper",
		"model":         "haiku",
		"description":   "Keeps changelog tidy",
		"allowed_tools": []string{"Read", "Edit"},
	}
	keys := []string{"name", "model", "description", "allowed_tools"}
	got := FrontmatterOrdered(meta, keys)
	// name must appear before description and allowed_tools.
	idxName := strings.Index(got, "name:")
	idxDesc := strings.Index(got, "description:")
	idxTools := strings.Index(got, "allowed_tools:")
	if idxName < 0 || idxDesc < 0 || idxTools < 0 {
		t.Fatalf("missing keys in output: %q", got)
	}
	if idxName >= idxDesc || idxDesc >= idxTools {
		t.Errorf("expected name<description<allowed_tools order, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_TwoSpaceArrayIndent(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"allowed_tools": []string{"Read", "Edit"}},
		[]string{"allowed_tools"},
	)
	// yaml.v3 default is 4 spaces; the helper forces 2.
	if !strings.Contains(got, "\n  - Read\n  - Edit\n") {
		t.Errorf("expected 2-space sequence indent, got:\n%s", got)
	}
	if strings.Contains(got, "    - Read") {
		t.Errorf("expected no 4-space indent, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_PromotesSingleToDoubleQuotes(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"argument-hint": "[findings-file-or-scope]"},
		[]string{"argument-hint"},
	)
	if !strings.Contains(got, `"[findings-file-or-scope]"`) {
		t.Errorf("expected double-quoted scalar, got:\n%s", got)
	}
	if strings.Contains(got, `'[findings-file-or-scope]'`) {
		t.Errorf("expected no single-quoted scalar, got:\n%s", got)
	}
}

// TestFrontmatterStyled_PreservesSourceDoubleQuotes covers the source
// side of round-trip: when the value came from a file that wrapped
// `<feature>` in double quotes, the emitted scalar must stay double-
// quoted. The styles map carries that information from the spec
// loader.
func TestFrontmatterStyled_PreservesSourceDoubleQuotes(t *testing.T) {
	got := FrontmatterStyled(
		map[string]any{"argument-hint": "<feature-or-problem-statement>"},
		[]string{"argument-hint"},
		map[string]yaml.Style{"argument-hint": yaml.DoubleQuotedStyle},
	)
	if !strings.Contains(got, `"<feature-or-problem-statement>"`) {
		t.Errorf("expected DoubleQuotedStyle to flow through, got:\n%s", got)
	}
}

// TestFrontmatterStyled_PreservesPlainAngleBrackets is the inverse
// case: a plain `<ver>` source must stay plain instead of being
// auto-promoted to double-quoted. Regresses round-trip drift where
// `argument-hint: <ver>` got re-emitted as `argument-hint: "<ver>"`.
func TestFrontmatterStyled_PreservesPlainAngleBrackets(t *testing.T) {
	got := FrontmatterStyled(
		map[string]any{"argument-hint": "<ver>"},
		[]string{"argument-hint"},
		nil,
	)
	if !strings.Contains(got, "argument-hint: <ver>\n") {
		t.Errorf("expected plain <ver>, got:\n%s", got)
	}
	if strings.Contains(got, `"<ver>"`) {
		t.Errorf("plain scalar was force-quoted, got:\n%s", got)
	}
}

// TestFrontmatterOrdered_PreservesPlainScalar regresses #226. A short
// hand-authored plain scalar must round-trip unchanged through the
// emitter (no double quotes, no folding).
func TestFrontmatterOrdered_PreservesPlainScalar(t *testing.T) {
	got := FrontmatterOrdered(
		map[string]any{"description": "Review code changes for quality."},
		[]string{"description"},
	)
	if !strings.Contains(got, "description: Review code changes for quality.\n") {
		t.Errorf("expected plain scalar preserved, got:\n%s", got)
	}
	if strings.Contains(got, `description: "`) {
		t.Errorf("plain scalar was force-quoted, got:\n%s", got)
	}
}

// TestFrontmatterOrdered_PreservesLongPlainDescriptions regresses #226.
// Long plain scalars stay plain on round-trip: yaml.v3 v3.0.1 does not
// auto-wrap plain scalars, so promoting them to double-quoted would
// only add noise to byte-stable diffs after `import -> sync`.
func TestFrontmatterOrdered_PreservesLongPlainDescriptions(t *testing.T) {
	long := "Execute the implementation planning workflow using the plan template to generate design artifacts."
	got := FrontmatterOrdered(
		map[string]any{"description": long},
		[]string{"description"},
	)
	if strings.Contains(got, `"`+long+`"`) {
		t.Errorf("long plain description was force-quoted, want plain:\n%s", got)
	}
	if !strings.Contains(got, "description: "+long+"\n") {
		t.Errorf("expected plain `description: <long>` on one line, got:\n%s", got)
	}
}

func TestFrontmatterOrdered_UnhintedKeysAppendAlphabetically(t *testing.T) {
	meta := map[string]any{"zeta": 1, "alpha": 2, "name": "foo"}
	got := FrontmatterOrdered(meta, []string{"name"})
	idxName := strings.Index(got, "name:")
	idxAlpha := strings.Index(got, "alpha:")
	idxZeta := strings.Index(got, "zeta:")
	if idxName >= idxAlpha || idxAlpha >= idxZeta {
		t.Errorf("expected hinted-then-alpha order, got:\n%s", got)
	}
}

func TestStartCounting_CountsRealWrites(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	sess.StartCounting()
	if err := sess.WriteFile(filepath.Join(dir, "a.txt"), "aaa", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteFile(filepath.Join(dir, "b.txt"), "bbb", false); err != nil {
		t.Fatal(err)
	}
	n := sess.StopCounting()
	if n != 2 {
		t.Fatalf("want 2, got %d", n)
	}
}

func TestStartCounting_IgnoresDryRun(t *testing.T) {
	sess := NewSession()
	dir := t.TempDir()
	sess.StartCounting()
	_ = sess.WriteFile(filepath.Join(dir, "x.txt"), "xxx", true)
	n := sess.StopCounting()
	if n != 0 {
		t.Fatalf("want 0 for dry-run, got %d", n)
	}
}

func TestStartCounting_IgnoresCaptureMode(t *testing.T) {
	sess := NewSession()
	sess.StartCapture()
	sess.StartCounting()
	_ = sess.WriteFile("/nonexistent/fake.txt", "zzz", false)
	_ = sess.StopCapture()
	n := sess.StopCounting()
	if n != 0 {
		t.Fatalf("want 0 in capture mode, got %d", n)
	}
}

func TestStopCounting_WithoutStart_ReturnsZero(t *testing.T) {
	sess := NewSession()
	if n := sess.StopCounting(); n != 0 {
		t.Fatalf("want 0 when not counting, got %d", n)
	}
}

// The prune race surfaces as two different errnos, and only ENOENT was
// handled: sync failed intermittently with `invalid argument` instead
// (#701). EACCES is in the table because widening the guard must not
// start swallowing a permission failure as a missing parent.
func TestParentGone_CoversBothPruneErrnosAndNothingElse(t *testing.T) {
	pathErr := func(e error) error {
		return &os.PathError{Op: "open", Path: "agents/agents/reviewer.md", Err: e}
	}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"enoent", pathErr(syscall.ENOENT), true},
		{"einval", pathErr(syscall.EINVAL), true},
		{"eacces", pathErr(syscall.EACCES), false},
		{"unrelated", errors.New("boom"), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parentGone(c.err); got != c.want {
				t.Errorf("parentGone(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestRemoveGeneratedTree_WritesOnlyCaptureSkipsTheSweep(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "legacy")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.toml"), []byte("# Generated by agnostic-ai\nkey = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	full := NewSession()
	full.StartCapture()
	if err := full.RemoveGeneratedTree(tree, false); err != nil {
		t.Fatalf("RemoveGeneratedTree: %v", err)
	}
	if got := len(full.CapturedRemovals()); got != 1 {
		t.Fatalf("capture recorded %d removals, want 1", got)
	}

	writesOnly := NewSession()
	writesOnly.StartWritesOnlyCapture()
	if err := writesOnly.RemoveGeneratedTree(tree, false); err != nil {
		t.Fatalf("RemoveGeneratedTree: %v", err)
	}
	if got := writesOnly.CapturedRemovals(); len(got) != 0 {
		t.Errorf("writes-only capture recorded removals %v, want none", got)
	}
	if err := writesOnly.WriteFile(filepath.Join(dir, "out.md"), "body", false); err != nil {
		t.Fatal(err)
	}
	if got := writesOnly.StopCapture(); len(got) != 1 || got[0].Path != filepath.Join(dir, "out.md") {
		t.Errorf("writes-only capture kept %v, want the one write", got)
	}
	if _, err := os.Stat(filepath.Join(tree, "a.toml")); err != nil {
		t.Errorf("capture touched disk: %v", err)
	}
}

func TestStartCapture_ResetsAWritesOnlyCapture(t *testing.T) {
	tree := filepath.Join(t.TempDir(), "legacy")
	if err := os.MkdirAll(tree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "a.toml"), []byte("# Generated by agnostic-ai\nkey = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartWritesOnlyCapture()
	sess.StartCapture()
	if err := sess.RemoveGeneratedTree(tree, false); err != nil {
		t.Fatalf("RemoveGeneratedTree: %v", err)
	}
	if got := len(sess.CapturedRemovals()); got != 1 {
		t.Errorf("StartCapture after a writes-only capture recorded %d removals, want 1", got)
	}
}

func TestCapture_ReportsOnlyEnforcedModes(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	sess.StartCapture()
	path := filepath.Join(t.TempDir(), "ordinary")
	if err := sess.WriteFile(path, "ordinary", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteExecutableFile(path+"-script", "#!/bin/sh", false); err != nil {
		t.Fatal(err)
	}
	files := sess.StopCapture()
	if len(files) != 2 {
		t.Fatalf("captured %d files", len(files))
	}
	if files[0].Mode != 0 || files[1].Mode != 0o755 {
		t.Errorf("captured modes = %v, %v", files[0].Mode, files[1].Mode)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("capture wrote a file: %v", err)
	}
}

func TestSession_RefusesTheGitFolderAndAbsolutePathsOutsideTheProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.md")
	testutilChdir(t, root)
	sess := NewSession()
	sess.SetProjectRoot(".")
	trailingSpace := filepath.Join(".claude", "skills", ".GIT ", "HEAD")
	paths := []string{
		filepath.Join(".git", "hooks", "post-checkout"),
		trailingSpace,
		outside,
		filepath.Join(root, ".git", "config"),
	}

	for _, path := range paths {
		// Windows cannot create a folder whose name ends in a space, so
		// there only the name check is asserted, by its message, since
		// the failed create would also return an error.
		if path == trailingSpace && runtime.GOOS == "windows" {
			if err := sess.WriteFile(path, "y\n", false); err == nil || !strings.Contains(err.Error(), "inside .git") {
				t.Errorf("WriteFile(%s) = %v, want the .git refusal", path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := sess.WriteFile(path, "y\n", false); err == nil {
			t.Errorf("WriteFile(%s) succeeded, want a refusal", path)
		}
		if removed, _ := sess.RemoveOwned(path, ContentSum("x\n"), false); removed {
			t.Errorf("RemoveOwned(%s) removed it, want a refusal", path)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "x\n" {
			t.Errorf("%s changed: %q, %v", path, got, err)
		}
	}
	if err := sess.WriteFile(filepath.Join(root, "inside.md"), "x\n", false); err != nil {
		t.Errorf("absolute path inside the project refused: %v", err)
	}
}
