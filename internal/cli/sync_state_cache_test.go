package cli

import (
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHoldStateFile_ServesOneCopyUntilReleased(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)

	release := holdStateFile(".")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Errorf("while held: sum %q, want the held one", got)
	}
	release()
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("after release: sum %q, want two from disk", got)
	}
}

func TestHoldPriorState_ServesTheGivenCopy(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"disk"}}`)

	release := holdPriorState(".", syncStateFile{OutputSums: map[string]string{"a.md": "held"}})
	if got := priorStateFile().OutputSums["a.md"]; got != "held" {
		t.Errorf("while held: sum %q, want held", got)
	}
	release()
	if got := priorStateFile().OutputSums["a.md"]; got != "disk" {
		t.Errorf("after release: sum %q, want disk", got)
	}
}

func TestHoldPriorState_OtherRootHoldsNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"disk"}}`)

	release := holdPriorState(t.TempDir(), syncStateFile{OutputSums: map[string]string{"a.md": "other"}})
	defer release()
	if got := priorStateFile().OutputSums["a.md"]; got != "disk" {
		t.Errorf("sum %q, want disk: the hooks read the working directory", got)
	}
}

func TestRunSyncOnce_ReleasesTheHeldState(t *testing.T) {
	setupUnmanagedFixture(t)
	silence(t)
	captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	mustWriteFile(t, filepath.Join(".agnostic-ai", ".sync-state"), `{"output_sums":{"a.md":"after"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "after" {
		t.Errorf("sum %q, want after: sync must not hold the state once it returns", got)
	}
}

func TestPriorStateFile_ReadsTheDiskWhenNothingIsHeld(t *testing.T) {
	dir := testutil.TempCwd(t)
	path := filepath.Join(dir, ".agnostic-ai", ".sync-state")
	mustWriteFile(t, path, `{"output_sums":{"a.md":"one"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "one" {
		t.Fatalf("first read: sum %q, want one", got)
	}
	mustWriteFile(t, path, `{"output_sums":{"a.md":"two"}}`)
	if got := priorStateFile().OutputSums["a.md"]; got != "two" {
		t.Errorf("same-size rewrite: sum %q, want two", got)
	}
}
