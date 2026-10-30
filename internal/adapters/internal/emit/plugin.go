package emit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectPluginsDir is the project plugin root shared by the vendors
// that follow the Open Plugins layout: Goose ("| Project plugin |
// `<project>/.agents/plugins/<plugin-name>/` |") and Antigravity
// ("place your plugin folder in `.agents/plugins/` at the root of your
// workspace").
const ProjectPluginsDir = ".agents/plugins"

// PluginRootOf returns the plugin root path belongs to, or "" when it
// lands outside one. The segment under `<root>/<name>/` must be exactly
// a recognized component: a nested path such as
// `<name>/skills/extra` is inside a component, not a component, and an
// unrelated file below a plugin directory is neither.
func PluginRootOf(path string, components map[string]bool) string {
	rest, ok := strings.CutPrefix(filepath.ToSlash(filepath.Clean(path)), ProjectPluginsDir+"/")
	if !ok {
		return ""
	}
	name, component, ok := strings.Cut(rest, "/")
	if !ok || name == "" || !components[component] {
		return ""
	}
	return ProjectPluginsDir + "/" + name
}

// WritePluginManifests writes one `plugin.json` per distinct plugin
// root. Both vendors require it before the directory is a plugin at
// all: "A plugin is a directory with a plugin manifest and optional
// component directories" (Goose) and "Every plugin requires a
// `plugin.json` file at its root to identify the directory as a plugin
// and define its metadata" (Antigravity). A bundle assembled from the
// per-kind output keys is inert without one.
//
// Callers pass roots, not component paths, because each adapter decides
// which of its outputs count as components of a plugin.
func WritePluginManifests(sess *Session, roots []string, dryRun bool) error {
	for _, root := range sortedPluginRoots(roots) {
		path := filepath.Join(root, "plugin.json")
		if PriorOutputSum == nil || PriorOutputSum(path) == "" {
			if _, err := os.Lstat(path); err == nil {
				NoteProject(fmt.Sprintf("%s: kept existing plugin manifest because sync did not write it", filepath.ToSlash(path)))
				continue
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("%s: %w", path, err)
			}
		}
		body, err := json.MarshalIndent(map[string]string{
			"name":        filepath.Base(root),
			"version":     "1.0.0",
			"description": "Managed by agnostic-ai",
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := sess.WriteFile(path, string(body)+"\n", dryRun); err != nil {
			return err
		}
	}
	return nil
}

// sortedPluginRoots drops empties and duplicates so several components
// of one plugin write a single manifest, in a stable order across runs.
func sortedPluginRoots(roots []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}
