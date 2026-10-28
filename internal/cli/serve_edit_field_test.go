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
		{`{"audience":"shared","path":"settings/defaults.yaml","key":"effort","value":"high"}`, 200, "effort: high"},
		{`{"audience":"shared","key":"effort","value":"turbo"}`, 400, "effort: high"},
		{`{"audience":"shared","key":"effort","value":"low"}`, 200, "effort: low"},
		{`{"audience":"shared","key":"effort","value":null}`, 200, ""},
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
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":"sonnet"}`, func(r *http.Request) { r.Header.Set("Origin", "https://elsewhere.example") })
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
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	data, _ := os.ReadFile(file)
	want := strings.Replace(original, "model: sonnet", "model: opus", 1)
	if string(data) != want {
		t.Fatalf("changed unrelated bytes:\n%s", data)
	}
	for _, body := range []string{`{"audience":"shared","key":"model","value":""}`, `{"audience":"shared","key":"session-retention-days","value":0}`, `{"audience":"shared","key":"session-retention-days","value":1.5}`, `{"audience":"shared","key":"model","value":42}`, `{"audience":"shared","key":"model","value":"two\nlines"}`} {
		if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code != 400 {
			t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if data, _ := os.ReadFile(file); string(data) != want {
		t.Fatal("invalid edit changed file")
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":null}`, nil); rec.Code != 422 {
		t.Fatalf("clear inline comment = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(file); string(data) != want {
		t.Fatal("unsafe clear changed file")
	}
	second := filepath.Join(source, "settings", "second.yaml")
	if err := os.WriteFile(second, []byte("name: second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"effort","value":"high"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "choose a path") {
		t.Fatalf("ambiguous = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"settings/second.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 200 {
		t.Fatalf("chosen = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"../outside.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "is not a settings spec") {
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
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":"sonnet"}`, nil); rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":null}`, nil); rec.Code != 200 {
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
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"session-retention-days","value":7}`, nil); rec.Code != 400 {
		t.Fatalf("project retention = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"sonnet"}`, nil)
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
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"session-retention-days","value":7}`, nil); rec.Code != 200 {
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
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":null}`, nil); rec.Code != 200 {
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
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","path":"settings/defaults.yaml","key":"model","value":"sonnet"}`, nil)
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
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"settings/defaults.yaml","key":"model","value":"sonnet"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "settings/team.yaml") {
		t.Fatalf("default path = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"sonnet"}`, nil); rec.Code != 200 {
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
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"settings/early.yaml","key":"model","value":"haiku"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "settings/team.yaml") {
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
	if rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":null}`, nil); rec.Code != 422 {
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
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/a.yaml","key":"model","value":"opus"}`, nil); rec.Code != 200 {
		t.Fatalf("claude edit = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"haiku"}`, nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "a.yaml") {
		t.Fatalf("default target = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/nowhere.yaml","key":"model","value":"opus"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "is not a settings spec") {
		t.Fatalf("unknown path = %d %s", rec.Code, rec.Body)
	}
}

func TestEditField_GlobalLaterClaudeModelNameOverridesCodex(t *testing.T) {
	silence(t)
	source := editHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, codex]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "a.yaml"), "name: a\nmodel:\n  codex: gpt-5\n")
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "b.yaml"), "name: b\nmodel: claude-opus-4\n")
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","path":"settings/a.yaml","key":"model","target":"codex","value":"gpt-5.5"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "b.yaml sets model and overrides") {
		t.Fatalf("codex edit = %d %s", rec.Code, rec.Body)
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
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"audience":"shared","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), ".agnostic-ai/settings/defaults.yaml") {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(pack); string(data) != "name: team\nmodel: sonnet\n" {
		t.Fatalf("pack changed: %s", data)
	}
}

func TestEditField_AbsoluteSettingsSourceAndDescribedScaffold(t *testing.T) {
	silence(t)
	project := t.TempDir()
	settings := filepath.Join(t.TempDir(), "settings")
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nsources:\n  settings: "+filepath.ToSlash(settings)+"\n")
	shown := filepath.ToSlash(filepath.Join(settings, "defaults.yaml"))
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"audience":"shared","path":"`+shown+`","key":"model","value":"sonnet"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	data, err := os.ReadFile(filepath.Join(settings, "defaults.yaml"))
	if err != nil || !strings.Contains(string(data), "model: sonnet") || strings.Contains(string(data), "TODO") {
		t.Fatalf("defaults = %s %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(project, strings.TrimPrefix(settings, filepath.VolumeName(settings)))); !os.IsNotExist(err) {
		t.Fatalf("wrote under the project: %v", err)
	}
}

func TestEditField_NeverShadowsASpecNamedDefaults(t *testing.T) {
	silence(t)
	source := editHome(t)
	if err := os.Remove(filepath.Join(source, "settings", "native.yaml")); err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, filepath.Join(source, "settings", "base.yaml"), "name: defaults\npermissions:\n  deny: [Bash]\n")
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"audience":"shared","key":"model","value":"sonnet"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "settings/base.yaml") {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(source, "settings", "defaults.yaml")); !os.IsNotExist(err) {
		t.Fatalf("defaults created: %v", err)
	}
}

func TestEditField_PackValueLaterInOrderIsNotHiddenByAProjectSpec(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	mustWrite(t, ".agnostic-ai/packs/team/settings/base.yaml", "name: base\npermissions:\n  allow: [Read]\n")
	mustWrite(t, ".agnostic-ai/packs/team/settings/model.yaml", "name: model\nmodel: sonnet\n")
	base := ".agnostic-ai/settings/base.yaml"
	mustWrite(t, base, "name: base\neffort: high\n")
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+base+`","key":"model","value":"opus"}`, nil); rec.Code != 400 || !strings.Contains(rec.Body.String(), "packs/team/settings/model.yaml") {
		t.Fatalf("base path = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), ".agnostic-ai/settings/defaults.yaml") {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(base); string(data) != "name: base\neffort: high\n" {
		t.Fatalf("base = %s", data)
	}
}

func TestEditField_EditsASpecWithFlowStyleTargets(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWrite(t, ".agnostic-ai/settings/a.yaml", "name: a\nmodel: sonnet\n")
	z := ".agnostic-ai/settings/z.yaml"
	mustWrite(t, z, "name: z\ntargets: [codex]\nmodel: gpt-5\n")
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+z+`","key":"model","value":"o3"}`, nil); rec.Code != 200 {
		t.Fatalf("update = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(z); string(data) != "name: z\ntargets: [codex]\nmodel: o3\n" {
		t.Fatalf("z = %s", data)
	}
	if out, err := setScalarField([]byte("name: z\ntargets: [codex]\n"), "model", "o3"); err != nil || string(out) != "name: z\ntargets: [codex]\nmodel: o3\n" {
		t.Fatalf("insert = %q %v", out, err)
	}
}

func TestEditField_ClearRefusesAnInheritedLocalValue(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, ".agnostic-ai/settings/a.yaml", "name: a\nmodel: sonnet\n")
	local := ".agnostic-ai/local/settings/a.yaml"
	mustWrite(t, local, "name: a\neffort: high\n")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"audience":"personal","path":"`+local+`","key":"model","value":null}`, nil)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "inherits model") || !strings.Contains(rec.Body.String(), "switch to Shared") {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	if data, _ := os.ReadFile(local); string(data) != "name: a\neffort: high\n" {
		t.Fatalf("local = %s", data)
	}
}

func TestEditField_DefaultPathOverridesALaterPackValue(t *testing.T) {
	silence(t)
	project := t.TempDir()
	testutil.Chdir(t, project)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	mustWrite(t, ".agnostic-ai/packs/team/settings/base.yaml", "name: base\neffort: high\n")
	mustWrite(t, ".agnostic-ai/packs/team/settings/model.yaml", "name: model\nmodel: sonnet\n")
	mustWrite(t, ".agnostic-ai/settings/base.yaml", "name: base\nmodel: haiku\n")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"audience":"shared","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if data, err := os.ReadFile(".agnostic-ai/settings/defaults.yaml"); err != nil || !strings.Contains(string(data), "model: opus") {
		t.Fatalf("defaults = %s %v", data, err)
	}
}

func TestEditField_RejectsAStaleExpectedValue(t *testing.T) {
	silence(t)
	source := editHome(t)
	native := filepath.Join(source, "settings", "native.yaml")
	created := filepath.Join(source, "settings", "defaults.yaml")
	overlay := filepath.Join(source, "local", "settings", "native.yaml")
	mustWriteGlobalTest(t, native, "name: native\nmodel: opus\n")
	h := newEditHandler(source, true)
	for _, tc := range []struct {
		name, body string
		status     int
		file, want string
	}{
		{"team value changed outside", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":"haiku","expected":"sonnet"}`, 409, native, "name: native\nmodel: opus\n"},
		{"team clear of a changed value", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":null,"expected":"sonnet"}`, 409, native, "name: native\nmodel: opus\n"},
		{"team save over the current value", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":"haiku","expected":"opus"}`, 200, native, "name: native\nmodel: haiku\n"},
		{"retry of a save that already landed", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":"haiku","expected":"opus"}`, 200, native, "name: native\nmodel: haiku\n"},
		{"just me, first tab", `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":"high","expected":null}`, 200, overlay, "effort: high"},
		{"just me, second tab", `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":"low","expected":null}`, 409, overlay, "effort: high"},
		{"just me reset of a changed value", `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":null,"expected":"low"}`, 409, overlay, "effort: high"},
		{"just me reset", `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":null,"expected":"high"}`, 200, overlay, ""},
		{"retry of a reset that already landed", `{"audience":"personal","path":"settings/native.yaml","key":"effort","value":null,"expected":"high"}`, 200, overlay, ""},
		{"team clear", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":null,"expected":"haiku"}`, 200, native, "name: native\n"},
		{"retry of a clear that already landed", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":null,"expected":"haiku"}`, 200, native, "name: native\n"},
		{"team save after the clear", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":"haiku","expected":null}`, 200, native, "name: native\nmodel: haiku\n"},
		{"expected must be text or null", `{"audience":"shared","path":"settings/native.yaml","key":"model","value":"opus","expected":5}`, 400, native, "name: native\nmodel: haiku\n"},
	} {
		rec := postEdit(t, h, editFieldPath, tc.body, nil)
		if rec.Code != tc.status {
			t.Fatalf("%s = %d %s", tc.name, rec.Code, rec.Body)
		}
		if tc.status == 409 && !strings.Contains(rec.Body.String(), "changed after this page loaded") {
			t.Errorf("%s: conflict body = %s", tc.name, rec.Body)
		}
		got := readOptional(t, tc.file)
		if tc.want == "" && got != "" || !strings.Contains(got, tc.want) || strings.HasPrefix(tc.want, "name:") && got != tc.want {
			t.Errorf("%s: %s = %q, want %q", tc.name, tc.file, got, tc.want)
		}
	}

	if err := os.Remove(native); err != nil {
		t.Fatal(err)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"settings/defaults.yaml","key":"model","value":"sonnet","expected":null}`, nil); rec.Code != 200 {
		t.Fatalf("first tab creates the spec = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"settings/defaults.yaml","key":"model","value":"haiku","expected":null}`, nil)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "now sonnet") {
		t.Fatalf("second tab over the new spec = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, created); !strings.Contains(got, "model: sonnet") {
		t.Errorf("new spec = %q", got)
	}
}

func TestEditField_RefusesAnExpectedValueThePageHides(t *testing.T) {
	silence(t)
	source := editHome(t)
	native := filepath.Join(source, "settings", "native.yaml")
	original := "name: native\nmodel: abcdefghijklmnopqrstuvwxyz0123456789ABCD\n"
	mustWriteGlobalTest(t, native, original)
	h := newEditHandler(source, true)
	for _, body := range []string{
		`{"audience":"shared","path":"settings/native.yaml","key":"model","value":"haiku","expected":"abcdefghijklmnopqrstuvwxyz0123456789ABCE"}`,
		`{"audience":"shared","path":"settings/native.yaml","key":"model","value":"abcdefghijklmnopqrstuvwxyz0123456789ABCE","expected":"abcdefghijklmnopqrstuvwxyz0123456789ABCD"}`,
	} {
		if rec := postEdit(t, h, editFieldPath, body, nil); rec.Code == 200 {
			t.Errorf("%s = %d %s", body, rec.Code, rec.Body)
		}
		if got := readOptional(t, native); got != original {
			t.Fatalf("file = %q", got)
		}
	}
}

func TestEditField_ExpectsTheEffortAModelTierGives(t *testing.T) {
	project := scopeProject(t)
	spec := ".agnostic-ai/settings/defaults.yaml"
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nmodels:\n  fast:\n    claude: haiku\n    effort: low\n")
	mustWrite(t, spec, "name: defaults\nmodel: fast\n")
	h := newEditHandler(project, false)
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+spec+`","key":"effort","value":"high","expected":"medium"}`, nil)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "is now low") {
		t.Fatalf("stale = %d %s", rec.Code, rec.Body)
	}
	rec = postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+spec+`","key":"effort","value":null,"expected":"low"}`, nil)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "inherits effort") {
		t.Fatalf("clear = %d %s", rec.Code, rec.Body)
	}
	rec = postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+spec+`","key":"effort","value":"high","expected":"low"}`, nil)
	if rec.Code != 200 || !strings.Contains(readOptional(t, spec), "effort: high") {
		t.Fatalf("save = %d %s %q", rec.Code, rec.Body, readOptional(t, spec))
	}
}

func TestEditField_ConflictsWhenAnotherFileNowSetsTheField(t *testing.T) {
	project := scopeProject(t)
	team := ".agnostic-ai/settings/defaults.yaml"
	h := newEditHandler(project, false)
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","path":"`+team+`","key":"effort","value":"low","expected":null}`, nil); rec.Code != 200 {
		t.Fatalf("other tab = %d %s", rec.Code, rec.Body)
	}
	rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+team+`","key":"effort","value":"medium","expected":"high"}`, nil)
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "sets effort for you") {
		t.Fatalf("team save under a new personal value = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+team+`","key":"effort","value":"medium"}`, nil); rec.Code != 400 {
		t.Fatalf("without a precondition = %d %s", rec.Code, rec.Body)
	}
	if rec := postEdit(t, h, editFieldPath, `{"audience":"personal","path":"`+team+`","key":"effort","value":null,"expected":"low"}`, nil); rec.Code != 200 {
		t.Fatalf("reset = %d %s", rec.Code, rec.Body)
	}
	mustWrite(t, ".agnostic-ai/settings/zz.yaml", "name: zz\neffort: low\n")
	rec = postEdit(t, h, editFieldPath, `{"audience":"shared","path":"`+team+`","key":"effort","value":"medium","expected":"high"}`, nil)
	if rec.Code != 400 || !strings.Contains(rec.Body.String(), "overrides") {
		t.Fatalf("team save under an overriding spec the page may list = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, team); !strings.Contains(got, "effort: high") {
		t.Errorf("team spec = %q", got)
	}
}
