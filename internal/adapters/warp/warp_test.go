package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WritesWarpMd(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	a := New()
	if a.Name() != "warp" {
		t.Errorf("expected warp, got %s", a.Name())
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	}
	if err := a.Emit(spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "WARP.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "rule body") {
		t.Errorf("missing rule body in %s", got)
	}
	if !strings.Contains(string(got), "# WARP.md") {
		t.Errorf("missing title in %s", got)
	}
}
