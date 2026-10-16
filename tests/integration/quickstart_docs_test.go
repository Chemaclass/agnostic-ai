package integration

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSiteDocs_LandingQuickstartMatchesREADME(t *testing.T) {
	want := readQuickstart(t, "README.md")

	var landing struct {
		Workflow struct {
			Steps []struct {
				Command string `toml:"command"`
			} `toml:"steps"`
			Installers []struct {
				ID      string `toml:"id"`
				Command string `toml:"command"`
			} `toml:"installers"`
		} `toml:"workflow"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode landing data: %v", err)
	}
	var commands []string
	for _, step := range landing.Workflow.Steps {
		if step.Command != "" {
			commands = append(commands, step.Command)
		}
	}
	for _, command := range commands {
		if !strings.Contains(want, command+"\n") {
			t.Errorf("landing command %q is missing from README quickstart", command)
		}
	}
	lines := strings.Split(want, "\n")
	var installer string
	for _, entry := range landing.Workflow.Installers {
		if entry.ID == "npm" {
			installer = entry.Command
		}
	}
	if installer == "" || lines[0] != installer {
		t.Errorf("quickstart install %q differs from supported npm installer %q", lines[0], installer)
	}
	for _, command := range []string{"agnostic-ai init\n", "agnostic-ai sync --plan\n", "agnostic-ai sync\n", "agnostic-ai sync --check"} {
		if !strings.Contains(want, command) {
			t.Errorf("README quickstart is missing %q", command)
		}
	}

}

func TestSiteDocs_FreshSetupRoutesExistingConfigurationToImport(t *testing.T) {
	guide := readRepoFile(t, "docs/site/content/docs/getting-started.md")
	_, manual, ok := strings.Cut(guide, "## In your terminal\n")
	if !ok {
		t.Fatal("setup guide has no terminal walkthrough")
	}
	importGuidance := strings.Index(manual, "offers to import")
	firstWrite := strings.Index(manual, "\nagnostic-ai sync\n")
	if importGuidance < 0 || firstWrite < 0 || importGuidance > firstWrite {
		t.Error("existing instructions must be handled before the first sync writes files")
	}
	if !strings.Contains(manual, "@/docs/migration.md#import-existing-tool-configuration") {
		t.Error("manual setup needs a link to selective import")
	}
	if !strings.Contains(manual, "agnostic-ai init\n") {
		t.Error("manual setup must initialize the project")
	}
	for _, command := range []string{"agnostic-ai sync --plan", "agnostic-ai sync\n", "agnostic-ai sync --check"} {
		if !strings.Contains(guide, command) {
			t.Errorf("fresh setup is missing %q", command)
		}
	}
}

func readQuickstart(t *testing.T, path string) string {
	t.Helper()
	_, section, ok := strings.Cut(readRepoFile(t, path), "## Quickstart\n")
	if !ok {
		t.Fatalf("%s has no Quickstart section", path)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	matches := regexp.MustCompile("(?s)```bash\\n(.*?)\\n```").FindAllStringSubmatch(section, -1)
	if len(matches) == 0 {
		t.Fatalf("%s has no Quickstart shell block", path)
	}
	var blocks []string
	for _, match := range matches {
		blocks = append(blocks, match[1])
	}
	return strings.Join(blocks, "\n")
}

func assertRenderedQuickstart(t *testing.T, home string) {
	t.Helper()
	section := regexp.MustCompile(`(?s)<section\b[^>]*\bid="quickstart"[^>]*>(.*?)</section>`).FindStringSubmatch(home)
	if section == nil {
		t.Fatal("landing page has no quickstart section")
	}
	for _, command := range []string{"agnostic-ai init", ".agnostic-ai/AGNOSTIC_AI.md", "agnostic-ai sync --plan", "agnostic-ai sync"} {
		if !strings.Contains(html.UnescapeString(section[1]), command) {
			t.Errorf("rendered landing workflow is missing %q", command)
		}
	}
}
