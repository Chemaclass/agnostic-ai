package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Claude Code plugin ships from this repo: `.claude-plugin/marketplace.json`
// at the root lists the plugins, and each names a directory holding its own
// `.claude-plugin/plugin.json`. CI already refuses a plugin change that forgets
// to bump the version, because the marketplace pins installs to it. Nothing
// checked that the manifests still describe something installable.
//
// Everything here is cheap and static. The point is that a typo in a path, a
// name that stops matching, or a skill missing its frontmatter fails the build
// instead of failing at `/plugin install` on someone else's machine.

type marketplaceManifest struct {
	Name  string `json:"name"`
	Owner struct {
		Name string `json:"name"`
	} `json:"owner"`
	Plugins []struct {
		Name        string `json:"name"`
		Source      string `json:"source"`
		Description string `json:"description"`
	} `json:"plugins"`
}

type pluginManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

func readMarketplace(t *testing.T) (string, marketplaceManifest) {
	t.Helper()
	root := repoRoot(t)
	path := filepath.Join(root, ".claude-plugin", "marketplace.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var m marketplaceManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return root, m
}

// repoRoot returns the repository root relative to this package.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// TestPluginMarketplace_DeclaresAnInstallableMarketplace checks the fields
// Claude Code needs to list the marketplace at all.
func TestPluginMarketplace_DeclaresAnInstallableMarketplace(t *testing.T) {
	_, m := readMarketplace(t)
	if m.Name == "" {
		t.Error("marketplace.json has no name; `/plugin install <plugin>@<marketplace>` needs it")
	}
	if m.Owner.Name == "" {
		t.Error("marketplace.json has no owner.name")
	}
	if len(m.Plugins) == 0 {
		t.Fatal("marketplace.json lists no plugins")
	}
}

// TestPluginMarketplace_EveryEntryResolvesToItsManifest is the check that
// would have caught a renamed or moved plugin directory: the marketplace
// entry's source has to point at a directory that really holds a plugin.
func TestPluginMarketplace_EveryEntryResolvesToItsManifest(t *testing.T) {
	root, m := readMarketplace(t)
	for _, entry := range m.Plugins {
		t.Run(entry.Name, func(t *testing.T) {
			if entry.Source == "" {
				t.Fatal("entry has no source")
			}
			dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(entry.Source, "./")))
			manifestPath := filepath.Join(dir, ".claude-plugin", "plugin.json")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatalf("source %q does not hold a plugin manifest: %v", entry.Source, err)
			}
			var p pluginManifest
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatalf("parse %s: %v", manifestPath, err)
			}
			// Claude Code allows these to differ, and resolves
			// `/plugin install name@marketplace` against the
			// marketplace entry's name. Keeping them equal is this
			// repo's own rule: two names for one plugin is a trap for
			// whoever next reads an install command and greps for it.
			if p.Name != entry.Name {
				t.Errorf("plugin.json name %q != marketplace entry name %q", p.Name, entry.Name)
			}
			if p.Version == "" {
				t.Error("plugin.json has no version; the marketplace pins installs to it")
			}
			if p.Description == "" {
				t.Error("plugin.json has no description")
			}
		})
	}
}

// TestPluginMarketplace_EverySkillIsLoadable checks each skill the plugin
// ships. A skill whose frontmatter is missing or whose name disagrees with
// its directory does not load, and the plugin installs anyway, so this is
// silent without a test.
func TestPluginMarketplace_EverySkillIsLoadable(t *testing.T) {
	root, m := readMarketplace(t)
	for _, entry := range m.Plugins {
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(entry.Source, "./")))
		skillsDir := filepath.Join(dir, "skills")
		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			t.Fatalf("%s: read skills dir: %v", entry.Name, err)
		}
		found := 0
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			found++
			t.Run(entry.Name+"/"+e.Name(), func(t *testing.T) {
				path := filepath.Join(skillsDir, e.Name(), "SKILL.md")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("skill directory has no SKILL.md: %v", err)
				}
				name, desc := skillFrontmatter(t, string(data), path)
				if name != e.Name() {
					t.Errorf("frontmatter name %q != directory %q; Claude Code keys the skill on the frontmatter", name, e.Name())
				}
				if desc == "" {
					t.Error("frontmatter has no description; the model cannot tell when to invoke the skill")
				}
			})
		}
		if found == 0 {
			t.Errorf("%s ships no skills", entry.Name)
		}
	}
}

// skillFrontmatter pulls name and description out of a SKILL.md YAML
// frontmatter block. It is deliberately minimal: a missing or unterminated
// block is a failure, not something to recover from.
func skillFrontmatter(t *testing.T, body, path string) (name, description string) {
	t.Helper()
	if !strings.HasPrefix(body, "---\n") {
		t.Fatalf("%s does not open with YAML frontmatter", path)
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		t.Fatalf("%s has an unterminated frontmatter block", path)
	}
	for _, line := range strings.Split(body[4:4+end], "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			name = strings.TrimSpace(value)
		case "description":
			description = strings.TrimSpace(value)
		}
	}
	return name, description
}

// TestPluginMarketplace_DocumentsItself keeps the plugin directory from being
// a bare file listing. README.md links to it as "Claude Code plugin", so a
// reader who follows that link lands here and needs to find the install
// commands without leaving the page.
func TestPluginMarketplace_DocumentsItself(t *testing.T) {
	root, m := readMarketplace(t)
	for _, entry := range m.Plugins {
		t.Run(entry.Name, func(t *testing.T) {
			dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(entry.Source, "./")))
			data, err := os.ReadFile(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatalf("plugin has no README.md: %v", err)
			}
			body := string(data)
			// Both halves of the install are needed: adding the
			// marketplace does not install anything on its own.
			for _, want := range []string{
				"/plugin marketplace add " + m.Owner.Name + "/agnostic-ai",
				"/plugin install " + entry.Name + "@" + m.Name,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("README.md does not show %q", want)
				}
			}
		})
	}
}
