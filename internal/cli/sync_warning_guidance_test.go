package cli

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncGlobal_UnsupportedAgentsGiveOneHomeConfigFix(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	_, warnings, err := runGlobalAgentTest("--only", "amp,zed,warp,crush")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(warnings, "fix:") != 1 {
		t.Errorf("want one fix for unsupported global agents: %s", warnings)
	}
	for _, want := range []string{filepath.Join(source, "agnostic-ai.yaml"), "`targets:`", "`targets-exclude:"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("missing global fix %q: %s", want, warnings)
		}
	}
	if strings.Contains(warnings, "agents: skill") || strings.Contains(warnings, "on-unsupported: silent") {
		t.Errorf("global agent fix must address target selection: %s", warnings)
	}
}

func TestSyncJSON_FieldSuggestionsMatchPlainSyncAndPreviewModes(t *testing.T) {
	for _, mode := range []string{"--check", "--plan", "--dry-run"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, gemini]\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md"), "---\nname: deploy\nargument-hint: '[version]'\nmetadata:\n  owner: team\ndisable-model-invocation: true\n---\nDeploy.\n")
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\nreadonly: true\n---\nReview.\n")
			testutil.Chdir(t, dir)
			silence(t)
			captureLogOut(t)
			warner := captureNotes(t)

			preview := runSyncJSONDropsMode(t, mode)
			out := runSyncJSONDrops(t)
			if !slices.Equal(preview.Notes, out.Notes) {
				t.Errorf("%s suggestions differ from sync: %+v vs %+v", mode, preview.Notes, out.Notes)
			}
			if _, err := runCLI(t, "sync"); err != nil {
				t.Fatal(err)
			}
			wanted := map[string][]string{
				"argument-hint":            {"x-claude", "description"},
				"metadata":                 {"x-claude", "x-cursor"},
				"disable-model-invocation": {"invocation", "targets-exclude"},
				"readonly":                 {"read-only", "targets-exclude"},
				"sandbox_mode":             {"config.toml", "read-only", "x-codex: {sandbox_mode: null}"},
			}
			for field, parts := range wanted {
				found := false
				for _, note := range out.Notes {
					if !strings.HasPrefix(note.Message, "`"+field+"`") {
						continue
					}
					found = true
					for _, part := range parts {
						if !strings.Contains(note.Suggestion, part) {
							t.Errorf("%s suggestion lacks %q: %+v", field, part, note)
						}
					}
					if note.Suggestion == "" || !strings.Contains(warner.String(), "fix: "+note.Suggestion) {
						t.Errorf("%s JSON suggestion must match plain output: %+v\n%s", field, note, warner.String())
					}
					if strings.Contains(note.Suggestion, "on-unsupported: silent") || strings.Contains(note.Suggestion, "readonly: false") {
						t.Errorf("unsafe coverage suggestion: %+v", note)
					}
				}
				if !found {
					t.Errorf("no note for %s: %+v", field, out.Notes)
				}
			}
			warner.Reset()
			if _, err := runCLI(t, "sync"); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(warner.String(), "fix:") {
				t.Errorf("unchanged coverage notes repeated their fixes: %s", warner.String())
			}
		})
	}
}

func TestSyncGlobalJSON_SuggestionsIncludeSharedSkillLimitAndPreviewParity(t *testing.T) {
	for _, mode := range []string{"--check", "--plan", "--dry-run"} {
		t.Run(mode, func(t *testing.T) {
			_, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "skills", "deploy.md"), "---\nname: deploy\nargument-hint: '[version]'\nmetadata:\n  owner: team\ndisable-model-invocation: true\n---\nDeploy.\n")
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\nreadonly: true\n---\nReview.\n")
			out, previewWarnings, _ := runGlobalAgentTest("--only", "claude,codex,cursor,amp,zed", "--json", mode)
			if previewWarnings != "" {
				t.Errorf("global JSON %s leaked diagnostics: %s", mode, previewWarnings)
			}
			var preview syncJSONDrops
			if err := json.Unmarshal([]byte(out), &preview); err != nil {
				t.Fatalf("decode global preview: %v\n%s", err, out)
			}
			actual, warnings, err := runGlobalAgentTest("--only", "claude,codex,cursor,amp,zed", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var got syncJSONDrops
			if err := json.Unmarshal([]byte(actual), &got); err != nil {
				t.Fatal(err)
			}
			if warnings != "" {
				t.Errorf("global JSON leaked diagnostics: %s", warnings)
			}
			if !slices.Equal(preview.Warnings, got.Warnings) || !slices.Equal(preview.Notes, got.Notes) {
				t.Errorf("global %s differs from sync: %+v vs %+v", mode, preview, got)
			}
			if len(got.Warnings) != 2 {
				t.Errorf("missing unsupported global agents: %+v", got.Warnings)
			}
			for _, warning := range got.Warnings {
				if !strings.Contains(warning.Suggestion, filepath.Join(source, "agnostic-ai.yaml")) || !strings.Contains(warning.Suggestion, "targets-exclude") {
					t.Errorf("global warning lacks source fix: %+v", warning)
				}
			}
			shared := 0
			_, plain, err := runGlobalAgentTest("--only", "claude,codex,cursor,amp,zed")
			if err != nil {
				t.Fatal(err)
			}
			for _, note := range got.Notes {
				if (note.Target == "amp" || note.Target == "zed" || note.Target == "codex") && (strings.HasPrefix(note.Message, "`argument-hint`") || strings.HasPrefix(note.Message, "`metadata`")) {
					shared++
					if !strings.Contains(note.Suggestion, "shared global") || !strings.Contains(note.Suggestion, "omit target overrides") {
						t.Errorf("shared skill guidance promises target overrides: %+v", note)
					}
				}
				if note.Suggestion != "" && !strings.Contains(plain, "fix: "+note.Suggestion) {
					t.Errorf("global JSON suggestion differs from plain: %+v\n%s", note, plain)
				}
			}
			if shared == 0 {
				t.Errorf("missing shared skill notes: %+v", got.Notes)
			}
		})
	}
}

func TestSyncJSON_PolicySuggestionsKeepTheRequiredAccess(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, gemini]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "deploy.md"), "---\nname: deploy\ndisable-model-invocation: false\n---\nDeploy.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "writer.md"), "---\nname: writer\nreadonly: false\nx-codex:\n  sandbox_mode: workspace-write\n---\nWrite code.\n")
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	captureNotes(t)

	out := runSyncJSONDrops(t)
	wanted := map[string]string{
		"disable-model-invocation": "if manual-only invocation is required",
		"readonly":                 "if read-only access is required",
		"sandbox_mode":             "set the required `sandbox_mode`",
	}
	for field, condition := range wanted {
		found := false
		for _, note := range out.Notes {
			if !strings.HasPrefix(note.Message, "`"+field+"`") {
				continue
			}
			found = true
			if note.Count != 1 || !strings.Contains(note.Suggestion, condition) {
				t.Errorf("%s prescribes an unrequested restriction or changed count: %+v", field, note)
			}
			if field == "sandbox_mode" && (!strings.Contains(note.Suggestion, "if read-only access is required") || !strings.Contains(note.Suggestion, "then set `x-codex: {sandbox_mode: null}`")) {
				t.Errorf("sandbox suggestion must keep read-only conditional and omission after verification: %+v", note)
			}
		}
		if !found {
			t.Errorf("missing %s note for nonrestrictive field: %+v", field, out.Notes)
		}
	}
}

func TestSyncGlobal_UnsupportedAgentFixNamesLocalTargetOverride(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	localConfig := filepath.Join(source, "local", "agnostic-ai.yaml")
	mustWriteGlobalTest(t, localConfig, "targets: [amp]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	_, warnings, err := runGlobalAgentTest()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "amp: global agents are unsupported") || !strings.Contains(warnings, localConfig) {
		t.Errorf("unsupported global agent fix must name overriding target config: %s", warnings)
	}
}
