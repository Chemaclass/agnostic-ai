package approvals_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestQwenSettingsFiles_DerivesDefaultsBesideSystemSettings(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(root, "home", ".qwen")
	project := filepath.Join(root, "project")
	cwd := filepath.Join(project, "sub")
	systemDir := filepath.Join(root, "system")
	systemFile := filepath.Join(systemDir, "custom.json")
	env := map[string]string{"QWEN_CODE_SYSTEM_SETTINGS_PATH": systemFile}
	files, err := approvals.QwenSettingsFiles(user, cwd, "linux", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(systemDir, "system-defaults.json"),
		filepath.Join(user, "settings.json"),
		filepath.Join(project, ".qwen", "settings.json"),
		filepath.Join(cwd, ".qwen", "settings.json"),
		systemFile,
	}
	last := -1
	for _, path := range want {
		i := slices.Index(files, path)
		if i <= last {
			t.Errorf("files %v lack %s in precedence order", files, path)
		}
		last = i
	}
	env["QWEN_CODE_SYSTEM_DEFAULTS_PATH"] = filepath.Join(root, "explicit-defaults.json")
	files, err = approvals.QwenSettingsFiles(user, cwd, "linux", func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 || files[0] != env["QWEN_CODE_SYSTEM_DEFAULTS_PATH"] {
		t.Errorf("files %v do not start with the explicit defaults file", files)
	}
}

func TestQwenSettingsFiles_RejectsRelativeSystemPaths(t *testing.T) {
	for _, variable := range []string{"QWEN_CODE_SYSTEM_SETTINGS_PATH", "QWEN_CODE_SYSTEM_DEFAULTS_PATH"} {
		t.Run(variable, func(t *testing.T) {
			root := t.TempDir()
			env := map[string]string{variable: "settings.json"}
			files, err := approvals.QwenSettingsFiles(root, root, "linux", func(k string) string { return env[k] })
			if err == nil || files != nil {
				t.Errorf("files = %v, err = %v, want no files and an error", files, err)
			}
		})
	}
}

func TestReadJSONSettings(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		body    string
		want    approvals.JSONSettings
		wantErr bool
	}{
		"allow only":    {body: `{"permissions":{"allow":["Bash"]}}`},
		"empty ask":     {body: `{"permissions":{"ask":[]}}`},
		"ask rule":      {body: `{"permissions":{"ask":["Bash(git push *)"]}}`, want: approvals.JSONSettings{Ask: true}},
		"with comments": {body: "{\n  // note\n  \"permissions\": {\"ask\": [\"edit\"]}\n}", want: approvals.JSONSettings{Ask: true}},
		"hooks off":     {body: `{"disableAllHooks":true,"tools":{"approvalMode":"default"}}`, want: approvals.JSONSettings{HooksOff: true, ApprovalMode: "default"}},
		"broken":        {body: `{"permissions":`, wantErr: true},
		"ask not list":  {body: `{"permissions":{"ask":"edit"}}`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			writeFile(t, path, tc.body)
			got, err := approvals.ReadJSONSettings([]string{path})
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("got %+v, %v; want %+v, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
	user, project := filepath.Join(dir, "u.json"), filepath.Join(dir, "p.json")
	writeFile(t, user, `{"tools":{"approvalMode":"default"}}`)
	writeFile(t, project, `{"tools":{"approvalMode":"auto"}}`)
	if got, _ := approvals.ReadJSONSettings([]string{user, project}); got.ApprovalMode != "auto" {
		t.Errorf("approval mode = %q, want the later file's", got.ApprovalMode)
	}
	if got, err := approvals.ReadJSONSettings([]string{filepath.Join(dir, "missing.json")}); got != (approvals.JSONSettings{}) || err != nil {
		t.Errorf("missing = %+v, %v", got, err)
	}
}
