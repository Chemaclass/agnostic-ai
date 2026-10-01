package emit

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func foreignModelCaps() Capabilities {
	return Capabilities{
		Target:              "codex",
		Supports:            []spec.Kind{spec.KindAgent, spec.KindSettings},
		ForeignClaudeModels: ClaudeModelNames,
	}
}

func agentWithModel(path string, model any) spec.Entry {
	return spec.Entry{Name: "reviewer", Path: path, Meta: map[string]any{"model": model}}
}

func TestReportUnsupported_NotesSharedClaudeModelOnForeignTarget(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Agents: []spec.Entry{
		agentWithModel("agents/a.md", "sonnet"),
		agentWithModel("agents/b.md", map[string]any{"claude": "opus", "default": "claude-opus-4-8"}),
	}}
	if err := ReportUnsupported(foreignModelCaps(), b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	got := buf.String()
	for _, want := range []string{
		"`model` on 1 agent has no effect on codex (sonnet is a Claude model name; write model: {claude: sonnet} so codex uses its own default)",
		"`model` on 1 agent has no effect on codex (claude-opus-4-8 is a Claude model name; write model: {claude: claude-opus-4-8} so codex uses its own default)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing note %q in:\n%s", want, got)
		}
	}
}

func TestReportUnsupported_SkipsClaudeModelNamedForTarget(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Agents: []spec.Entry{
		agentWithModel("agents/a.md", map[string]any{"claude": "sonnet"}),
		agentWithModel("agents/b.md", map[string]any{"codex": "claude-sonnet-4-6"}),
		agentWithModel("agents/c.md", "gpt-5.5"),
		{Name: "d", Meta: map[string]any{"model": "sonnet", "x-codex": map[string]any{"model": "gpt-5.5"}}},
	}}
	if err := ReportUnsupported(foreignModelCaps(), b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	if got := buf.String(); strings.Contains(got, "`model`") {
		t.Errorf("expected no model note, got:\n%s", got)
	}
}

func TestReportUnsupported_SkipsClaudeModelTheTargetReads(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	caps := Capabilities{Target: "cursor", Supports: []spec.Kind{spec.KindAgent}, ForeignClaudeModels: []string{"sonnet", "opus", "haiku"}}
	b := spec.Bundle{Agents: []spec.Entry{
		agentWithModel("agents/a.md", "inherit"),
		agentWithModel("agents/b.md", "claude-opus-5[effort=high]"),
	}}
	if err := ReportUnsupported(caps, b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	if got := buf.String(); strings.Contains(got, "`model`") {
		t.Errorf("expected no model note, got:\n%s", got)
	}
}

func TestReportUnsupported_ErrorModeFailsOnSharedClaudeModel(t *testing.T) {
	swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Agents: []spec.Entry{agentWithModel("agents/reviewer.md", "sonnet")}}
	err := ReportUnsupported(foreignModelCaps(), b, OnUnsupportedError)
	if err == nil {
		t.Fatal("expected an error under on-unsupported: error")
	}
	want := `agents/reviewer.md: model "sonnet" is a Claude model name; write model: {claude: sonnet} so codex uses its own default`
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestReportUnsupported_NotesSharedClaudeSettingsModel(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Settings: []spec.Entry{
		{Name: "base", Path: "settings/base.yaml", Meta: map[string]any{"model": "gpt-5.5"}},
		{Name: "claude", Path: "settings/claude.yaml", Meta: map[string]any{"model": map[string]any{"default": "opus"}}},
	}}
	if err := ReportUnsupported(foreignModelCaps(), b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	want := "`model` on 1 settings has no effect on codex (opus is a Claude model name; write model: {claude: opus} so codex uses its own default)"
	if got := buf.String(); !strings.Contains(got, want) {
		t.Errorf("missing note %q in:\n%s", want, got)
	}
}

func TestReportUnsupported_SkipsSettingsModelNamedForTarget(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	b := spec.Bundle{Settings: []spec.Entry{
		{Name: "base", Path: "settings/base.yaml", Meta: map[string]any{"model": "sonnet"}},
		{Name: "codex", Path: "settings/codex.yaml", Meta: map[string]any{"model": map[string]any{"codex": "gpt-5.5"}}},
	}}
	if err := ReportUnsupported(foreignModelCaps(), b, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	if got := buf.String(); strings.Contains(got, "`model`") {
		t.Errorf("expected no model note, got:\n%s", got)
	}
}

func TestReportUnsupported_NamesTheTierOfATierModel(t *testing.T) {
	buf := swapWarner(t)
	ResetCoverageNotes()
	t.Cleanup(ResetCoverageNotes)
	agent := agentWithModel("agents/a.md", map[string]any{"claude": "opus", "default": "opus"})
	agent.ModelTier = "strong"
	if err := ReportUnsupported(foreignModelCaps(), spec.Bundle{Agents: []spec.Entry{agent}}, OnUnsupportedWarn); err != nil {
		t.Fatal(err)
	}
	FlushCoverageNotes()
	want := "(opus is a Claude model name from models.strong; add models.strong.codex so codex gets its own model)"
	if got := buf.String(); !strings.Contains(got, want) {
		t.Errorf("missing %q in:\n%s", want, got)
	}
}

func TestClaudeModel_KnowsEveryClaudeCodeAlias(t *testing.T) {
	for _, model := range []string{"sonnet", "opus", "haiku", "fable", "best", "opusplan", "sonnet[1m]", "opus[1m]", "inherit", "claude-opus-5"} {
		if !ClaudeModel(model) {
			t.Errorf("ClaudeModel(%q) = false, want true", model)
		}
	}
	for _, model := range []string{"default", "gpt-6.1-sol", "fable-5", "Opus"} {
		if ClaudeModel(model) {
			t.Errorf("ClaudeModel(%q) = true, want false", model)
		}
	}
}

func TestForeignClaudeModel_AliasListSkipsWhatAliasFreeTargetsRead(t *testing.T) {
	for _, model := range []string{"fable", "best", "opusplan", "opus[1m]"} {
		if !ForeignClaudeModel(ClaudeModelAliases, model) {
			t.Errorf("%q is an alias a target without aliases cannot load", model)
		}
	}
	for _, model := range []string{"inherit", "claude-opus-5"} {
		if ForeignClaudeModel(ClaudeModelAliases, model) {
			t.Errorf("%q is no alias, so a target that reads claude-* ids loads it", model)
		}
	}
}
