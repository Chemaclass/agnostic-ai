package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_Check_PassesWhenInSync(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check"})
	if err := root.Execute(); err != nil {
		t.Errorf("check should pass after sync, got: %v", err)
	}
}

func TestDoctor_DetectsMissing(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	err := root.Execute()
	if err == nil {
		t.Error("doctor should fail when no sync has been run")
	}
}

func TestDoctor_Fix_ReconcilesMissingAndStale(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claudeMd := filepath.Join(dir, ".claude/rules/r1.md")
	original, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claudeMd, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --fix should succeed, got: %v", err)
	}

	got, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Errorf("doctor --fix did not restore CLAUDE.md")
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Errorf("doctor should be clean after --fix, got: %v", err)
	}
}

func TestDoctor_Fix_BackupPreservesHandEdits(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	claudeMd := filepath.Join(dir, ".claude/rules/r1.md")
	handEdit := []byte("hand-edited contents\n")
	if err := os.WriteFile(claudeMd, handEdit, 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	bak, err := os.ReadFile(claudeMd + ".bak")
	if err != nil {
		t.Fatalf("expected .bak file, got: %v", err)
	}
	if string(bak) != string(handEdit) {
		t.Errorf("backup mismatch: got %q, want %q", bak, handEdit)
	}
}

// TestDoctor_NoFalseDriftAfterSyncWithOverlay regresses #215. After a
// clean sync that materializes statusLine and enabledPlugins from the
// settings overlay, doctor must read the overlay too (capture mode is
// not allowed to skip source inputs) so its captured bytes match the
// disk bytes. Otherwise it reports false drift and `--fix` deletes the
// overlay-supplied keys.
func TestDoctor_NoFalseDriftAfterSyncWithOverlay(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	overlayPath := filepath.Join(dir, ".agnostic-ai/overlays/claude.settings.json")
	if err := os.MkdirAll(filepath.Dir(overlayPath), 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := `{
  "enabledPlugins": {"plugin-a": true},
  "statusLine": {"type": "command", "command": "echo status"}
}
`
	if err := os.WriteFile(overlayPath, []byte(overlay), 0o644); err != nil {
		t.Fatal(err)
	}
	hooksDir := filepath.Join(dir, ".agnostic-ai", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "h1.yaml"),
		[]byte("name: h1\nevent: PostToolUse\nmatcher: Edit\ncommand: echo hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync: %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Errorf("doctor reports false drift immediately after sync: %v", err)
	}

	settings, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"enabledPlugins"`, `"statusLine"`, `"hooks"`} {
		if !strings.Contains(string(settings), key) {
			t.Errorf("post-sync settings.json missing %s:\n%s", key, settings)
		}
	}

	// `doctor --fix` must be a no-op here. Run it; settings.json bytes
	// must be unchanged so we don't lose overlay-supplied keys.
	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--fix"})
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --fix: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(settings) != string(after) {
		t.Errorf("doctor --fix mutated settings.json:\nBEFORE:\n%s\nAFTER:\n%s", settings, after)
	}
}

// TestDoctor_Fix_PreservesUserKeysInMergedJSON regresses #465. opencode
// shares opencode.json with the user, who keeps sibling keys next to the
// managed `mcp` block. doctor runs the adapter in capture mode; the JSON
// merge reader must read the existing file so captured bytes match disk.
// Otherwise doctor reports false drift and --fix overwrites the file with
// the managed keys only, deleting the user's siblings.
func TestDoctor_Fix_PreservesUserKeysInMergedJSON(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets:\n  - opencode\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "mcps"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"),
		[]byte("name: fs\ncommand: fs-server\n"), 0o644))
	// User keeps a sibling key in the shared opencode.json.
	must(os.WriteFile(filepath.Join(dir, "opencode.json"),
		[]byte(`{"theme": "dark"}`), 0o644))

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "opencode"})
	must(root.Execute())

	// sync merges, so the synced file carries both the user key and mcp.
	synced := readFile(t, filepath.Join(dir, "opencode.json"))
	for _, want := range []string{`"theme"`, `"mcp"`} {
		if !strings.Contains(synced, want) {
			t.Fatalf("post-sync opencode.json missing %s:\n%s", want, synced)
		}
	}

	// doctor must report no drift right after a clean sync.
	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "opencode"})
	if err := root.Execute(); err != nil {
		t.Errorf("doctor reports false drift after clean sync: %v", err)
	}

	// doctor --fix must not delete the user's sibling key.
	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "opencode", "--fix"})
	must(root.Execute())
	after := readFile(t, filepath.Join(dir, "opencode.json"))
	if !strings.Contains(after, `"theme"`) {
		t.Errorf("doctor --fix deleted user key:\n%s", after)
	}
	if !strings.Contains(after, `"mcp"`) {
		t.Errorf("doctor --fix dropped managed mcp:\n%s", after)
	}
}

func TestDoctor_DetectsStale(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, ".claude/rules/r1.md"),
		[]byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude"})
	if err := root.Execute(); err == nil {
		t.Error("doctor should detect stale claude rule")
	}
}
