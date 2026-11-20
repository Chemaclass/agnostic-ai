package approvals

import (
	"path/filepath"
	"slices"
)

// QoderSettingsFiles lists the Qoder CLI settings files whose
// permissions.ask rules may apply to a session: the user file in home,
// then settings.json and settings.local.json under .qoder in every
// folder from the root down to each of dirs, the session folder and the
// project folder Qoder names (docs.qoder.com/cli/permissions).
func QoderSettingsFiles(home string, dirs ...string) []string {
	files := []string{filepath.Join(home, ".qoder", "settings.json")}
	for _, start := range dirs {
		if !filepath.IsAbs(start) {
			continue
		}
		var project []string
		for dir := filepath.Clean(start); ; dir = filepath.Dir(dir) {
			project = append(project, filepath.Join(dir, ".qoder", "settings.local.json"), filepath.Join(dir, ".qoder", "settings.json"))
			if filepath.Dir(dir) == dir {
				break
			}
		}
		slices.Reverse(project)
		for _, file := range project {
			if !slices.Contains(files, file) {
				files = append(files, file)
			}
		}
	}
	return files
}
