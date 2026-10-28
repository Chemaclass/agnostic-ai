package cli

import (
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func targetProject(t *testing.T, spec string) *editHandler {
	t.Helper()
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nmodels:\n  strong:\n    claude: opus\n    codex: gpt-5.5\n")
	if spec != "" {
		mustWrite(t, ".agnostic-ai/settings/defaults.yaml", spec)
	}
	return &editHandler{root: project}
}

func readProjectFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditFieldTarget_ScalarBecomesMapAndCollapsesBack(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5.6-terra","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet\n  codex: gpt-5.6-terra\n" {
		t.Fatalf("after set = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":null,"expected":"gpt-5.6-terra"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel: sonnet\n" {
		t.Fatalf("after reset = %q", got)
	}
}

func TestEditFieldTarget_EditsOneEntryOfAMap(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel:\n  default: sonnet # team pick\n  claude: opus\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("add codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet # team pick\n  claude: opus\n  codex: gpt-5.5\n" {
		t.Fatalf("after add = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":"haiku","expected":"opus"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("change claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"sonnet-5","expected":"sonnet"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("change all = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet-5 # team pick\n  claude: haiku\n  codex: gpt-5.5\n" {
		t.Fatalf("after changes = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":"opus","expected":"opus"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("stale claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("clear claude = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet-5 # team pick\n  codex: gpt-5.5\n" {
		t.Fatalf("after clear = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":null}`, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("clear absent claude = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_AbsentKeyStartsAMapWithoutDefault(t *testing.T) {
	h := targetProject(t, "name: defaults\neffort: high\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5.5","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\neffort: high\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("after set = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\neffort: high\n" {
		t.Fatalf("after reset = %q", got)
	}
}

func TestEditFieldTarget_TierNameStaysTheDefault(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: strong\n")
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5.6-terra","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel:\n  default: strong\n  codex: gpt-5.6-terra\n" {
		t.Fatalf("after set = %q", got)
	}
}

func TestEditFieldTarget_PersonalOverSharedScalar(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\n")
	personal := ".agnostic-ai/local/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":"gpt-5.5","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != fieldCreatedMarker+"name: defaults\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("personal = %q", got)
	}
	if got := loadedSettingsValue(t, "defaults", "model"); !reflect.DeepEqual(got, map[string]any{"default": "sonnet", "codex": "gpt-5.5"}) {
		t.Fatalf("merged model = %v", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "for you") {
		t.Fatalf("team codex under personal = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":null,"expected":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != "" {
		t.Fatalf("personal after reset = %q", got)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel: sonnet\n" {
		t.Fatalf("team changed: %q", got)
	}
}

func TestEditFieldTarget_PersonalOverSharedMapBlocksOnlyItsAddress(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel:\n  default: sonnet\n")
	personal := ".agnostic-ai/local/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != fieldCreatedMarker+"name: defaults\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("personal = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":"opus"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("team claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("team codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel:\n  default: sonnet\n  claude: opus\n" {
		t.Fatalf("team = %q", got)
	}
}

func TestEditFieldTarget_PersonalScalarOverSharedMapIsRefused(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel:\n  default: sonnet\n  claude: opus\n")
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\nmodel: haiku\n")
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code == http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("team codex under a personal scalar = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_EffortIsValidatedForTheChosenTool(t *testing.T) {
	h := targetProject(t, "name: defaults\neffort: high\n")
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","target":"claude","value":"max"}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Claude Code") && !strings.Contains(rec.Body.String(), "claude") {
		t.Fatalf("claude max = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","target":"codex","value":"max"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("codex max = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","value":"max"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("all max = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_RejectsUnknownToolsAndFieldsWithoutPerToolValues(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\nx-claude:\n  verbose: false\n")
	for _, tc := range []struct{ path, body string }{
		{editFieldPath, `{"audience":"shared","key":"model","target":"cursor","value":"x"}`},
		{editValuePath, `{"audience":"shared","path":".agnostic-ai/settings/defaults.yaml","key":"x-claude.verbose","target":"claude","value":true}`},
		{editBuiltinPath, `{"audience":"shared","name":"memory","enabled":true,"target":"claude"}`},
	} {
		if rec := postEdit(t, h, tc.path, tc.body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, rec.Code, rec.Body)
		}
	}
	if rec := postEdit(t, h, editValuePath, `{"audience":"shared","path":".agnostic-ai/settings/defaults.yaml","key":"x-claude.verbose","target":"all","value":true}`, nil); rec.Code != http.StatusOK {
		t.Errorf("value with target all = %d %s", rec.Code, rec.Body)
	}
	source := editHome(t)
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"session-retention-days","target":"claude","value":7}`, nil); rec.Code != http.StatusBadRequest {
		t.Errorf("retention per tool = %d %s", rec.Code, rec.Body)
	}
}

func TestSettingChanges_PerToolPreviewChangesOnlyThatTool(t *testing.T) {
	root := builtinChangeProject(t)
	mustWriteFile(t, ".agnostic-ai/settings/defaults.yaml", "name: defaults\nmodel: gpt-5.5\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	h := newBuiltinChangeHandler(root, false)
	settingProposal(t, h, audienceShared, map[string]any{"key": "model", "target": "cursor", "value": "x", "expected": nil}, http.StatusUnprocessableEntity)
	proposal := settingProposal(t, h, audienceShared, map[string]any{"key": "model", "target": "codex", "value": "gpt-5.6-terra", "expected": nil}, http.StatusOK)
	paths := plannedChanges(proposal)
	if !slices.ContainsFunc(paths, func(p string) bool { return strings.HasSuffix(p, ".codex/config.toml") }) {
		t.Errorf("changes = %v, want .codex/config.toml", paths)
	}
	if slices.ContainsFunc(paths, func(p string) bool { return strings.HasSuffix(p, ".claude/settings.json") }) {
		t.Errorf("changes = %v, want .claude/settings.json unchanged", paths)
	}
	if proposal.Title != "model for codex set to gpt-5.6-terra" {
		t.Errorf("title = %q", proposal.Title)
	}
	applied := builtinAction(t, h, builtinApplyPath, proposal.ID)
	if applied.Status != "applied" {
		t.Fatalf("apply = %+v", applied)
	}
	if data, _ := os.ReadFile(".codex/config.toml"); !strings.Contains(string(data), "gpt-5.6-terra") {
		t.Errorf("config.toml = %s", data)
	}
	reset := settingProposal(t, h, audienceShared, map[string]any{"key": "model", "target": "codex", "value": nil, "expected": "gpt-5.6-terra"}, http.StatusOK)
	builtinAction(t, h, builtinApplyPath, reset.ID)
	if data, _ := os.ReadFile(".agnostic-ai/settings/defaults.yaml"); string(data) != "name: defaults\nmodel: gpt-5.5\n" {
		t.Errorf("defaults.yaml = %q", data)
	}
}

func loadedSettingsValue(t *testing.T, name, key string) any {
	t.Helper()
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range scope.bundle.Settings {
		if e.Name == name {
			return e.Meta[key]
		}
	}
	t.Fatalf("no settings spec named %s", name)
	return nil
}

func TestEditFieldTarget_PersonalToolEntryFollowsTheTeamValue(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\n")
	personal := ".agnostic-ai/local/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":"gpt-5.5","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != fieldCreatedMarker+"name: defaults\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("personal = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"opus","expected":"sonnet"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("team all = %d %s", rec.Code, rec.Body)
	}
	if got := loadedSettingsValue(t, "defaults", "model"); !reflect.DeepEqual(got, map[string]any{"default": "opus", "codex": "gpt-5.5"}) {
		t.Fatalf("merged model = %v", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","value":null}`, nil); rec.Code == http.StatusOK {
		t.Fatalf("reset all with no personal default = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":null,"expected":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != "" {
		t.Fatalf("personal after reset = %q", got)
	}
}

func TestEditFieldTarget_TeamEditsUnderAPersonalMapKeepOtherTools(t *testing.T) {
	h := targetProject(t, "name: defaults\neffort: high\n")
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set codex = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"opus","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("team all = %d %s", rec.Code, rec.Body)
	}
	if got := loadedSettingsValue(t, "defaults", "model"); !reflect.DeepEqual(got, map[string]any{"default": "opus", "codex": "gpt-5.5"}) {
		t.Fatalf("merged model = %v", got)
	}
	mustWrite(t, ".agnostic-ai/settings/defaults.yaml", "name: defaults\nmodel:\n  default: sonnet\n  claude: opus\n")
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","target":"claude","value":null,"expected":"opus"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("team reset claude = %d %s", rec.Code, rec.Body)
	}
	if got := loadedSettingsValue(t, "defaults", "model"); !reflect.DeepEqual(got, map[string]any{"default": "sonnet", "codex": "gpt-5.5"}) {
		t.Fatalf("merged model after collapse = %v", got)
	}
}

func TestEditFieldTarget_TierEffortRefusesOneTool(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: strong\n")
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\nmodels:\n  strong:\n    claude: opus\n    codex: gpt-5.5\n    effort: high\n")
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","target":"codex","value":"low"}`, nil)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "tier") {
		t.Fatalf("tier effort per tool = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel: strong\n" {
		t.Fatalf("spec changed: %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","value":"low"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("tier effort for every tool = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_EffortForAToolWithoutEffort(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, gemini]\n")
	rec := postEdit(t, &editHandler{root: project}, editFieldPath, `{"audience":"shared","key":"effort","target":"gemini","value":"high"}`, nil)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "gemini has no effort setting") {
		t.Fatalf("gemini effort = %d %s", rec.Code, rec.Body)
	}
}

func TestSetFieldAt_KeepsAnInlineCommentOnTheDefault(t *testing.T) {
	in := "name: defaults\nmodel: sonnet # team pick\neffort: high\n"
	out, err := setFieldAt([]byte(in), newFieldAddress("model", "codex"), "gpt-5.5", nil)
	if want := "name: defaults\nmodel:\n  default: sonnet # team pick\n  codex: gpt-5.5\neffort: high\n"; err != nil || string(out) != want {
		t.Fatalf("expand = %q %v", out, err)
	}
	back, err := setFieldAt(out, newFieldAddress("model", "codex"), nil, nil)
	if err != nil || string(back) != in {
		t.Fatalf("collapse = %q %v", back, err)
	}
	crlf := "name: defaults\r\nmodel: sonnet # pick\r\n"
	out, err = setFieldAt([]byte(crlf), newFieldAddress("model", "codex"), "gpt-5.5", nil)
	if want := "name: defaults\r\nmodel:\r\n  default: sonnet # pick\r\n  codex: gpt-5.5\r\n"; err != nil || string(out) != want {
		t.Fatalf("crlf expand = %q %v", out, err)
	}
}

func TestEditFieldTarget_LaterPerToolValueOverridesOnlyItsTool(t *testing.T) {
	for _, tc := range []struct{ key, shared, claude, codex string }{
		{"model", "sonnet", "opus", "gpt-5.5"},
		{"effort", "high", "low", "medium"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			h := targetProject(t, "")
			mustWrite(t, ".agnostic-ai/settings/a.yaml", "name: a\n"+tc.key+": "+tc.shared+"\n")
			mustWrite(t, ".agnostic-ai/settings/b.yaml", "name: b\n"+tc.key+":\n  claude: "+tc.claude+"\n")
			edit := func(target, value string) (int, string) {
				rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/a.yaml","key":"`+tc.key+`","target":"`+target+`","value":"`+value+`"}`, nil)
				return rec.Code, rec.Body.String()
			}
			if code, body := edit("codex", tc.codex); code != http.StatusOK {
				t.Fatalf("codex edit = %d %s", code, body)
			}
			if got := readProjectFile(t, ".agnostic-ai/settings/a.yaml"); !strings.Contains(got, "codex: "+tc.codex) {
				t.Fatalf("a.yaml = %q", got)
			}
			if code, body := edit("claude", tc.codex); code != http.StatusBadRequest || !strings.Contains(body, "b.yaml sets "+tc.key+" and overrides") {
				t.Fatalf("claude edit = %d %s", code, body)
			}
			mustWrite(t, ".agnostic-ai/settings/b.yaml", "name: b\n"+tc.key+":\n  default: "+tc.codex+"\n")
			if code, body := edit("codex", tc.shared); code != http.StatusBadRequest || !strings.Contains(body, "b.yaml sets "+tc.key+" and overrides") {
				t.Fatalf("codex edit under default = %d %s", code, body)
			}
		})
	}
}

func TestEditFieldTarget_OverrideErrorNamesTheFileThatSetsTheValue(t *testing.T) {
	for _, tc := range []struct{ name, personal, want string }{
		{"shared value", "name: b\neffort: low\n", ".agnostic-ai/settings/b.yaml sets model"},
		{"personal value", "name: b\nmodel: haiku\n", ".agnostic-ai/local/settings/b.yaml sets model"},
		{"personal value for another tool", "name: b\nmodel:\n  codex: gpt-5.5\n", ".agnostic-ai/settings/b.yaml sets model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := targetProject(t, "")
			mustWrite(t, ".agnostic-ai/settings/a.yaml", "name: a\nmodel: sonnet\n")
			mustWrite(t, ".agnostic-ai/settings/b.yaml", "name: b\nmodel: opus\n")
			mustWrite(t, ".agnostic-ai/local/settings/b.yaml", tc.personal)
			rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/a.yaml","key":"model","target":"claude","value":"haiku"}`, nil)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error":"`+tc.want) {
				t.Fatalf("edit = %d %s, want %q", rec.Code, rec.Body, tc.want)
			}
		})
	}
}
