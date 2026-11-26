package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryLint_WarnsWhenTheBuiltInHasNoProjectIndex(t *testing.T) {
	setupMemoryProject(t, "claude")

	out, err := runRoot(t, "memory", "lint")

	want := "LINT067 [warn] " + filepath.Join(".agnostic-ai", "memory", "MEMORY.md")
	if err != nil || !strings.Contains(out, want) || !strings.Contains(out, "agnostic-ai memory index") {
		t.Errorf("memory lint must warn, without failing, and name the fix (err %v):\n%s", err, out)
	}
	if lintOut, _ := runRoot(t, "lint"); !strings.Contains(lintOut, want) {
		t.Errorf("lint must report the same finding:\n%s", lintOut)
	}
}

func TestMemoryLint_NoMissingIndexWarningWithoutTheBuiltIn(t *testing.T) {
	setupMemoryConfig(t, "version: 1\ntargets: [claude]\n")

	if out, _ := runRoot(t, "memory", "lint"); strings.Contains(out, "LINT067") {
		t.Errorf("memory off must not ask for an index:\n%s", out)
	}
}

func TestMemoryIndex_CreatesTheMissingProjectIndex(t *testing.T) {
	dir := setupMemoryProject(t, "claude")

	out, err := runRoot(t, "memory", "index")

	if err != nil || !strings.Contains(out, "MEMORY.md: created, 0 fact(s)") {
		t.Fatalf("memory index must create the project index (err %v):\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md")); err != nil {
		t.Errorf("no index on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "local", "memory", "MEMORY.md")); !os.IsNotExist(err) {
		t.Errorf("memory index must not create personal memory: %v", err)
	}
	if out, _ := runRoot(t, "memory", "lint"); strings.Contains(out, "LINT067") {
		t.Errorf("the warning must clear once the index exists:\n%s", out)
	}
}

func TestMemoryIndex_CreatedIndexListsExistingFacts(t *testing.T) {
	dir := setupMemoryProject(t, "claude")
	writeMemory(t, dir, ".agnostic-ai/memory", map[string]string{
		"ci.md": memoryFact("ci", "project", "CI runs on Ubuntu"),
	})

	if out, err := runRoot(t, "memory", "index"); err != nil || !strings.Contains(out, "created, 1 fact(s)") {
		t.Fatalf("memory index (err %v):\n%s", err, out)
	}

	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md")); !strings.Contains(got, "(ci.md)") {
		t.Errorf("the created index must link the fact:\n%s", got)
	}
}
