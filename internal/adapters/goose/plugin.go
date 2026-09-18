package goose

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// projectPluginsDir is Goose's project plugin root: "| Project plugin |
// `<project>/.agents/plugins/<plugin-name>/` | Available when goose is
// working in that project. |"
// (documentation/docs/guides/context-engineering/plugins.md).
const projectPluginsDir = ".agents/plugins"

// pluginSkillsRoot returns the plugin root a skills directory belongs
// to, or "" when it lands outside one. Goose loads a plugin's skills
// from its own `skills/` component directory, so
// `.agents/plugins/<name>/skills` is the only layout that makes the
// folders plugin skills; anywhere else under the plugin root is not a
// component Goose reads and gets no manifest.
func pluginSkillsRoot(skillsDir string) string {
	rel := filepath.ToSlash(filepath.Clean(skillsDir))
	rest, ok := strings.CutPrefix(rel, projectPluginsDir+"/")
	if !ok {
		return ""
	}
	name, component, ok := strings.Cut(rest, "/")
	if !ok || name == "" || component != "skills" {
		return ""
	}
	return projectPluginsDir + "/" + name
}

// writePluginManifests writes one `plugin.json` per distinct plugin root
// goose output lands in. "A plugin is a directory with a plugin manifest
// and optional component directories", and "A plugin can provide skills,
// hooks, or both", so a bundle carrying only skills needs the manifest
// just as much as one carrying hooks (target-audit 2026-09-18, #862).
func writePluginManifests(sess *emit.Session, roots []string, dryRun bool) error {
	for _, root := range sortedPluginRoots(roots) {
		body, err := json.MarshalIndent(map[string]string{
			"name":        filepath.Base(root),
			"version":     "1.0.0",
			"description": "Managed by agnostic-ai",
		}, "", "  ")
		if err != nil {
			return err
		}
		if err := sess.WriteFile(filepath.Join(root, "plugin.json"), string(body)+"\n", dryRun); err != nil {
			return err
		}
	}
	return nil
}

// sortedPluginRoots drops empties and duplicates so two components of
// one plugin write a single manifest, in a stable order across runs.
func sortedPluginRoots(roots []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true
		out = append(out, root)
	}
	sort.Strings(out)
	return out
}
