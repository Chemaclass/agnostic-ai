package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func opencodeRuleBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{{
		Kind: spec.KindRule,
		Name: "style",
		Path: "rules/style.md",
		Body: "always use tabs",
	}})
}

// OpenCode's instruction lookup is an upward walk for files named
// exactly `AGENTS.md` (opencode.ai/docs/rules, and `fs.up({ targets:
// ["AGENTS.md"] })` in packages/core/src/instruction-context.ts on
// branch `dev`). A project syncing only opencode must therefore get its
// rule bodies at the root `AGENTS.md`, not under `.opencode/` (#623).
func TestWriteAgnosticEntryPoints_OpencodeAloneWritesRootAgentsMD(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Pointer body\n")

	cfg := &config.Config{Targets: []string{"opencode"}}
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, opencodeRuleBundle(), cfg.Targets, false); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(got), "always use tabs") {
		t.Errorf("root AGENTS.md missing inlined rule body:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".opencode/AGENTS.md")); err == nil {
		t.Error("sync still wrote .opencode/AGENTS.md, a path OpenCode never opens")
	}
}

// opencode joining the AGENTS.md consumers must dedupe with them rather
// than collide, and the shared rules block must appear once, not once
// per inlining consumer of the same path (#623).
func TestWriteAgnosticEntryPoints_OpencodeSharesOneAgentsMDWithCodex(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Pointer body\n")

	b := opencodeRuleBundle()
	cfg := &config.Config{Targets: []string{"opencode", "codex"}}
	files, err := renderEntryPointFiles(cfg, b, cfg.Targets, "# Pointer body\n")
	if err != nil {
		t.Fatal(err)
	}
	var agents int
	for _, f := range files {
		if f.Path == "AGENTS.md" {
			agents++
		}
		if f.Path == ".opencode/AGENTS.md" {
			t.Error("renderEntryPointFiles still emits .opencode/AGENTS.md")
		}
	}
	if agents != 1 {
		t.Fatalf("want a single AGENTS.md write for opencode+codex, got %d", agents)
	}

	if err := detectCollisions(cfg, b, cfg.Targets); err != nil {
		t.Fatalf("opencode+codex sharing AGENTS.md must not collide: %v", err)
	}
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if n := strings.Count(string(got), adapters.RulesStartMarker); n != 1 {
		t.Errorf("want exactly 1 inlined rules block in AGENTS.md, got %d:\n%s", n, got)
	}
}
