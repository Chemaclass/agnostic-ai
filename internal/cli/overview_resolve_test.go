package cli

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/overview"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestOverview_ModelAndEffortMatchWhatSyncWrites(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWrite(t, dir+"/agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\nmodels:\n  fast:\n    claude: haiku\n    codex: gpt-5.4-mini\n")
	mustWrite(t, dir+"/.gitignore", ".agnostic-ai/local/\n")
	mustWrite(t, dir+"/.agnostic-ai/settings/team.yaml", "model: opus\neffort: high\n")
	mustWrite(t, dir+"/.agnostic-ai/local/settings/team.yaml", "model: sonnet\n")
	mustWrite(t, dir+"/.agnostic-ai/settings/z-codex.yaml", "targets: [codex]\nmodel: {codex: gpt-5.5}\neffort: medium\n")

	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	resolved := resolvedByTarget(t, overview.Build(in))

	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"claude", "codex"} {
		adapter, _ := adapters.Get(target)
		files, err := captureEmit(adapter, b, cfg)
		if err != nil {
			t.Fatal(err)
		}
		model, _ := emittedConfiguredModel(cfg, target, files)
		if got := resolved["model"][target]; got.Value != model || got.Dropped != "" {
			t.Errorf("%s model = %+v, sync writes %q", target, got, model)
		}
		path, _ := emittedModelFile(cfg, target)
		content, _ := capturedContent(files, path)
		effort := ""
		if target == "codex" {
			doc := map[string]any{}
			if _, err := toml.Decode(content, &doc); err != nil {
				t.Fatal(err)
			}
			effort, _ = doc["model_reasoning_effort"].(string)
		} else if strings.Contains(content, `"effortLevel": "high"`) {
			effort = "high"
		}
		if got := resolved["effort"][target]; got.Value != effort {
			t.Errorf("%s effort = %+v, sync writes %q", target, got, effort)
		}
	}

	claude := resolved["model"]["claude"]
	if claude.Value != "sonnet" || claude.From == nil || !claude.From.Local || claude.From.Path != ".agnostic-ai/local/settings/team.yaml" {
		t.Errorf("claude model = %+v, want the personal sonnet", claude)
	}
	if !hasOther(claude.Others, ".agnostic-ai/settings/team.yaml", "opus", "personal file") || !hasOther(claude.Others, ".agnostic-ai/settings/z-codex.yaml", "", "does not apply to Claude Code") {
		t.Errorf("claude others = %+v", claude.Others)
	}
	codex := resolved["model"]["codex"]
	if codex.Value != "gpt-5.5" || !hasOther(codex.Others, ".agnostic-ai/local/settings/team.yaml", "sonnet", "z-codex.yaml loads later") {
		t.Errorf("codex model = %+v", codex)
	}
	if cursor := resolved["effort"]["cursor"]; cursor.Value != "high" || cursor.Dropped == "" || !cursor.Checked {
		t.Errorf("cursor effort = %+v, want high with the note that cursor drops it", cursor)
	}
}

func resolvedByTarget(t *testing.T, view *overview.View) map[string]map[string]overview.TargetValue {
	t.Helper()
	out := map[string]map[string]overview.TargetValue{}
	for _, s := range view.Sections {
		for _, f := range s.Fields {
			for _, r := range f.Resolved {
				if out[f.Key] == nil {
					out[f.Key] = map[string]overview.TargetValue{}
				}
				out[f.Key][r.Target] = r
			}
		}
	}
	return out
}

func hasOther(others []overview.ValueOrigin, path, value, reason string) bool {
	for _, o := range others {
		if o.Path == path && (value == "" || o.Value == value) && strings.Contains(o.Reason, reason) {
			return true
		}
	}
	return false
}
