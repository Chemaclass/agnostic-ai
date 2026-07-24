package qoder

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestEmit_ProvenanceHeaderOnEveryEmittedFile is the qoder adapter's
// header-coverage contract: every Markdown file the adapter writes
// must carry the agnostic-ai provenance marker. Qoder emits no JSON so
// there are no header exemptions.
func TestEmit_ProvenanceHeaderOnEveryEmittedFile(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	var checked int
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, ".agnostic-ai/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !header.Has(string(data)) {
			t.Errorf("missing provenance header in %s:\n%s", rel, headFor(t, data))
			return nil
		}
		checked++
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if checked == 0 {
		t.Fatalf("no header-bearing files inspected; kit-sink bundle likely emitted nothing")
	}
}

// kitSinkBundle returns a Bundle exercising every kind the qoder
// adapter declares in caps.Supports (Rule only) with three specimens.
func kitSinkBundle() spec.Bundle {
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule 1 body"},
		{Kind: spec.KindRule, Name: "r2", Path: "rules/r2.md", Body: "rule 2 body"},
		{Kind: spec.KindRule, Name: "r3", Path: "rules/r3.md", Body: "rule 3 body"},
	}
	return spec.NewBundle(entries)
}

func headFor(t *testing.T, data []byte) string {
	t.Helper()
	if i := strings.IndexByte(string(data), '\n'); i >= 0 && i < 120 {
		return string(data[:i])
	}
	if len(data) > 120 {
		return string(data[:120]) + "..."
	}
	return string(data)
}
