package kilo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_RepoMemoryIndexOutsideRootIsNotWritten(t *testing.T) {
	testutil.TempCwd(t)
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	notes := swapNoteWarner(t)
	cfg := &config.Config{
		Builtins:  []string{emit.MemoryBuiltin},
		Memory:    config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Gitignore: config.Gitignore{Enabled: true},
	}
	if err := os.WriteFile("kilo.jsonc", []byte(`{"instructions":["own.md"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, "kilo.jsonc")
	if strings.Contains(got, home) || strings.Contains(got, filepath.ToSlash(home)) {
		t.Errorf("absolute store index written to kilo.jsonc: %s", got)
	}
	if !strings.Contains(got, emit.ProjectMemoryIndexPath) || !strings.Contains(got, "own.md") {
		t.Errorf("project index or user entry missing: %s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "kilo:") || !strings.Contains(notes.String(), "outside the project root") {
		t.Errorf("missing coverage note: %q", notes.String())
	}
}

func TestEmit_CheckoutMemoryIndexStaysInKiloJSONC(t *testing.T) {
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	cfg := &config.Config{Builtins: []string{emit.MemoryBuiltin}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, "kilo.jsonc"); !strings.Contains(got, emit.PersonalMemoryIndexPath) {
		t.Errorf("checkout personal index missing: %s", got)
	}
	emit.FlushCoverageNotes()
	if notes.Len() != 0 {
		t.Errorf("unexpected note: %q", notes.String())
	}
}
