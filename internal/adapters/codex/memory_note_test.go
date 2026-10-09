package codex

import (
	"bytes"
	"fmt"
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

func TestEmit_RepoMemoryNamesMissingWritableRootInPreservedConfig(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	cfg := &config.Config{Builtins: []string{emit.MemoryBuiltin}, Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}
	dir, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	original := "model = \"o3\"\n"
	if err := os.MkdirAll(".codex", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultConfigFile, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	notes := memoryNoteWarner(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{defaultConfigFile, "sandbox_workspace_write.writable_roots", filepath.ToSlash(dir)} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("missing %q in note: %s", want, notes.String())
		}
	}
	if got := readFile(t, defaultConfigFile); got != original {
		t.Errorf("user config changed: %q", got)
	}
}

func TestEmit_RepoMemoryNamesMissingWritableRootInUnmanagedConfig(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	cfg := &config.Config{
		Builtins: []string{emit.MemoryBuiltin},
		Memory:   config.MemoryConfig{Personal: config.PersonalMemoryRepo},
		Outputs:  map[string]config.Output{"codex": {Config: &config.CodexConfig{Model: "o3"}}},
	}
	dir, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	const original = "model = \"o3\"\n"
	if err := os.MkdirAll(".codex", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultConfigFile, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	notes := memoryNoteWarner(t)
	sess := emit.NewSession()
	sess.SetUnmanaged([]string{defaultConfigFile})
	if err := New().Emit(sess, spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, want := range []string{defaultConfigFile, "sandbox_workspace_write.writable_roots", filepath.ToSlash(dir)} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("missing %q in note: %s", want, notes.String())
		}
	}
	if got := readFile(t, defaultConfigFile); got != original {
		t.Errorf("user config changed: %q", got)
	}
}

func TestEmit_RepoMemoryAlreadyWritableNeedsNoNote(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	cfg := &config.Config{Builtins: []string{emit.MemoryBuiltin}, Memory: config.MemoryConfig{Personal: config.PersonalMemoryRepo}}
	dir, err := emit.PersonalMemoryDir(cfg, ".")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".codex", 0755); err != nil {
		t.Fatal(err)
	}
	original := fmt.Sprintf("[sandbox_workspace_write]\nwritable_roots = [%q]\n", filepath.ToSlash(dir))
	if err := os.WriteFile(defaultConfigFile, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	notes := memoryNoteWarner(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if notes.Len() != 0 {
		t.Errorf("unexpected note: %s", notes.String())
	}
	if got := readFile(t, defaultConfigFile); got != original {
		t.Errorf("user config changed: %q", got)
	}
}

func TestEmit_PreservedMemoryRootNoteUsesConfigAndPreview(t *testing.T) {
	for _, tc := range []struct {
		name     string
		personal string
		builtins []string
		dryRun   bool
		wantNote bool
	}{
		{"checkout memory", "", []string{emit.MemoryBuiltin}, false, false},
		{"memory disabled", config.PersonalMemoryRepo, nil, false, false},
		{"repo memory preview", config.PersonalMemoryRepo, []string{emit.MemoryBuiltin}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git init: %v\n%s", err, out)
			}
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			const path = "custom/config.toml"
			cfg := &config.Config{Builtins: tc.builtins, Memory: config.MemoryConfig{Personal: tc.personal}, Outputs: map[string]config.Output{"codex": {MCPFile: path}}}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			const original = "model = \"o3\"\n"
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			notes := memoryNoteWarner(t)
			if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), cfg, tc.dryRun); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			if got := strings.Contains(notes.String(), path); got != tc.wantNote {
				t.Errorf("note = %q, want note: %v", notes.String(), tc.wantNote)
			}
			if got := readFile(t, path); got != original {
				t.Errorf("user config changed: %q", got)
			}
		})
	}
}

func memoryNoteWarner(t *testing.T) *bytes.Buffer {
	t.Helper()
	var notes bytes.Buffer
	prev := emit.Warner
	emit.Warner = &notes
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	return &notes
}
