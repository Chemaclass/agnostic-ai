package cli

import (
	"net/http"
	"os"
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
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5.6-terra","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet\n  codex: gpt-5.6-terra\n" {
		t.Fatalf("after set = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":null,"expected":"gpt-5.6-terra"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel: sonnet\n" {
		t.Fatalf("after reset = %q", got)
	}
}

func TestEditFieldTarget_EditsOneEntryOfAMap(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel:\n  default: sonnet # team pick\n  claude: opus\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("add codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet # team pick\n  claude: opus\n  codex: gpt-5.5\n" {
		t.Fatalf("after add = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"claude","value":"haiku","expected":"opus"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("change claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","value":"sonnet-5","expected":"sonnet"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("change all = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet-5 # team pick\n  claude: haiku\n  codex: gpt-5.5\n" {
		t.Fatalf("after changes = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"claude","value":"opus","expected":"opus"}`, nil); rec.Code != http.StatusConflict {
		t.Fatalf("stale claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"claude","value":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("clear claude = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\nmodel:\n  default: sonnet-5 # team pick\n  codex: gpt-5.5\n" {
		t.Fatalf("after clear = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"claude","value":null}`, nil); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("clear absent claude = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_AbsentKeyStartsAMapWithoutDefault(t *testing.T) {
	h := targetProject(t, "name: defaults\neffort: high\n")
	file := ".agnostic-ai/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5.5","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\neffort: high\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("after set = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, file); got != "name: defaults\neffort: high\n" {
		t.Fatalf("after reset = %q", got)
	}
}

func TestEditFieldTarget_TierNameStaysTheDefault(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: strong\n")
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5.6-terra","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel:\n  default: strong\n  codex: gpt-5.6-terra\n" {
		t.Fatalf("after set = %q", got)
	}
}

func TestEditFieldTarget_PersonalOverSharedScalar(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\n")
	personal := ".agnostic-ai/local/settings/defaults.yaml"
	if rec := postEdit(t, h, editFieldPath, `{"scope":"local","key":"model","target":"codex","value":"gpt-5.5","expected":null}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != fieldCreatedMarker+"name: defaults\nmodel:\n  default: sonnet\n  codex: gpt-5.5\n" {
		t.Fatalf("personal = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "for you") {
		t.Fatalf("team codex under personal = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"local","key":"model","target":"codex","value":null,"expected":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
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
	if rec := postEdit(t, h, editFieldPath, `{"scope":"local","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, personal); got != fieldCreatedMarker+"name: defaults\nmodel:\n  codex: gpt-5.5\n" {
		t.Fatalf("personal = %q", got)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"claude","value":"opus"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("team claude = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("team codex = %d %s", rec.Code, rec.Body)
	}
	if got := readProjectFile(t, ".agnostic-ai/settings/defaults.yaml"); got != "name: defaults\nmodel:\n  default: sonnet\n  claude: opus\n" {
		t.Fatalf("team = %q", got)
	}
}

func TestEditFieldTarget_PersonalScalarOverSharedMapIsRefused(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel:\n  default: sonnet\n  claude: opus\n")
	mustWrite(t, ".agnostic-ai/local/settings/defaults.yaml", "name: defaults\nmodel: haiku\n")
	if rec := postEdit(t, h, editFieldPath, `{"scope":"local","key":"model","target":"codex","value":"gpt-5.5"}`, nil); rec.Code == http.StatusOK {
		t.Fatalf("set = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"model","target":"codex","value":"gpt-5"}`, nil); rec.Code != http.StatusBadRequest {
		t.Fatalf("team codex under a personal scalar = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_EffortIsValidatedForTheChosenTool(t *testing.T) {
	h := targetProject(t, "name: defaults\neffort: high\n")
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"effort","target":"claude","value":"max"}`, nil); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Claude Code") && !strings.Contains(rec.Body.String(), "claude") {
		t.Fatalf("claude max = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"effort","target":"codex","value":"max"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("codex max = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"scope":"team","key":"effort","value":"max"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("all max = %d %s", rec.Code, rec.Body)
	}
}

func TestEditFieldTarget_RejectsUnknownToolsAndFieldsWithoutPerToolValues(t *testing.T) {
	h := targetProject(t, "name: defaults\nmodel: sonnet\nx-claude:\n  verbose: false\n")
	for _, tc := range []struct{ path, body string }{
		{editFieldPath, `{"scope":"team","key":"model","target":"cursor","value":"x"}`},
		{editValuePath, `{"scope":"team","path":".agnostic-ai/settings/defaults.yaml","key":"x-claude.verbose","target":"claude","value":true}`},
		{editBuiltinPath, `{"scope":"team","name":"memory","enabled":true,"target":"claude"}`},
	} {
		if rec := postEdit(t, h, tc.path, tc.body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, rec.Code, rec.Body)
		}
	}
	if rec := postEdit(t, h, editValuePath, `{"scope":"team","path":".agnostic-ai/settings/defaults.yaml","key":"x-claude.verbose","target":"all","value":true}`, nil); rec.Code != http.StatusOK {
		t.Errorf("value with target all = %d %s", rec.Code, rec.Body)
	}
	source := editHome(t)
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"scope":"team","key":"session-retention-days","target":"claude","value":7}`, nil); rec.Code != http.StatusBadRequest {
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
	settingProposal(t, h, scopeTeam, map[string]any{"key": "model", "target": "cursor", "value": "x", "expected": nil}, http.StatusUnprocessableEntity)
	proposal := settingProposal(t, h, scopeTeam, map[string]any{"key": "model", "target": "codex", "value": "gpt-5.6-terra", "expected": nil}, http.StatusOK)
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
	reset := settingProposal(t, h, scopeTeam, map[string]any{"key": "model", "target": "codex", "value": nil, "expected": "gpt-5.6-terra"}, http.StatusOK)
	builtinAction(t, h, builtinApplyPath, reset.ID)
	if data, _ := os.ReadFile(".agnostic-ai/settings/defaults.yaml"); string(data) != "name: defaults\nmodel: gpt-5.5\n" {
		t.Errorf("defaults.yaml = %q", data)
	}
}
