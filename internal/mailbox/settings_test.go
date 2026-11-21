package mailbox

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSettings_ApprovalStaysOnUnlessTheFileTurnsItOff(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		setup func(t *testing.T, path string)
		want  bool
	}{
		{"missing", func(*testing.T, string) {}, true},
		{"garbage", writeSettingsFile("not json"), true},
		{"empty object", writeSettingsFile("{}"), true},
		{"wrong type", writeSettingsFile(`{"approveAgentSends":"no"}`), true},
		{"unknown field", writeSettingsFile(`{"approveAgentSends":false,"other":1}`), true},
		{"directory", func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, true},
		{"turned off", writeSettingsFile(`{"approveAgentSends":false}`), false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			scenario.setup(t, filepath.Join(s.Dir, "settings.json"))
			if got := s.Settings().ApproveAgentSends; got != scenario.want {
				t.Errorf("approveAgentSends = %t, want %t", got, scenario.want)
			}
		})
	}
}

func TestSettings_UnreadableFileKeepsApprovalOn(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not block reads here")
	}
	s := Store{Dir: t.TempDir()}
	path := filepath.Join(s.Dir, "settings.json")
	writeSettingsFile(`{"approveAgentSends":false}`)(t, path)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if !s.Settings().ApproveAgentSends {
		t.Error("an unreadable settings file turned approval off")
	}
}

func TestSettings_SaveWritesAPrivateFileThatReadsBack(t *testing.T) {
	s := Store{Dir: filepath.Join(t.TempDir(), "mailbox")}
	if err := s.SaveSettings(t.Context(), Settings{ApproveAgentSends: false}); err != nil {
		t.Fatal(err)
	}
	if s.Settings().ApproveAgentSends {
		t.Error("saved setting did not read back")
	}
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(filepath.Join(s.Dir, "settings.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("settings file = %v, %v", info, err)
	}
}

func writeSettingsFile(content string) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
