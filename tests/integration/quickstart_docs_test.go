package integration

import (
	"html"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSiteDocs_QuickstartSharedAcrossEntryPoints(t *testing.T) {
	want := readQuickstart(t, "README.md")
	if got := readQuickstart(t, "docs/site/content/docs/getting-started.md"); got != want {
		t.Errorf("getting started quickstart differs from README:\n%s\nwant:\n%s", got, want)
	}

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
		commands = append(commands, step.Command)
	}
	if got := strings.Join(commands, "\n"); got != want {
		t.Errorf("landing quickstart differs from README:\n%s\nwant:\n%s", got, want)
	}
	lines := strings.Split(want, "\n")
	if len(lines) != 4 {
		t.Fatalf("quickstart has %d commands, want install, import, preview, sync", len(lines))
	}
	var installer string
	for _, entry := range landing.Workflow.Installers {
		if entry.ID == "npm" {
			installer = entry.Command
		}
	}
	if installer == "" || lines[0] != installer {
		t.Errorf("quickstart install %q differs from supported npm installer %q", lines[0], installer)
	}
	if lines[1] != "agnostic-ai init --from all" || lines[2] != "agnostic-ai sync --plan" || lines[3] != "agnostic-ai sync" {
		t.Errorf("quickstart must import before previewing and syncing:\n%s", want)
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
	var commands []string
	for _, match := range regexp.MustCompile(`(?s)<code>(.*?)</code>`).FindAllStringSubmatch(section[1], -1) {
		commands = append(commands, html.UnescapeString(match[1]))
	}
	if got, want := strings.Join(commands, "\n"), readQuickstart(t, "README.md"); got != want {
		t.Errorf("rendered landing quickstart differs from README:\n%s\nwant:\n%s", got, want)
	}
}
