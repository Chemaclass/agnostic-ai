package approvals_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
)

func TestQoderSettingsFiles(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	home := filepath.Join(root, "home", "me")
	work := filepath.Join(root, "work")
	repo := filepath.Join(work, "repo")
	other := filepath.Join(root, "other")
	qoderPath := func(dir, name string) string { return filepath.Join(dir, ".qoder", name) }
	got := approvals.QoderSettingsFiles(home, repo)
	want := []string{qoderPath(home, "settings.json")}
	for _, dir := range []string{root, work, repo} {
		want = append(want, qoderPath(dir, "settings.json"), qoderPath(dir, "settings.local.json"))
	}
	if !slices.Equal(got, want) {
		t.Errorf("files = %v, want %v", got, want)
	}
	if got := approvals.QoderSettingsFiles(home, "relative", ""); !slices.Equal(got, want[:1]) {
		t.Errorf("relative cwd files = %v", got)
	}
	both := approvals.QoderSettingsFiles(home, repo, other)
	if !slices.Contains(both, qoderPath(other, "settings.json")) || !slices.Contains(both, qoderPath(repo, "settings.local.json")) || len(both) != len(want)+2 {
		t.Errorf("files for two folders = %v", both)
	}
}
