package approvals_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestQwenSettingsFiles(t *testing.T) {
	env := map[string]string{"QWEN_CODE_SYSTEM_SETTINGS_PATH": "/custom/settings.json"}
	got := approvals.QwenSettingsFiles("/home/me/.qwen", "/work/repo/sub", "linux", func(k string) string { return env[k] })
	want := []string{"/etc/qwen-code/system-defaults.json", "/home/me/.qwen/settings.json", "/.qwen/settings.json", "/work/.qwen/settings.json", "/work/repo/.qwen/settings.json", "/work/repo/sub/.qwen/settings.json", "/custom/settings.json"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v in precedence order", got, want)
	}
	if mac := approvals.QwenSettingsFiles("/u", "/w", "darwin", func(string) string { return "" }); !slices.Contains(mac, "/Library/Application Support/QwenCode/settings.json") {
		t.Errorf("macOS files %v miss the system file", mac)
	}
}

func TestReadQwenSettings(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		body    string
		want    approvals.QwenSettings
		wantErr bool
	}{
		"allow only":    {body: `{"permissions":{"allow":["Bash"]}}`},
		"empty ask":     {body: `{"permissions":{"ask":[]}}`},
		"ask rule":      {body: `{"permissions":{"ask":["Bash(git push *)"]}}`, want: approvals.QwenSettings{Ask: true}},
		"with comments": {body: "{\n  // note\n  \"permissions\": {\"ask\": [\"edit\"]}\n}", want: approvals.QwenSettings{Ask: true}},
		"hooks off":     {body: `{"disableAllHooks":true,"tools":{"approvalMode":"default"}}`, want: approvals.QwenSettings{HooksOff: true, ApprovalMode: "default"}},
		"broken":        {body: `{"permissions":`, wantErr: true},
		"ask not list":  {body: `{"permissions":{"ask":"edit"}}`, wantErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			writeFile(t, path, tc.body)
			got, err := approvals.ReadQwenSettings([]string{path})
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Errorf("got %+v, %v; want %+v, error %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
	user, project := filepath.Join(dir, "u.json"), filepath.Join(dir, "p.json")
	writeFile(t, user, `{"tools":{"approvalMode":"default"}}`)
	writeFile(t, project, `{"tools":{"approvalMode":"auto"}}`)
	if got, _ := approvals.ReadQwenSettings([]string{user, project}); got.ApprovalMode != "auto" {
		t.Errorf("approval mode = %q, want the later file's", got.ApprovalMode)
	}
	if got, err := approvals.ReadQwenSettings([]string{filepath.Join(dir, "missing.json")}); got != (approvals.QwenSettings{}) || err != nil {
		t.Errorf("missing = %+v, %v", got, err)
	}
}
