package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func scopeProject(t *testing.T) string {
	t.Helper()
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: [memory]\n")
	mustWrite(t, ".agnostic-ai/settings/defaults.yaml", "name: defaults\ndescription: Team defaults.\nmodel: sonnet\neffort: high\n")
	return project
}

func readOptional(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func TestEditField_JustMeWritesAnOverlayAndResetRemovesIt(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	shared := ".agnostic-ai/settings/defaults.yaml"
	overlay := ".agnostic-ai/local/settings/defaults.yaml"
	before := readOptional(t, shared)

	rec := postEdit(t, h, editFieldPath, `{"audience":"personal","path":"`+shared+`","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), overlay) {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, shared); got != before {
		t.Errorf("shared spec changed:\n%s", got)
	}
	if got := readOptional(t, overlay); !strings.Contains(got, "name: defaults\n") || !strings.Contains(got, "model: opus\n") || strings.Contains(got, "effort") {
		t.Errorf("overlay = %q", got)
	}
	scope, err := loadCheckScope(false)
	if err != nil {
		t.Fatal(err)
	}
	if got := scope.bundle.Settings[0].Meta; got["model"] != "opus" || got["effort"] != "high" {
		t.Errorf("loaded settings = %v, want the local model over the team effort", got)
	}

	rec = postEdit(t, h, editFieldPath, `{"audience":"personal","path":"`+shared+`","key":"model","value":null}`, nil)
	if rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(overlay); !os.IsNotExist(err) {
		t.Errorf("reset kept the overlay: %v", err)
	}
	if got := readOptional(t, shared); got != before {
		t.Errorf("reset changed the shared spec:\n%s", got)
	}
}

func TestEditField_JustMeKeepsAHandWrittenOverlay(t *testing.T) {
	project := scopeProject(t)
	overlay := ".agnostic-ai/local/settings/mine.yaml"
	mustWrite(t, overlay, "# Mine.\nname: defaults\nmodel: opus\neffort: low\n")
	h := newEditHandler(project, false)
	rec := postEdit(t, h, editFieldPath, `{"audience":"personal","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":null}`, nil)
	if rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, overlay); got != "# Mine.\nname: defaults\neffort: low\n" {
		t.Errorf("overlay = %q", got)
	}
}

func TestEditField_TeamScopeWritesTheSharedSpecBehindAnOverlay(t *testing.T) {
	project := scopeProject(t)
	overlay := ".agnostic-ai/local/settings/defaults.yaml"
	mustWrite(t, overlay, "name: defaults\nmodel: opus\n")
	h := newEditHandler(project, false)

	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/defaults.yaml","key":"effort","value":"low"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("team save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, ".agnostic-ai/settings/defaults.yaml"); !strings.Contains(got, "effort: low") {
		t.Errorf("shared = %q", got)
	}
	if got := readOptional(t, overlay); got != "name: defaults\nmodel: opus\n" {
		t.Errorf("overlay changed: %q", got)
	}

	rec = postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"haiku"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), overlay) {
		t.Fatalf("team save under a local override = %d %s", rec.Code, rec.Body)
	}
}

func TestEditField_JustMeInTheGlobalViewWritesUnderLocal(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	shared := readOptional(t, filepath.Join(source, "settings", "native.yaml"))
	rec := postEdit(t, h, editFieldPath, `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":"high"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, filepath.Join(source, "local", "settings", "native.yaml")); !strings.Contains(got, "effort: high") {
		t.Errorf("global overlay = %q", got)
	}
	if got := readOptional(t, filepath.Join(source, "settings", "native.yaml")); got != shared {
		t.Errorf("wrote the shared global spec: %q", got)
	}
}

func TestEdit_RequiresAnExplicitAudience(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	for _, tc := range []struct{ path, body, want string }{
		{editFieldPath, `{"key":"model","value":"opus"}`, "audience"},
		{editFieldPath, `{"scope":"mine","key":"model","value":"opus"}`, "scope"},
		{editBuiltinPath, `{"name":"handoff","enabled":true}`, "audience"},
		{editValuePath, `{"path":".agnostic-ai/settings/defaults.yaml","key":"x","value":true}`, "audience"},
	} {
		if rec := postEdit(t, h, tc.path, tc.body, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("%s %s = %d %s", tc.path, tc.body, rec.Code, rec.Body)
		}
	}
}

func TestEditBuiltin_JustMeSeedsTheLocalListAndResetRemovesIt(t *testing.T) {
	project := scopeProject(t)
	h := newEditHandler(project, false)
	before := readOptional(t, "agnostic-ai.yaml")

	rec := postEdit(t, h, editBuiltinPath, `{"audience":"personal","name":"memory","enabled":false}`, nil)
	if rec.Code != 200 {
		t.Fatalf("turn off = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); !strings.Contains(got, "builtins: []") {
		t.Errorf("local = %q", got)
	}
	rec = postEdit(t, h, editBuiltinPath, `{"audience":"personal","name":"handoff","enabled":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("turn on = %d %s", rec.Code, rec.Body)
	}
	cfg, err := loadProjectConfig(t)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg, ",") != "handoff" {
		t.Errorf("effective builtins = %v, want handoff", cfg)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != before {
		t.Errorf("shared config changed:\n%s", got)
	}

	for _, body := range []string{`{"audience":"personal","name":"handoff","enabled":false}`, `{"audience":"personal","name":"memory","enabled":true}`} {
		if rec := postEdit(t, h, editBuiltinPath, body, nil); rec.Code != 200 {
			t.Fatalf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat("agnostic-ai.local.yaml"); !os.IsNotExist(err) {
		t.Errorf("matching the team list kept the local file: %q", readOptional(t, "agnostic-ai.local.yaml"))
	}
}

func TestEditBuiltin_ResetKeepsOtherLocalKeys(t *testing.T) {
	project := scopeProject(t)
	mustWrite(t, "agnostic-ai.local.yaml", "# Mine.\nbuiltins:\n  - memory\n  - handoff\nmemory:\n  personal: repo\n")
	rec := postEdit(t, newEditHandler(project, false), editBuiltinPath, `{"audience":"personal","name":"handoff","enabled":false}`, nil)
	if rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.local.yaml"); got != "# Mine.\nmemory:\n  personal: repo\n" {
		t.Errorf("local = %q", got)
	}
}

func TestEditBuiltin_TeamScopeRefusesAListTheLocalConfigReplaces(t *testing.T) {
	project := scopeProject(t)
	mustWrite(t, "agnostic-ai.local.yaml", "builtins: [memory, rtk]\n")
	rec := postEdit(t, newEditHandler(project, false), editBuiltinPath, `{"audience":"shared","name":"handoff","enabled":true}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "agnostic-ai.local.yaml") {
		t.Fatalf("team edit = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); strings.Contains(got, "handoff") {
		t.Errorf("shared = %q", got)
	}
}

func TestOverviewInputs_ReportTheTeamAndLocalBuiltins(t *testing.T) {
	scopeProject(t)
	mustWrite(t, "agnostic-ai.local.yaml", "builtins: [handoff]\n")
	in, err := overviewInputs(".")
	if err != nil {
		t.Fatal(err)
	}
	if !in.BuiltinsLocal || strings.Join(in.BuiltinsTeam, ",") != "memory" {
		t.Errorf("team %v local %v", in.BuiltinsTeam, in.BuiltinsLocal)
	}
}

func loadProjectConfig(t *testing.T) ([]string, error) {
	t.Helper()
	scope, err := loadCheckScope(false)
	if err != nil {
		return nil, err
	}
	return scope.cfg.Builtins, nil
}

func TestBuiltinChanges_JustMeAppliesToTheLocalConfigAndResetDeletesIt(t *testing.T) {
	root := builtinChangeProject(t)
	shared := readOptional(t, "agnostic-ai.yaml")
	h := newBuiltinChangeHandler(root, false)

	proposal := builtinScopedProposal(t, h, audiencePersonal, "caveman", true)
	if proposal.Config != "agnostic-ai.local.yaml" {
		t.Fatalf("config = %s", proposal.Config)
	}
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	if got := readOptional(t, "agnostic-ai.local.yaml"); !strings.Contains(got, "caveman") {
		t.Errorf("local = %q", got)
	}
	if got := readOptional(t, "agnostic-ai.yaml"); got != shared {
		t.Errorf("shared config changed:\n%s", got)
	}

	proposal = builtinScopedProposal(t, h, audiencePersonal, "caveman", false)
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	if _, err := os.Stat("agnostic-ai.local.yaml"); !os.IsNotExist(err) {
		t.Errorf("reset kept the local config: %q", readOptional(t, "agnostic-ai.local.yaml"))
	}
}

func TestEditValue_TeamScopeWritesASharedSpecThatAlsoGrantsPermissions(t *testing.T) {
	project := scopeProject(t)
	shared := ".agnostic-ai/settings/perm.yaml"
	mustWrite(t, shared, "name: perm\npermissions:\n  allow: [Read]\nx-claude:\n  verbose: false\n")
	mustWrite(t, ".agnostic-ai/local/settings/perm.yaml", "name: perm\nx-claude:\n  alwaysThinkingEnabled: true\n")
	rec := postEdit(t, newEditHandler(project, false), editValuePath, `{"audience":"shared","path":"`+shared+`","key":"x-claude.verbose","value":true}`, nil)
	if rec.Code != 200 {
		t.Fatalf("team toggle = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, shared); !strings.Contains(got, "verbose: true") {
		t.Errorf("shared = %q", got)
	}
}
