package emit

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func assertUnmanagedSkips(t *testing.T, sess *Session, want ...string) {
	t.Helper()
	if got := sess.UnmanagedSkips(); !reflect.DeepEqual(got, want) {
		t.Errorf("UnmanagedSkips = %q, want %q", got, want)
	}
}

func TestWriteFile_SkipsUnmanagedPath(t *testing.T) {
	testutil.TempCwd(t)
	const path = ".cursor/rules/legacy.mdc"
	mustWrite(t, path, "mine\n")
	sess := NewSession()
	sess.SetUnmanaged([]string{path})

	if err := sess.WriteFile(path, "generated\n", false); err != nil {
		t.Fatal(err)
	}

	if got := readFileString(t, path); got != "mine\n" {
		t.Errorf("user-owned file rewritten: %q", got)
	}
	assertUnmanagedSkips(t, sess, path)
}

func TestWriteFile_UnmanagedIsNotCapturedRecordedOrDetailed(t *testing.T) {
	testutil.TempCwd(t)
	const owned, sibling = ".claude/agents/hand.md", ".claude/agents/gen.md"

	capture := NewSession()
	capture.SetUnmanaged([]string{owned})
	capture.StartCapture()
	for _, p := range []string{owned, sibling} {
		if err := capture.WriteFile(p, "x\n", false); err != nil {
			t.Fatal(err)
		}
	}
	captured := capture.StopCapture()
	if len(captured) != 1 || captured[0].Path != sibling {
		t.Errorf("captured = %+v, want only %s", captured, sibling)
	}

	writer := NewSession()
	writer.SetUnmanaged([]string{owned})
	writer.StartRecording()
	writer.StartDetailedRecording()
	for _, p := range []string{owned, sibling} {
		if err := writer.WriteFile(p, "x\n", false); err != nil {
			t.Fatal(err)
		}
	}
	if got := writer.StopRecording(); !reflect.DeepEqual(got, []string{sibling}) {
		t.Errorf("recorded = %q, want only %s", got, sibling)
	}
	detailed := writer.StopDetailedRecording()
	if len(detailed) != 1 || detailed[0].Path != sibling {
		t.Errorf("detailed = %+v, want only %s", detailed, sibling)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Errorf("user-owned path must not be created, err=%v", err)
	}
}

func TestWriteFile_UnmanagedGlobAndDirPrefix(t *testing.T) {
	testutil.TempCwd(t)
	sess := NewSession()
	sess.SetUnmanaged([]string{".claude/agents/hand-*.md", ".claude/skills/legacy/"})

	for _, p := range []string{".claude/agents/hand-x.md", ".claude/skills/legacy/SKILL.md", ".claude/agents/other.md"} {
		if err := sess.WriteFile(p, "x\n", false); err != nil {
			t.Fatal(err)
		}
	}

	for _, p := range []string{".claude/agents/hand-x.md", ".claude/skills/legacy/SKILL.md"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s must not be written, err=%v", p, err)
		}
	}
	if got := readFileString(t, ".claude/agents/other.md"); got != "x\n" {
		t.Errorf("managed sibling not written: %q", got)
	}
}

func TestWriteFile_DryRunSkipsUnmanagedAndRecordsIt(t *testing.T) {
	testutil.TempCwd(t)
	sess := NewSession()
	sess.SetUnmanaged([]string{"AGENTS.md"})

	if err := sess.WriteFile("AGENTS.md", "x\n", true); err != nil {
		t.Fatal(err)
	}

	assertUnmanagedSkips(t, sess, "AGENTS.md")
}

func TestMergeJSONFile_SkipsUnmanaged(t *testing.T) {
	testutil.TempCwd(t)
	const path, mine = "opencode.json", "{\"mine\": true}\n"
	mustWrite(t, path, mine)
	sess := NewSession()
	sess.SetUnmanaged([]string{path})

	if err := sess.MergeJSONFile(path, map[string]any{"mcp": map[string]any{}}, false); err != nil {
		t.Fatal(err)
	}

	if got := readFileString(t, path); got != mine {
		t.Errorf("user-owned JSON merged: %q", got)
	}
}

func TestCopyTree_SkipsUnmanagedFileCopiesSiblings(t *testing.T) {
	testutil.TempCwd(t)
	mustWrite(t, "src/a.md", "a\n")
	mustWrite(t, "src/b.md", "b\n")
	mustWrite(t, "dst/b.md", "mine\n")
	sess := NewSession()
	sess.SetUnmanaged([]string{"dst/b.md"})

	if err := sess.CopyTree("src", "dst", nil, false); err != nil {
		t.Fatal(err)
	}

	if got := readFileString(t, "dst/a.md"); got != "a\n" {
		t.Errorf("sibling not copied: %q", got)
	}
	if got := readFileString(t, "dst/b.md"); got != "mine\n" {
		t.Errorf("user-owned file overwritten: %q", got)
	}
}

func TestRemoveGenerated_KeepsUnmanagedGeneratedFile(t *testing.T) {
	testutil.TempCwd(t)
	const path = "AGENT.md"
	body := header.Marker + "\nold\n"
	mustWrite(t, path, body)
	sess := NewSession()
	sess.SetUnmanaged([]string{path})

	if err := sess.RemoveGenerated(path, false); err != nil {
		t.Fatal(err)
	}

	if got := readFileString(t, path); got != body {
		t.Errorf("user-owned file removed or changed: %q", got)
	}
	assertUnmanagedSkips(t, sess, path)
}

func TestRemoveGenerated_UnmanagedMissingFileIsNotReported(t *testing.T) {
	testutil.TempCwd(t)
	sess := NewSession()
	sess.SetUnmanaged([]string{"AGENT.md"})

	if err := sess.RemoveGenerated("AGENT.md", false); err != nil {
		t.Fatal(err)
	}

	assertUnmanagedSkips(t, sess)
}

func TestRemoveGeneratedTree_KeepsUnmanagedFileAndItsDir(t *testing.T) {
	testutil.TempCwd(t)
	body := header.Marker + "\n"
	mustWrite(t, "legacy/hand.md", body)
	mustWrite(t, "legacy/gen.md", body)
	sess := NewSession()
	sess.SetUnmanaged([]string{"legacy/hand.md"})

	if err := sess.RemoveGeneratedTree("legacy", false); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat("legacy/gen.md"); !os.IsNotExist(err) {
		t.Errorf("generated sibling should be removed, err=%v", err)
	}
	if got := readFileString(t, "legacy/hand.md"); got != body {
		t.Errorf("user-owned file removed: %q", got)
	}
}

func TestWriteIgnoreFile_SkipsUnmanagedWithoutRefusal(t *testing.T) {
	testutil.TempCwd(t)
	const path, mine = ".kiroignore", "# hand-authored\nmy-secrets/\n"
	mustWrite(t, path, mine)
	sess := NewSession()
	sess.SetUnmanaged([]string{path})

	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "dist/"}}, "kiro", path, false); err != nil {
		t.Fatalf("user-owned ignore file must be skipped, not refused: %v", err)
	}

	if got := readFileString(t, path); got != mine {
		t.Errorf("user-owned ignore file changed: %q", got)
	}
	assertUnmanagedSkips(t, sess, path)
}

func TestMigrateLegacyFile_KeepsUnmanagedLegacyFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	body := header.Marker + "\nstale\n"
	mustWrite(t, filepath.Join(dir, "LEGACY.md"), body)
	sess := NewSession()
	sess.SetUnmanaged([]string{"LEGACY.md"})

	sess.MigrateLegacyFile(&config.Config{}, "amp", "LEGACY.md", "NEW.md", false)

	if got := readFileString(t, "LEGACY.md"); got != body {
		t.Errorf("user-owned legacy file renamed or changed: %q", got)
	}
	if _, err := os.Stat("LEGACY.md.bak"); !os.IsNotExist(err) {
		t.Errorf("no backup expected, err=%v", err)
	}
	assertUnmanagedSkips(t, sess, "LEGACY.md")
}

func TestUnmanagedSkips_EmptyWhenNothingIsOwned(t *testing.T) {
	testutil.TempCwd(t)
	sess := NewSession()
	sess.SetUnmanaged(nil)

	if err := sess.WriteFile("AGENTS.md", "x\n", false); err != nil {
		t.Fatal(err)
	}

	assertUnmanagedSkips(t, sess)
	if got := readFileString(t, "AGENTS.md"); got != "x\n" {
		t.Errorf("managed file not written: %q", got)
	}
}
