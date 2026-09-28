package openhands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "openhands" {
		t.Errorf("Name() = %q, want %q", got, "openhands")
	}
}

// The project-root AGENTS.md is written centrally by sync, never by
// this adapter. Always-on rules have no direct per-rule file here.
func TestEmit_NoRootAGENTSMd_ByDefault(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Path: "rules/r1.md", Body: "rule body"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("adapter should not write AGENTS.md, err=%v", err)
	}
}

// Skills emit natively as one folder per skill under .agents/skills/,
// the shared cross-tool tree OpenHands scans.
func TestEmit_Skill_WritesSharedSkillFolder(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindSkill,
			Name: "yaml-validator",
			Meta: map[string]any{"description": "Validate YAML."},
			Body: "Validate against schema.",
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/skills/yaml-validator/SKILL.md"))
	for _, want := range []string{"name: yaml-validator", "description: Validate YAML.", "Validate against schema."} {
		if !strings.Contains(got, want) {
			t.Errorf("SKILL.md missing %q:\n%s", want, got)
		}
	}
}

func TestEmit_Skill_SkillsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{"openhands": {SkillsDir: "custom/skills"}},
	}
	entries := []spec.Entry{{Kind: spec.KindSkill, Name: "yaml-validator", Body: "body"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/skills/yaml-validator/SKILL.md")); err != nil {
		t.Errorf("expected custom/skills/yaml-validator/SKILL.md: %v", err)
	}
}

func TestEmit_Agent_WritesSharedFlatProfile(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{{
		Kind: spec.KindAgent,
		Name: "reviewer",
		Meta: map[string]any{
			"description": "Reviews code changes.",
			"model":       "anthropic/claude-sonnet-4",
			"tools":       []any{"Read"},
		},
		Body: "Review the diff.",
	}}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".agents/agents/reviewer.md"))
	for _, want := range []string{"name: reviewer", "description: Reviews code changes.", "model: anthropic/claude-sonnet-4", "Review the diff."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in agent profile:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tools:") {
		t.Errorf("portable tools leaked into OpenHands agent profile:\n%s", got)
	}
}

func TestEmit_AgentsDirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"openhands": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Body: "Review."}}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/reviewer.md")); err != nil {
		t.Errorf("expected override dir to hold the agent file: %v", err)
	}
}

func TestEmit_AgentToolsSurfaceCoverageNote(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}}}}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := emit.PendingCoverageNotesCount(); got != 1 {
		t.Errorf("expected one tools coverage note, got %d", got)
	}
}

func TestEmit_EmptyBundle_WritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(nil), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
		t.Errorf("expected no .agents dir for an empty bundle, err=%v", err)
	}
}

// An environment spec's `install` field writes `.openhands/setup.sh`
// with a shebang line and the provenance header, in that order, then
// the install command verbatim (#662).
func TestEmit_Environment_WritesSetupScript(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml",
			Meta: map[string]any{"name": "default", "scope": "ignored", "install": "go mod download"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".openhands/setup.sh"))
	if !strings.HasPrefix(got, "#!/bin/bash\n") {
		t.Errorf("setup.sh must start with a shebang, got:\n%s", got)
	}
	if !strings.Contains(got, "go mod download") {
		t.Errorf("missing install passthrough in setup.sh:\n%s", got)
	}
	for _, leaked := range []string{"name: default", "scope: ignored"} {
		if strings.Contains(got, leaked) {
			t.Errorf("routing key leaked into setup.sh: %s", got)
		}
	}
}

// A later environment spec's `install` overrides an earlier one,
// matching Cursor's last-wins merge policy for the same spec kind.
func TestEmit_Environment_MergeLastWins(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindEnvironment, Name: "a", Path: "environments/a.yaml", Meta: map[string]any{"install": "first"}},
		{Kind: spec.KindEnvironment, Name: "b", Path: "environments/b.yaml", Meta: map[string]any{"install": "second"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".openhands/setup.sh"))
	if strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("last-wins violated, got:\n%s", got)
	}
}

// The setup script path is overridable via outputs.openhands.setup-file.
func TestEmit_Environment_SetupFileOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{Outputs: map[string]config.Output{"openhands": {SetupFile: "bootstrap.sh"}}}
	entries := []spec.Entry{
		{Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml", Meta: map[string]any{"install": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bootstrap.sh")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openhands/setup.sh")); !os.IsNotExist(err) {
		t.Errorf("default path should not exist alongside the override, err=%v", err)
	}
}

// No environment spec sets `install`: nothing to write, and no error.
func TestEmit_Environment_NoInstallWritesNothing(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml",
			Meta: map[string]any{"terminals": []any{map[string]any{"name": "dev", "command": "go run ."}}}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".openhands/setup.sh")); !os.IsNotExist(err) {
		t.Errorf("expected no setup.sh when no environment spec sets install, err=%v", err)
	}
}

// `terminals` has no `.openhands/setup.sh` equivalent (the script runs
// once, synchronously, not as a set of long-running processes), so it
// surfaces a coverage note instead of vanishing silently, while
// `install` still reaches the target in full.
func TestEmit_Environment_TerminalsSurfacesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindEnvironment, Name: "default", Path: "environments/default.yaml", Meta: map[string]any{
			"install":   "go mod download",
			"terminals": []any{map[string]any{"name": "dev", "command": "go run ./cmd/agnostic-ai"}},
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`terminals`", "1 environment", "openhands"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}

	got := readFile(t, filepath.Join(dir, ".openhands/setup.sh"))
	if !strings.Contains(got, "go mod download") {
		t.Errorf("install should still reach openhands in full, got: %s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
