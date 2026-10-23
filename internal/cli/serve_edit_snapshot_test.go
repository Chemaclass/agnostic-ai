package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEditSnapshot_WrittenIdentitySurvivesAnotherRename(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	file := filepath.Join(dir, "config.yaml")
	data := []byte("# chosen bytes\ntargets: [claude]\n")
	written, err := (editFileSnapshot{}).replace(file, data)
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "renamed.yaml")
	if err := os.Rename(file, moved); err != nil {
		t.Fatal(err)
	}
	if err := written.check(moved); err != nil {
		t.Fatalf("own file identity depended on an obsolete pathname: %v", err)
	}
	got, err := os.ReadFile(moved)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("written bytes = %q", got)
	}
}

func TestEditSnapshot_RejectsIdenticalExternalReplacementAfterOwnWrite(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	file := filepath.Join(dir, "config.yaml")
	data := []byte("targets: [claude]\n")
	written, err := (editFileSnapshot{}).replace(file, data)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(dir, "editor.yaml")
	if err := os.WriteFile(external, data, written.info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(external, written.info.ModTime(), written.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(external, file); err != nil {
		t.Fatal(err)
	}
	if err := written.check(file); err == nil || !strings.Contains(err.Error(), "changed since") {
		t.Errorf("external replacement was adopted: %v", err)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("external bytes changed: %q", got)
	}
}
