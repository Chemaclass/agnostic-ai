package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A hand-authored entry-point file (no agnostic-ai provenance marker)
// must trigger a one-line warning before sync overwrites it so the
// user knows their content is about to be replaced.
func TestWriteAgnosticEntryPoints_WarnsBeforeOverwritingHandAuthored(t *testing.T) {
	dir := testutil.TempCwd(t)

	// Pre-existing root CLAUDE.md without provenance header.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# My Hand-Written CLAUDE.md\n\nKeep me.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAgnosticFile(t, "# Pointer body\n")

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	cfg := &config.Config{Targets: []string{"claude"}}
	if err := writeAgnosticEntryPoints(cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "CLAUDE.md appears hand-authored") {
		t.Errorf("expected hand-authored warning, got:\n%s", buf.String())
	}
}

// A subsequent sync with a generated header on disk must NOT trigger
// the warning — the file is already managed.
func TestWriteAgnosticEntryPoints_NoWarnWhenGeneratedHeaderPresent(t *testing.T) {
	dir := testutil.TempCwd(t)
	managed := header.With("# Pointer body\n", header.FormatMarkdown)
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(managed), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAgnosticFile(t, "# Pointer body\n")

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	cfg := &config.Config{Targets: []string{"claude"}}
	if err := writeAgnosticEntryPoints(cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "appears hand-authored") {
		t.Errorf("expected no warning on already-managed file, got:\n%s", buf.String())
	}
}

func TestResolveAgnosticBody_SeedsTemplateWhenAbsent(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Sources: config.Sources{Rules: ".agnostic-ai/rules"}}

	body, err := resolveAgnosticBody(cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body == "" {
		t.Fatal("body is empty")
	}
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		t.Fatalf("AGNOSTIC_AI.md not created: %v", err)
	}
	if !header.Has(string(data)) {
		t.Errorf("AGNOSTIC_AI.md missing provenance header")
	}
	if !strings.Contains(string(data), body) {
		t.Errorf("AGNOSTIC_AI.md body mismatch")
	}
}

func TestResolveAgnosticBody_UsesExistingContent(t *testing.T) {
	testutil.TempCwd(t)
	custom := "# My Project\n\nCustom instructions here.\n"
	writeAgnosticFile(t, custom)

	body, err := resolveAgnosticBody(&config.Config{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != custom {
		t.Errorf("body = %q, want %q", body, custom)
	}
}

func TestResolveAgnosticBody_StripsHeaderFromExisting(t *testing.T) {
	testutil.TempCwd(t)
	rawBody := "# My Project\n\nInstructions.\n"
	writeAgnosticFile(t, header.With(rawBody, header.FormatMarkdown))

	body, err := resolveAgnosticBody(&config.Config{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if header.Has(body) {
		t.Errorf("returned body still contains header: %q", body)
	}
	if body != rawBody {
		t.Errorf("body = %q, want %q", body, rawBody)
	}
}

func TestWriteAgnosticEntryPoints_DistributesBodyToTargets(t *testing.T) {
	dir := testutil.TempCwd(t)
	custom := "# Project\n\nMy instructions.\n"
	writeAgnosticFile(t, custom)
	cfg := &config.Config{Targets: []string{"claude", "codex"}}

	if err := writeAgnosticEntryPoints(cfg, spec.Bundle{}, []string{"claude", "codex"}, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, path := range []string{"CLAUDE.md", "AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatalf("%s not written: %v", path, err)
		}
		if !strings.Contains(string(data), custom) {
			t.Errorf("%s missing custom body; got:\n%s", path, data)
		}
	}
}

func TestWriteAgnosticEntryPoints_AgnosticFileNotOverwrittenWhenExists(t *testing.T) {
	testutil.TempCwd(t)
	custom := "# My instructions\n"
	writeAgnosticFile(t, custom)

	if err := writeAgnosticEntryPoints(&config.Config{}, spec.Bundle{}, nil, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(adapters.AgnosticEntryPointPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Errorf("AGNOSTIC_AI.md was overwritten; got:\n%s", data)
	}
}

func writeAgnosticFile(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adapters.AgnosticEntryPointPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
