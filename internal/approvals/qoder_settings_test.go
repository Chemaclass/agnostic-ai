package approvals_test

import (
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestQoderSettingsFiles(t *testing.T) {
	got := approvals.QoderSettingsFiles("/home/me", "/work/repo")
	want := []string{"/home/me/.qoder/settings.json", "/.qoder/settings.json", "/.qoder/settings.local.json", "/work/.qoder/settings.json", "/work/.qoder/settings.local.json", "/work/repo/.qoder/settings.json", "/work/repo/.qoder/settings.local.json"}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	if got := approvals.QoderSettingsFiles("/home/me", "relative", ""); !slices.Equal(got, want[:1]) {
		t.Errorf("relative cwd files = %v", got)
	}
	both := approvals.QoderSettingsFiles("/home/me", "/work/repo", "/proj")
	if !slices.Contains(both, "/proj/.qoder/settings.json") || !slices.Contains(both, "/work/repo/.qoder/settings.local.json") || len(both) != len(want)+2 {
		t.Errorf("files for two folders = %v", both)
	}
}
