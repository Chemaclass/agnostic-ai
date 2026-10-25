package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEditField_CreateUpdateClearAndValidate(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	h := newEditHandler(source, true)
	for _, tc := range []struct {
		body     string
		status   int
		contains string
	}{
		{`{"path":"settings/defaults.yaml","key":"effort","value":"high"}`, 200, "effort: high"},
		{`{"key":"effort","value":"turbo"}`, 400, "effort: high"},
		{`{"key":"effort","value":"low"}`, 200, "effort: low"},
		{`{"key":"effort","value":null}`, 200, ""},
	} {
		rec := postEdit(t, h, editFieldPath, tc.body, nil)
		if rec.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.body, rec.Code, rec.Body)
		}
		data, err := os.ReadFile(filepath.Join(source, "settings", "defaults.yaml"))
		if tc.contains == "" {
			if !os.IsNotExist(err) {
				t.Fatalf("clear left file: %s %v", data, err)
			}
			continue
		}
		if err != nil || !strings.Contains(string(data), tc.contains) {
			t.Fatalf("file: %s %v", data, err)
		}
	}
}

func TestEditField_RejectsForeignOrigin(t *testing.T) {
	source := editHome(t)
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"key":"model","value":"sonnet"}`, func(r *http.Request) { r.Header.Set("Origin", "https://elsewhere.example") })
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
}

func TestEditField_ChoosesOnlyLoadedSpecAndPreservesOtherText(t *testing.T) {
	silence(t)
	source := editHome(t)
	original := "# Keep this comment.\nname: native\nmodel: sonnet # old choice\nx-claude:\n  verbose: false\n"
	file := filepath.Join(source, "settings", "native.yaml")
	if err := os.WriteFile(file, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	h := newEditHandler(source, true)
	rec := postEdit(t, h, editFieldPath, `{"key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(file)
	want := strings.Replace(original, "model: sonnet", "model: opus", 1)
	if string(data) != want {
		t.Fatalf("changed unrelated bytes:\n%s", data)
	}
	for _, body := range []string{`{"key":"model","value":""}`, `{"key":"session-retention-days","value":0}`, `{"key":"session-retention-days","value":1.5}`, `{"key":"model","value":42}`, `{"key":"model","value":"two\nlines"}`} {
		if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code != 400 {
			t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if data, _ := os.ReadFile(file); string(data) != want {
		t.Fatal("invalid edit changed file")
	}
	if rec := postEdit(t, h, editFieldPath, `{"key":"model","value":null}`, nil); rec.Code != 422 {
		t.Fatalf("clear inline comment = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(file); string(data) != want {
		t.Fatal("unsafe clear changed file")
	}
	second := filepath.Join(source, "settings", "second.yaml")
	if err := os.WriteFile(second, []byte("name: second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, h, editFieldPath, `{"key":"effort","value":"high"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "choose a path") {
		t.Fatalf("ambiguous = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"path":"settings/second.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 200 {
		t.Fatalf("chosen = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"path":"../outside.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "is not a settings spec") {
		t.Fatalf("outside = %d %s", rec.Code, rec.Body)
	}
}

func TestSetScalarField_RefusesUnsafeYAML(t *testing.T) {
	for _, body := range []string{"{model: old}\n", "model: &chosen old\n", "model: |\n  old\n"} {
		if _, err := setScalarField([]byte(body), "model", "new"); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
}

func TestEditField_ClearNewSpecAfterServerRestart(t *testing.T) {
	silence(t)
	source := editHome(t)
	if err := os.Remove(filepath.Join(source, "settings", "native.yaml")); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"key":"model","value":"sonnet"}`, nil); rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"key":"model","value":null}`, nil); rec.Code != 200 {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, "settings", "defaults.yaml")); !os.IsNotExist(err) {
		t.Fatalf("spec remains: %v", err)
	}
}

func TestEditField_ProjectScopeRejectsRetention(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	if err := os.WriteFile(filepath.Join(project, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editFieldPath, `{"key":"session-retention-days","value":7}`, nil); rec.Code != 400 {
		t.Fatalf("project retention = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, editFieldPath, `{"key":"model","value":"sonnet"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), ".agnostic-ai/settings/defaults.yaml") {
		t.Fatalf("project save = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(filepath.Join(project, ".agnostic-ai", "settings", "defaults.yaml")); err != nil || !strings.Contains(string(data), "model: sonnet") {
		t.Fatalf("project spec = %s %v", data, err)
	}
}

func TestEditField_GlobalRetention(t *testing.T) {
	silence(t)
	source := editHome(t)
	h := newEditHandler(source, true)
	if rec := postEdit(t, h, editFieldPath, `{"key":"session-retention-days","value":7}`, nil); rec.Code != 200 {
		t.Fatalf("global retention = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(filepath.Join(source, "settings", "native.yaml")); err != nil || !strings.Contains(string(data), "session-retention-days: 7") {
		t.Fatalf("global spec = %s %v", data, err)
	}
}

func TestEditField_ClearKeepsManuallyScaffoldedSpec(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	manual := newSpecTemplate("settings", "defaults") + "model: sonnet\n"
	file = filepath.Join(source, "settings", "defaults.yaml")
	if err := os.WriteFile(file, []byte(manual), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"key":"model","value":null}`, nil); rec.Code != 200 {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != newSpecTemplate("settings", "defaults") {
		t.Fatalf("manual scaffold lost: %s %v", data, err)
	}
}

func TestEditField_PermissionSpecDoesNotHideSettingsDefault(t *testing.T) {
	silence(t)
	source := editHome(t)
	if err := os.Remove(filepath.Join(source, "settings", "native.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "settings", "permissions.yaml"), []byte("name: permissions\npermissions:\n  allow: [Read]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"path":"settings/defaults.yaml","key":"model","value":"sonnet"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(filepath.Join(source, "settings", "defaults.yaml")); err != nil || !strings.Contains(string(data), "model: sonnet") {
		t.Fatalf("default = %s %v", data, err)
	}
}

func TestEditField_EditsTheSpecWhoseValueIsInEffect(t *testing.T) {
	silence(t)
	source := editHome(t)
	if err := os.Remove(filepath.Join(source, "settings", "native.yaml")); err != nil {
		t.Fatal(err)
	}
	team := filepath.Join(source, "settings", "team.yaml")
	if err := os.WriteFile(team, []byte("name: team\npermissions:\n  allow: [Read]\nmodel: opus\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h := newEditHandler(source, true)
	if rec := postEdit(t, h, editFieldPath, `{"path":"settings/defaults.yaml","key":"model","value":"sonnet"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "settings/team.yaml") {
		t.Fatalf("default path = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"key":"model","value":"sonnet"}`, nil); rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(team); !strings.Contains(string(data), "model: sonnet") {
		t.Fatalf("team = %s", data)
	}
	if _, err := os.Stat(filepath.Join(source, "settings", "defaults.yaml")); !os.IsNotExist(err) {
		t.Fatalf("defaults created: %v", err)
	}
	early := filepath.Join(source, "settings", "early.yaml")
	if err := os.WriteFile(early, []byte("name: early\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, h, editFieldPath, `{"path":"settings/early.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "settings/team.yaml") {
		t.Fatalf("overridden path = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(early); string(data) != "name: early\n" {
		t.Fatalf("early = %s", data)
	}
}

func TestEditField_ClearRefusesToEmptyAHandWrittenSpec(t *testing.T) {
	silence(t)
	source := editHome(t)
	file := filepath.Join(source, "settings", "native.yaml")
	if err := os.WriteFile(file, []byte("model: sonnet\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"key":"model","value":null}`, nil); rec.Code != 422 {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "model: sonnet\n" {
		t.Fatalf("hand-written spec = %q %v", data, err)
	}
}

func TestEditField_TargetedSpecOverridesOnlyItsTools(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, ".agnostic-ai/settings/a.yaml", "name: a\nmodel: sonnet\n")
	mustWrite(t, ".agnostic-ai/settings/z.yaml", "name: z\ntargets: [codex]\nmodel: gpt-5\n")
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editFieldPath, `{"path":".agnostic-ai/settings/a.yaml","key":"model","value":"opus"}`, nil); rec.Code != 200 {
		t.Fatalf("claude edit = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"key":"model","value":"haiku"}`, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "a.yaml") {
		t.Fatalf("default target = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"path":".agnostic-ai/settings/nowhere.yaml","key":"model","value":"opus"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "is not a settings spec") {
		t.Fatalf("unknown path = %d %s", rec.Code, rec.Body)
	}
}

func TestEditField_PackValueSavesToProjectDefault(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	pack := ".agnostic-ai/packs/team/settings/team.yaml"
	mustWrite(t, pack, "name: team\nmodel: sonnet\n")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"key":"model","value":"opus"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), ".agnostic-ai/settings/defaults.yaml") {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(pack); string(data) != "name: team\nmodel: sonnet\n" {
		t.Fatalf("pack changed: %s", data)
	}
}
