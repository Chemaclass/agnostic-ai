package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSupportsFileImports(t *testing.T) {
	if !SupportsFileImports("claude") {
		t.Error("claude resolves @-imports and should report support")
	}
	for _, tgt := range []string{"codex", "amp", "warp", "gemini", "aider", "opencode"} {
		if SupportsFileImports(tgt) {
			t.Errorf("%s does not resolve @-imports and must not report support", tgt)
		}
	}
}

func TestApplyImportMode_PassthroughLeavesBodyUnchanged(t *testing.T) {
	body := "# Project\n\n@docs/architecture.md\n@docs/errors.md\n"
	for _, mode := range []string{ImportModePassthrough, "", "bogus"} {
		got, err := ApplyImportMode(body, mode)
		if err != nil {
			t.Fatalf("mode %q: unexpected error: %v", mode, err)
		}
		if got != body {
			t.Errorf("mode %q changed body:\n%s", mode, got)
		}
	}
}

func TestApplyImportMode_StripDropsImportLines(t *testing.T) {
	body := "# Project\n\nGuidance.\n\n@docs/architecture.md\n@docs/errors.md\n"
	got, err := ApplyImportMode(body, ImportModeStrip)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "@docs/") {
		t.Errorf("strip left an import line:\n%s", got)
	}
	if !strings.Contains(got, "Guidance.") {
		t.Errorf("strip dropped prose:\n%s", got)
	}
}

func TestApplyImportMode_StripKeepsEmailLikeProse(t *testing.T) {
	// A lone @token is an import; an @mention inside a sentence is not.
	body := "Ping @alice for review.\n@docs/architecture.md\n"
	got, err := ApplyImportMode(body, ImportModeStrip)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Ping @alice for review.") {
		t.Errorf("strip removed prose @mention:\n%s", got)
	}
	if strings.Contains(got, "@docs/architecture.md") {
		t.Errorf("strip left the import line:\n%s", got)
	}
}

func TestApplyImportMode_InlineEmbedsFileContent(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "docs", "architecture.md"), "# Architecture\n\nLayered.\n")

	body := "# Project\n\n@docs/architecture.md\n"
	got, err := ApplyImportMode(body, ImportModeInline)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Layered.", "agnostic-ai:import:start docs/architecture.md", "agnostic-ai:import:end"} {
		if !strings.Contains(got, want) {
			t.Errorf("inline missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@docs/architecture.md\n") {
		t.Errorf("inline left the raw import line:\n%s", got)
	}
}

func TestApplyImportMode_InlineErrorsOnMissingFile(t *testing.T) {
	testutil.TempCwd(t)
	_, err := ApplyImportMode("@docs/missing.md\n", ImportModeInline)
	if err == nil {
		t.Fatal("want error for missing import target")
	}
	if !strings.Contains(err.Error(), "docs/missing.md") {
		t.Errorf("error should name the path, got: %v", err)
	}
}

func TestRestoreImportInlines_RoundTripsInlineMode(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "docs", "architecture.md"), "# Architecture\n\nLayered.\n")

	body := "# Project\n\n@docs/architecture.md\n\nMore.\n"
	inlined, err := ApplyImportMode(body, ImportModeInline)
	if err != nil {
		t.Fatal(err)
	}
	if got := RestoreImportInlines(inlined); got != body {
		t.Errorf("round-trip mismatch.\nwant %q\ngot  %q", body, got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
