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

// A fenced AGNOSTIC_AI.md body distributes each block only to the
// entry-point files whose readers are named in that fence. The shared
// line outside any fence lands in every file.
func TestRenderEntryPointFiles_FencesPerPath(t *testing.T) {
	body := "Shared line.\n\n::target claude\nClaude-only line.\n::end\n\n::target gemini\nGemini-only line.\n::end\n\n::target codex\nCodex-only line.\n::end\n"
	cfg := &config.Config{Targets: []string{"claude", "gemini", "codex"}}

	files, err := renderEntryPointFiles(cfg, spec.Bundle{}, cfg.Targets, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	byPath := map[string]string{}
	for _, f := range files {
		byPath[f.Path] = f.Content
	}

	claude := byPath["CLAUDE.md"]
	if !strings.Contains(claude, "Claude-only line.") || strings.Contains(claude, "Gemini-only line.") || strings.Contains(claude, "Codex-only line.") {
		t.Errorf("CLAUDE.md fence filtering wrong:\n%s", claude)
	}
	gemini := byPath["GEMINI.md"]
	if !strings.Contains(gemini, "Gemini-only line.") || strings.Contains(gemini, "Claude-only line.") || strings.Contains(gemini, "Codex-only line.") {
		t.Errorf("GEMINI.md fence filtering wrong:\n%s", gemini)
	}
	agents := byPath["AGENTS.md"]
	if !strings.Contains(agents, "Codex-only line.") || strings.Contains(agents, "Claude-only line.") || strings.Contains(agents, "Gemini-only line.") {
		t.Errorf("AGENTS.md fence filtering wrong:\n%s", agents)
	}
	for path, content := range byPath {
		if strings.Contains(content, "::target") || strings.Contains(content, "::end") {
			t.Errorf("%s leaked a marker line:\n%s", path, content)
		}
		if !strings.Contains(content, "Shared line.") {
			t.Errorf("%s missing the unfenced shared line:\n%s", path, content)
		}
	}
}

// AGENTS.md is shared by several targets (codex and cline here). A
// block naming only one of its readers must still reach the file.
func TestRenderEntryPointFiles_SharedPathTakesAnyConsumerFence(t *testing.T) {
	cfg := &config.Config{Targets: []string{"codex", "cline"}}
	body := "Shared.\n\n::target cline\nCline paragraph.\n::end\n"

	files, err := renderEntryPointFiles(cfg, spec.Bundle{}, cfg.Targets, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 || files[0].Path != "AGENTS.md" {
		t.Fatalf("expected a single deduplicated AGENTS.md, got %v", files)
	}
	if !strings.Contains(files[0].Content, "Cline paragraph.") {
		t.Errorf("AGENTS.md dropped a fence naming one of its several readers:\n%s", files[0].Content)
	}
}

// Fence filtering must run before the rules appendix is inlined, so a
// codex-only paragraph still lands ahead of the generated rules block.
func TestRenderEntryPointFiles_FencedBodyPrecedesRulesAppendix(t *testing.T) {
	cfg := &config.Config{Targets: []string{"codex"}}
	body := "::target codex\nCodex-only intro.\n::end\n"
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "Always-on rule body."},
	})

	files, err := renderEntryPointFiles(cfg, b, cfg.Targets, body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one file, got %d", len(files))
	}
	content := files[0].Content
	introIdx := strings.Index(content, "Codex-only intro.")
	rulesIdx := strings.Index(content, adapters.RulesStartMarker)
	if introIdx == -1 || rulesIdx == -1 || introIdx > rulesIdx {
		t.Errorf("fenced content must precede the rules appendix:\n%s", content)
	}
}

// The source .agnostic-ai/AGNOSTIC_AI.md keeps its ::target markers:
// only the per-file renders are filtered.
func TestWriteAgnosticEntryPoints_SourceKeepsFences(t *testing.T) {
	dir := testutil.TempCwd(t)
	body := "Shared.\n\n::target claude\nClaude only.\n::end\n"
	writeAgnosticFile(t, body)
	cfg := &config.Config{Targets: []string{"claude"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(dir, adapters.AgnosticEntryPointPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "::target claude") || !strings.Contains(string(source), "::end") {
		t.Errorf("AGNOSTIC_AI.md source lost its fence markers:\n%s", source)
	}
}
