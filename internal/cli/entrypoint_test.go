package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestResolveAgnosticBody_SeedsTemplateWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
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
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "# My Project\n\nCustom instructions here.\n"
	if err := os.WriteFile(adapters.AgnosticEntryPointPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}

	body, err := resolveAgnosticBody(cfg, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body != custom {
		t.Errorf("body = %q, want %q", body, custom)
	}
}

func TestResolveAgnosticBody_StripsHeaderFromExisting(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	rawBody := "# My Project\n\nInstructions.\n"
	withHeader := header.With(rawBody, header.FormatMarkdown)
	if err := os.WriteFile(adapters.AgnosticEntryPointPath, []byte(withHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}

	body, err := resolveAgnosticBody(cfg, false)
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
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "# Project\n\nMy instructions.\n"
	if err := os.WriteFile(adapters.AgnosticEntryPointPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Targets: []string{"claude", "codex"}}

	if err := writeAgnosticEntryPoints(cfg, []string{"claude", "codex"}, false); err != nil {
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
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	custom := "# My instructions\n"
	if err := os.WriteFile(adapters.AgnosticEntryPointPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}

	if err := writeAgnosticEntryPoints(cfg, nil, false); err != nil {
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
