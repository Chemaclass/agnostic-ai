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
	for _, want := range []string{"/etc/qwen-code/system-defaults.json", "/custom/settings.json", "/home/me/.qwen/settings.json", "/work/repo/sub/.qwen/settings.json", "/work/repo/.qwen/settings.json"} {
		if !slices.Contains(got, want) {
			t.Errorf("files %v miss %s", got, want)
		}
	}
	if mac := approvals.QwenSettingsFiles("/u", "/w", "darwin", func(string) string { return "" }); !slices.Contains(mac, "/Library/Application Support/QwenCode/settings.json") {
		t.Errorf("macOS files %v miss the system file", mac)
	}
}

func TestSettingsHaveAskRules(t *testing.T) {
	dir := t.TempDir()
	for name, tc := range map[string]struct {
		body     string
		ask, err bool
	}{
		"allow only":    {`{"permissions":{"allow":["Bash"]}}`, false, false},
		"empty ask":     {`{"permissions":{"ask":[]}}`, false, false},
		"ask rule":      {`{"permissions":{"ask":["Bash(git push *)"]}}`, true, false},
		"with comments": {"{\n  // note\n  \"permissions\": {\"ask\": [\"edit\"]}\n}", true, false},
		"broken":        {`{"permissions":`, false, true},
		"ask not list":  {`{"permissions":{"ask":"edit"}}`, false, true},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			writeFile(t, path, tc.body)
			ask, err := approvals.SettingsHaveAskRules([]string{path})
			if ask != tc.ask || (err != nil) != tc.err {
				t.Errorf("ask %v err %v; want ask %v err %v", ask, err, tc.ask, tc.err)
			}
		})
	}
	if ask, err := approvals.SettingsHaveAskRules([]string{filepath.Join(dir, "missing.json")}); ask || err != nil {
		t.Errorf("missing = %v, %v", ask, err)
	}
}
