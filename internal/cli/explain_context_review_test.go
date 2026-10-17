package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func TestExplainContext_ImportedAuthoredEndMarkerKeepsTailAttributed(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nsync:\n  resolve-imports: inline\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n@docs/extra.md\n")
	imported := words(30) + "\n<!-- agnostic-ai:import:end -->\n" + words(40) + "\n"
	mustWriteFile(t, filepath.Join(dir, "docs", "extra.md"), imported)
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report, err := explainContext(cfg, bundle, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, c := range report.Contributions {
		if c.Source == "docs/extra.md" {
			found++
			if c.Words < 70 || c.Bytes < len(imported)-1 {
				t.Errorf("imported tail lost its source: %+v; imported bytes %d", c, len(imported))
			}
		}
	}
	if found != 1 {
		t.Errorf("expected one imported source, got %d: %+v", found, report.Contributions)
	}
}

func TestExplainContext_OmitsRulesWithoutNativeOutput(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, cursor, amp]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n")
	nativeSource := ".agnostic-ai/rules/cursor-only.md"
	nativeBody := "Cursor exclusive body. " + words(27)
	mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(nativeSource)), "---\nname: cursor-only\nx-cursor:\n  globs: src/**\n---\n"+nativeBody+"\n")
	supportedSource := ".agnostic-ai/rules/conditional.md"
	mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(supportedSource)), "---\nname: conditional\nglobs: src/**\nalwaysApply: false\n---\n"+words(40)+"\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	file, err := explainFile("src/main.go", "claude", cfg, bundle, ".")
	if err != nil {
		t.Fatal(err)
	}
	omitted := false
	for _, item := range file.Instructions {
		if item.Source == nativeSource && item.Status == contextNotEmitted {
			omitted = true
		}
	}
	if !omitted {
		t.Fatalf("fixture must have no Claude output: %+v", file.Instructions)
	}
	adapter, err := adapters.Resolve("amp")
	if err != nil {
		t.Fatal(err)
	}
	captured, err := captureEmit(adapter, bundle, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range captured {
		if strings.Contains(string(output.Content), nativeBody) {
			t.Fatalf("fixture unexpectedly emits the cursor-only AMP rule: %s", output.Path)
		}
	}
	docs, err := plannedInstructionDocs(cfg, bundle, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	for _, doc := range docs {
		for _, writer := range doc.Writers {
			if writer == "amp" && strings.Contains(doc.Content, nativeBody) {
				t.Fatalf("fixture unexpectedly includes cursor-only rule in AMP planned instructions: %s", doc.Path)
			}
		}
	}
	for _, target := range []string{"claude", "cursor", "amp"} {
		for _, path := range []string{"", "src/main.go"} {
			if target == "amp" && path != "" {
				continue
			}
			report, err := explainContext(cfg, bundle, target, path)
			if err != nil {
				t.Fatal(err)
			}
			supported := false
			native := false
			for _, c := range report.Contributions {
				if c.Source == nativeSource {
					native = true
				}
				if c.Source == supportedSource {
					supported = true
					if (c.Load == "startup" && c.Words < 40) || (c.Load != "startup" && c.Words != 40) {
						t.Errorf("supported body changed: %+v", c)
					}
				}
			}
			if !supported {
				t.Errorf("%s %q lost supported conditional rule", target, path)
			}
			if target != "cursor" && native {
				t.Errorf("%s %q reports a rule without native output: %+v", target, path, report.Contributions)
			}
			if target == "cursor" && !native {
				t.Errorf("Cursor lost its emitted native rule: %+v", report.Contributions)
			}
		}
	}
}

func TestExplainContext_RootAuthoredImportMarkersUseRendererSources(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n```md\n<!-- agnostic-ai:import:start fake.md -->\nExample text.\n<!-- agnostic-ai:import:end -->\n```\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report, err := explainContext(cfg, bundle, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range report.Contributions {
		if c.Source == "fake.md" {
			t.Errorf("authored root marker invented a source: %+v", c)
		}
	}
}

func TestExplainContext_LocalWhitespaceAndMultipleImportsKeepDisjointCounts(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nsync:\n  resolve-imports: inline\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "local", "AGNOSTIC_AI.md"), " \t\n\nLocal guidance.\n@docs/one.md\nBetween imports.\n@docs/two.md\n\n")
	mustWriteFile(t, filepath.Join(dir, "docs", "one.md"), words(20)+"\n")
	mustWriteFile(t, filepath.Join(dir, "docs", "two.md"), words(30)+"\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	report, err := explainContext(cfg, bundle, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var sum contextSize
	for _, c := range report.Contributions {
		if c.Load != "startup" {
			continue
		}
		if c.Words < 0 || c.Bytes < 0 {
			t.Errorf("negative contribution: %+v", c)
		}
		counts[c.Source]++
		sum.Words += c.Words
		sum.Bytes += c.Bytes
	}
	for _, source := range []string{"docs/one.md", "docs/two.md", ".agnostic-ai/local/AGNOSTIC_AI.md"} {
		if counts[source] != 1 {
			t.Errorf("%s counted %d times: %+v", source, counts[source], report.Contributions)
		}
	}
	if sum != report.Startup {
		t.Errorf("startup sum %+v differs from %+v", sum, report.Startup)
	}
	body, err := resolveAgnosticBody(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderEntryPointFiles(cfg, bundle, cfg.Targets, body)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if file.Path != "AGENTS.md" {
			continue
		}
		found = true
		if report.Startup.Words != wordsIn(file.Content) || report.Startup.Bytes != len(file.Content) {
			t.Errorf("counts %+v do not cover rendered output: %d words, %d bytes", report.Startup, wordsIn(file.Content), len(file.Content))
		}
	}
	if !found {
		t.Fatal("expected rendered Codex AGENTS.md")
	}
	loads, err := projectSessionLoads(cfg, projectKindSupport(cfg), bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, load := range loads {
		if load.target == "codex" && report.Startup.Words != load.total() {
			t.Errorf("lint parity: %d != %d", report.Startup.Words, load.total())
		}
	}
}

func TestExplainContext_RootFrontmatterKeepsSourceAndImportAttributed(t *testing.T) {
	dir := budgetProject(t, "targets: [codex]\nsync:\n  resolve-imports: inline\n")
	body := "---\ntitle: Project\n---\nRoot guidance.\n@docs/extra.md\n"
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), body)
	mustWriteFile(t, filepath.Join(dir, "docs", "extra.md"), words(40)+"\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	files, err := renderEntryPointFiles(cfg, bundle, cfg.Targets, body)
	if err != nil {
		t.Fatal(err)
	}
	var rendered string
	for _, file := range files {
		if file.Path == "AGENTS.md" {
			rendered = file.Content
		}
	}
	if !strings.HasPrefix(rendered, "---\ntitle: Project\n---\n\n<!-- Generated by agnostic-ai.") || !strings.Contains(rendered, "Root guidance.") {
		t.Fatalf("expected preserved frontmatter then generated header: %q", rendered)
	}
	report, err := explainContext(cfg, bundle, "codex", "")
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, c := range report.Contributions {
		counts[c.Source]++
		if c.Source == "docs/extra.md" && c.Words < 40 {
			t.Errorf("import content lost: %+v", c)
		}
		if c.Source == ".agnostic-ai/AGNOSTIC_AI.md" && c.Words < 4 {
			t.Errorf("root content lost: %+v", c)
		}
		if c.Words < 0 || c.Bytes < 0 {
			t.Errorf("negative contribution: %+v", c)
		}
	}
	for _, source := range []string{"docs/extra.md", ".agnostic-ai/AGNOSTIC_AI.md"} {
		if counts[source] != 1 {
			t.Errorf("%s counted %d times: %+v", source, counts[source], report.Contributions)
		}
	}
	if report.Startup.Words != wordsIn(rendered) || report.Startup.Bytes != len(rendered) {
		t.Errorf("startup %+v differs from rendered %d words and %d bytes", report.Startup, wordsIn(rendered), len(rendered))
	}
	loads, err := projectSessionLoads(cfg, projectKindSupport(cfg), bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, load := range loads {
		if load.target == "codex" && report.Startup.Words != load.total() {
			t.Errorf("lint parity: %d != %d", report.Startup.Words, load.total())
		}
	}
}

func TestExplainContext_AuthoredSourceMarkerCannotInventRuleEmission(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, cursor]\n")
	source := ".agnostic-ai/rules/cursor-only.md"
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root guidance.\n```md\n<!-- source: "+source+" -->\n```\n")
	mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(source)), "---\nname: cursor-only\nx-cursor:\n  globs: src/**\n---\n"+words(30)+"\n")
	cfg, bundle, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Rules) != 1 {
		t.Fatalf("expected one fixture rule, got %d", len(bundle.Rules))
	}
	adapter, err := adapters.Resolve("claude")
	if err != nil {
		t.Fatal(err)
	}
	files, err := captureEmit(adapter, singleEntryBundle(bundle.Rules[0]), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("fixture unexpectedly emits a Claude rule: %+v", files)
	}
	for _, file := range []string{"", "src/main.go"} {
		report, err := explainContext(cfg, bundle, "claude", file)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range report.Contributions {
			if c.Source == source {
				t.Errorf("authored marker invents an emitted rule for %q: %+v", file, c)
			}
		}
	}
}
