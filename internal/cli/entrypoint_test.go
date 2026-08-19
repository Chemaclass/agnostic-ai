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
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
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
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "appears hand-authored") {
		t.Errorf("expected no warning on already-managed file, got:\n%s", buf.String())
	}
}

func TestResolveAgnosticBody_SeedsTemplateWhenAbsent(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Sources: config.Sources{Rules: ".agnostic-ai/rules"}}

	body, err := resolveAgnosticBody(adapters.NewSession(), cfg, false)
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

	body, err := resolveAgnosticBody(adapters.NewSession(), &config.Config{}, false)
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

	body, err := resolveAgnosticBody(adapters.NewSession(), &config.Config{}, false)
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

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, []string{"claude", "codex"}, false); err != nil {
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

	if err := writeAgnosticEntryPoints(adapters.NewSession(), &config.Config{}, spec.Bundle{}, nil, false); err != nil {
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

func TestWriteAgnosticEntryPoints_ImportModeWiresRulesIntoClaude(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nMy instructions.\n")
	cfg := &config.Config{
		Targets: []string{"claude"},
		Outputs: map[string]config.Output{"claude": {RulesMode: "import"}},
	}
	b := spec.Bundle{Rules: []spec.Entry{
		{Kind: spec.KindRule, Name: "style", Path: "rules/style.md", Body: "style body"},
	}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{adapters.RulesStartMarker, "@.claude/rules/style.md", "My instructions."} {
		if !strings.Contains(got, want) {
			t.Errorf("CLAUDE.md missing %q:\n%s", want, got)
		}
	}
	// Import mode keeps the pointer body; it must not inline rule bodies.
	if strings.Contains(got, "style body") {
		t.Errorf("import mode must not inline rule bodies:\n%s", got)
	}
}

func TestWriteAgnosticEntryPoints_DefaultClaudeOmitsRulesBlock(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nMy instructions.\n")
	cfg := &config.Config{Targets: []string{"claude"}}
	b := spec.Bundle{Rules: []spec.Entry{
		{Kind: spec.KindRule, Name: "style", Path: "rules/style.md", Body: "style body"},
	}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), adapters.RulesStartMarker) {
		t.Errorf("default claude must not wire rules into CLAUDE.md:\n%s", data)
	}
}

func TestWriteAgnosticEntryPoints_StripImportsForNonResolvingTargets(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nGuidance.\n\n@docs/architecture.md\n")
	cfg := &config.Config{
		Targets: []string{"claude", "codex"},
		Sync:    config.SyncConfig{ResolveImports: "strip"},
	}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claudeBody := readFile(t, filepath.Join(dir, "CLAUDE.md"))
	if !strings.Contains(claudeBody, "@docs/architecture.md") {
		t.Errorf("claude resolves imports; CLAUDE.md must keep the @-line:\n%s", claudeBody)
	}
	codexBody := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if strings.Contains(codexBody, "@docs/architecture.md") {
		t.Errorf("codex cannot resolve imports; AGENTS.md should drop the @-line:\n%s", codexBody)
	}
}

func TestWriteAgnosticEntryPoints_InlineImportsForNonResolvingTargets(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "architecture.md"), []byte("Layered design.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeAgnosticFile(t, "# Project\n\n@docs/architecture.md\n")
	cfg := &config.Config{
		Targets: []string{"claude", "codex"},
		Sync:    config.SyncConfig{ResolveImports: "inline"},
	}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if claudeBody := readFile(t, filepath.Join(dir, "CLAUDE.md")); !strings.Contains(claudeBody, "@docs/architecture.md") {
		t.Errorf("CLAUDE.md must keep the @-line for native resolution:\n%s", claudeBody)
	}
	codexBody := readFile(t, filepath.Join(dir, "AGENTS.md"))
	if !strings.Contains(codexBody, "Layered design.") {
		t.Errorf("AGENTS.md should inline the referenced content:\n%s", codexBody)
	}
}

func TestWriteAgnosticEntryPoints_DefaultPassesImportsThrough(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\n@docs/architecture.md\n")
	cfg := &config.Config{Targets: []string{"codex"}}

	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, spec.Bundle{}, cfg.Targets, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body := readFile(t, filepath.Join(dir, "AGENTS.md")); !strings.Contains(body, "@docs/architecture.md") {
		t.Errorf("default mode must pass @-lines through verbatim:\n%s", body)
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

// Zed writes .rules, not the shared AGENTS.md, because
// .github/copilot-instructions.md outranks AGENTS.md in Zed's ordered
// lookup and carries no rules. Cline still gets AGENTS.md for the
// pointer body; its rules arrive natively through .cline/rules/.
func TestWriteAgnosticEntryPoints_ZedWritesRulesFileNotAGENTSMd(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nMy instructions.\n")
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	})

	cfg := &config.Config{Targets: []string{"zed", "cline"}}
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, []string{"zed", "cline"}, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules, err := os.ReadFile(filepath.Join(dir, ".rules"))
	if err != nil {
		t.Fatalf(".rules not written: %v", err)
	}
	if !strings.Contains(string(rules), "My instructions.") {
		t.Errorf(".rules missing pointer body:\n%s", rules)
	}
	if !strings.Contains(string(rules), "rule body") {
		t.Errorf("zed inlines rules, so .rules must carry them:\n%s", rules)
	}

	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not written for cline: %v", err)
	}
	if !strings.Contains(string(data), "My instructions.") {
		t.Errorf("AGENTS.md missing pointer body:\n%s", data)
	}
}

func TestWriteAgnosticEntryPoints_ClineAloneNoInlineRules(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeAgnosticFile(t, "# Project\n\nMy instructions.\n")
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
	})

	cfg := &config.Config{Targets: []string{"cline"}}
	if err := writeAgnosticEntryPoints(adapters.NewSession(), cfg, b, []string{"cline"}, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not written: %v", err)
	}
	if strings.Contains(string(data), "rule body") {
		t.Errorf("cline delivers rules via .cline/rules/; AGENTS.md must stay a slim pointer:\n%s", data)
	}
}
