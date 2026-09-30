package adapters

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSkillRenderers_MatchProjectMetadataAndReportDroppedFields(t *testing.T) {
	for _, tc := range []struct{ target, path string }{
		{"claude", ".claude/skills/review/SKILL.md"},
		{"cursor", ".cursor/skills/review/SKILL.md"},
		{"codex", ".agents/skills/review/SKILL.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			var notes bytes.Buffer
			old := emit.Warner
			emit.Warner = &notes
			ResetCoverageNotes()
			t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
			skill := spec.Entry{Kind: spec.KindSkill, Name: "review", Body: "Review code.", Meta: map[string]any{
				"argument-hint": "[file]", "name": "review", "description": "Review code", "model": map[string]any{"claude": "opus", "default": "other-model"}, "effort": "high",
				"x-cursor": map[string]any{"model": "cursor-model", "effort": "low", "icon": "search"},
				"x-codex":  map[string]any{"model": "codex-model", "effort": "medium"},
			}}
			adapter, _ := Get(tc.target)
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{skill}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			FlushCoverageNotes()
			data, err := os.ReadFile(filepath.Join(dir, tc.path))
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			rendered, err := RenderSkillMarkdown(tc.target, skill, false)
			if err != nil {
				t.Fatal(err)
			}
			if got != rendered {
				t.Errorf("global renderer differs from project: %s\n%s", got, rendered)
			}
			if tc.target == "claude" {
				if !strings.Contains(got, "model: opus") || !strings.Contains(got, "effort: high") {
					t.Errorf("missing Claude metadata: %s", got)
				}
				if strings.Contains(notes.String(), "has no effect") {
					t.Errorf("unexpected Claude note: %s", notes.String())
				}
			} else {
				if strings.Contains(got, "model:") || strings.Contains(got, "effort:") {
					t.Errorf("unsupported fields emitted: %s", got)
				}
				for _, field := range []string{"model", "effort", "argument-hint"} {
					if !strings.Contains(notes.String(), "`"+field+"` on 1 skill has no effect on "+tc.target) {
						t.Errorf("missing coverage: %s", notes.String())
					}
				}
			}
			if tc.target == "cursor" && !strings.Contains(got, "icon: search") {
				t.Errorf("native Cursor key lost: %s", got)
			}
		})
	}
}

func TestSkillCoverage_ReportsEveryOmittedFieldAndKeepsNativeFields(t *testing.T) {
	for _, target := range []string{"claude", "cursor", "codex", "amp", "crush", "factory"} {
		t.Run(target, func(t *testing.T) {
			testutil.TempCwd(t)
			ResetCoverageNotes()
			t.Cleanup(ResetCoverageNotes)
			skill := spec.Entry{Kind: spec.KindSkill, Name: "review", Body: "Review.", Meta: map[string]any{
				"name": "review", "description": "Review code.", "license": "MIT",
				"argument-hint": "[file]", "allowed-tools": []any{"Read", "Bash"},
				"model": "review-model", "effort": "high", "context": "fork", "agent": "reviewer",
				"hooks": map[string]any{"Stop": []any{}}, "user-invocable": false,
				"disable-model-invocation": true, "paths": []any{"src/**"}, "icon": "search",
				"color": "blue", "metadata": map[string]any{"owner": "team"},
				"x-" + target: map[string]any{"allowed-tools": []any{"Read"}, "argument-hint": nil},
			}}
			adapter, _ := Get(target)
			if err := adapter.Emit(NewSession(), spec.NewBundle([]spec.Entry{skill}), &config.Config{}, true); err != nil {
				t.Fatal(err)
			}
			noted := map[string]int{}
			for _, note := range DrainNotes() {
				if note.Kind == spec.KindSkill && note.Shape == NoteField {
					noted[note.Field]++
				}
			}
			retained := map[string]bool{"name": true, "description": true, "license": true, "argument-hint": true, "allowed-tools": true}
			if target == "cursor" {
				for _, field := range []string{"paths", "disable-model-invocation", "icon", "color", "metadata"} {
					retained[field] = true
				}
			}
			if target == "codex" {
				retained["disable-model-invocation"] = true
			}
			for field := range skill.Meta {
				if strings.HasPrefix(field, "x-") {
					continue
				}
				want := 1
				if target == "claude" || retained[field] {
					want = 0
				}
				if noted[field] != want {
					t.Errorf("%s: %d coverage notes, want %d", field, noted[field], want)
				}
			}
		})
	}
}
