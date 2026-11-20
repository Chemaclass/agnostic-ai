package approvals

import (
	"path/filepath"
	"slices"
)

// QoderSettingsFiles lists the Qoder CLI settings files whose
// permissions.ask rules may apply to a session in cwd: the user file in
// home, then settings.json and settings.local.json under .qoder in every
// folder from the root down to cwd (docs.qoder.com/cli/permissions).
func QoderSettingsFiles(home, cwd string) []string {
	files := []string{filepath.Join(home, ".qoder", "settings.json")}
	if !filepath.IsAbs(cwd) {
		return files
	}
	var project []string
	for dir := filepath.Clean(cwd); ; dir = filepath.Dir(dir) {
		project = append(project, filepath.Join(dir, ".qoder", "settings.local.json"), filepath.Join(dir, ".qoder", "settings.json"))
		if filepath.Dir(dir) == dir {
			break
		}
	}
	slices.Reverse(project)
	return append(files, project...)
}
