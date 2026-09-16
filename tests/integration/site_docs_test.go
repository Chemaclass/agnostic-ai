package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

const siteDocsContentDir = "../../docs/site/content/docs"

const cronitorRUMSnippet = `https://rum.cronitor.io/script.js`
const cronitorRUMClientKey = `329b6c9d061abda36830327a5baa667a`

func TestSiteDocs_AllPageShellsLoadCronitorRUM(t *testing.T) {
	for _, path := range []string{
		"../../docs/site/templates/base.html",
		"../../docs/playground/index.html",
	} {
		page := readBuiltFile(t, path)
		if !strings.Contains(page, cronitorRUMSnippet) {
			t.Errorf("%s does not load Cronitor RUM", path)
		}
		if !strings.Contains(page, cronitorRUMClientKey) {
			t.Errorf("%s does not configure the Cronitor RUM client key", path)
		}
	}
}

func TestSiteDocs_LandingCapabilityMatrixMatchesAdapters(t *testing.T) {
	var landing struct {
		Targets struct {
			Features []string `toml:"features"`
			Matrix   []struct {
				ID       string `toml:"id"`
				Coverage int    `toml:"coverage"`
				Support  []bool `toml:"support"`
			} `toml:"matrix"`
		} `toml:"targets"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode landing data: %v", err)
	}

	wantFeatures := []string{"Rules", "Agents", "Skills", "MCP", "Hooks", "Commands", "Permissions"}
	if strings.Join(landing.Targets.Features, ",") != strings.Join(wantFeatures, ",") {
		t.Fatalf("capability features = %v, want %v", landing.Targets.Features, wantFeatures)
	}
	if len(landing.Targets.Matrix) != 10 {
		t.Fatalf("capability matrix has %d targets, want 10", len(landing.Targets.Matrix))
	}

	capabilitiesRE := regexp.MustCompile(`Supports:\s*\[\]spec\.Kind\{([^}]*)\}`)
	kinds := []string{"Rule", "Agent", "Skill", "MCP", "Hook", "Command"}
	// Permissions are a Settings field, not a spec kind. These adapters map the portable allow, deny, and ask lists.
	permissionTargets := map[string]bool{"claude": true, "qoder": true}
	selected := make(map[string]bool, len(landing.Targets.Matrix))

	for _, target := range landing.Targets.Matrix {
		if selected[target.ID] {
			t.Errorf("capability matrix repeats %s", target.ID)
		}
		selected[target.ID] = true
		if len(target.Support) != len(wantFeatures) {
			t.Errorf("%s has %d feature cells, want %d", target.ID, len(target.Support), len(wantFeatures))
			continue
		}

		source := readBuiltFile(t, filepath.Join("../../internal/adapters", target.ID, target.ID+".go"))
		match := capabilitiesRE.FindStringSubmatch(source)
		if len(match) != 2 {
			t.Fatalf("find declared capabilities for %s", target.ID)
		}
		declared := match[1]
		coverage := strings.Count(declared, "spec.Kind")
		if target.Coverage != coverage {
			t.Errorf("%s coverage = %d, adapter declares %d kinds", target.ID, target.Coverage, coverage)
		}

		for index, kind := range kinds {
			want := strings.Contains(declared, "spec.Kind"+kind)
			if target.Support[index] != want {
				t.Errorf("%s %s support = %t, adapter says %t", target.ID, wantFeatures[index], target.Support[index], want)
			}
		}
		if target.Support[len(target.Support)-1] != permissionTargets[target.ID] {
			t.Errorf("%s portable permissions support = %t, want %t", target.ID, target.Support[len(target.Support)-1], permissionTargets[target.ID])
		}
	}
}

func TestSiteDocs_PlaygroundUsesSharedNavigation(t *testing.T) {
	page := readBuiltFile(t, "../../docs/playground/index.html")
	for _, required := range []string{
		`href="../assets/styles/base.css"`,
		`src="../assets/scripts/theme.js"`,
		`class="site-header"`,
		`class="site-nav"`,
		`class="theme-toggle"`,
		`class="signal-rule"`,
		`href="./" aria-current="page">Playground</a>`,
	} {
		if !strings.Contains(page, required) {
			t.Errorf("playground navigation is missing %q", required)
		}
	}
	for _, label := range []string{"Home", "Updates", "Playground", "Docs", "GitHub"} {
		if !strings.Contains(page, ">"+label+"</a>") {
			t.Errorf("playground navigation is missing %s", label)
		}
	}
	if strings.Contains(page, `class="topbar"`) || strings.Contains(page, `class="topbar-links"`) {
		t.Error("playground still carries its separate navigation implementation")
	}
}

func TestSiteDocs_PlaygroundSurfacesAdapterCapabilities(t *testing.T) {
	page := readBuiltFile(t, "../../docs/playground/index.html")
	script := readBuiltFile(t, "../../docs/playground/playground.js")
	if !strings.Contains(page, `<option value="agent" selected>agent</option>`) {
		t.Error("playground does not default to the agent spec kind")
	}
	if !strings.Contains(script, `const DEFAULT_TARGETS = ["claude", "codex", "gemini"];`) {
		t.Error("playground defaults must select only claude, codex, and gemini")
	}
	for _, kind := range []string{"agent", "skill", "rule", "hook", "mcp", "command", "settings", "review", "environment", "ignore"} {
		if !strings.Contains(page, `value="`+kind+`"`) {
			t.Errorf("playground kind picker is missing %s", kind)
		}
		if !strings.Contains(script, kind+": `") {
			t.Errorf("playground samples are missing %s", kind)
		}
	}
	for _, required := range []string{
		"window.agnosticAICapabilities()",
		"updateCapabilityState()",
		"Unsupported selections have dashed outlines and are skipped.",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("playground capability UI is missing %q", required)
		}
	}
}

func TestSiteDocs_CanonicalPagesCarryNavigationMetadata(t *testing.T) {
	pages, err := filepath.Glob(filepath.Join(siteDocsContentDir, "[^_]*.md"))
	if err != nil {
		t.Fatalf("find documentation pages: %v", err)
	}
	if len(pages) == 0 {
		t.Fatal("no documentation pages found")
	}

	allowedGroups := map[string]bool{"Start": true, "Workflows": true, "Reference": true}
	weights := make(map[int]string, len(pages))
	agentSetupPrompt := ""
	for _, path := range pages {
		source := readBuiltFile(t, path)
		if !strings.HasPrefix(source, "+++\n") {
			t.Errorf("%s has no TOML frontmatter", filepath.Base(path))
			continue
		}
		frontmatterEnd := strings.Index(source[4:], "\n+++")
		if frontmatterEnd < 0 {
			t.Errorf("%s has no closing frontmatter delimiter", filepath.Base(path))
			continue
		}

		var metadata struct {
			Title       string `toml:"title"`
			Description string `toml:"description"`
			Weight      int    `toml:"weight"`
			Extra       struct {
				Group  string `toml:"group"`
				Prompt string `toml:"prompt"`
			} `toml:"extra"`
		}
		if _, err := toml.Decode(source[4:4+frontmatterEnd], &metadata); err != nil {
			t.Errorf("parse %s frontmatter: %v", filepath.Base(path), err)
			continue
		}
		if metadata.Title == "" || metadata.Description == "" {
			t.Errorf("%s needs a title and description", filepath.Base(path))
		}
		if !allowedGroups[metadata.Extra.Group] {
			t.Errorf("%s has unknown group %q", filepath.Base(path), metadata.Extra.Group)
		}
		if previous, duplicate := weights[metadata.Weight]; duplicate {
			t.Errorf("%s and %s share weight %d", previous, filepath.Base(path), metadata.Weight)
		}
		weights[metadata.Weight] = filepath.Base(path)
		if filepath.Base(path) == "agent-setup.md" {
			agentSetupPrompt = metadata.Extra.Prompt
		}
	}
	if agentSetupPrompt == "" {
		t.Fatal("agent-setup.md needs an extra.prompt value")
	}
	if readme := readBuiltFile(t, "../../README.md"); !strings.Contains(readme, agentSetupPrompt) {
		t.Error("README agent setup prompt differs from the canonical guide prompt")
	}

	if _, err := os.Stat("../../docs/user"); !os.IsNotExist(err) {
		t.Errorf("docs/user still exists; public documentation must have one canonical source")
	}
}

func TestSiteDocs_BuildsBrowsablePublicGuides(t *testing.T) {
	zolaPath, err := exec.LookPath("zola")
	if err != nil {
		t.Skip("zola is not installed")
	}

	outputDir := t.TempDir()
	command := exec.Command(zolaPath, "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build documentation site: %v\n%s", err, output)
	}

	index := readBuiltFile(t, filepath.Join(outputDir, "docs", "index.html"))
	guide := readBuiltFile(t, filepath.Join(outputDir, "docs", "getting-started", "index.html"))
	home := readBuiltFile(t, filepath.Join(outputDir, "index.html"))
	for _, required := range []string{
		"Documentation without detours.",
		"Paste into your coding agent",
		"/agnostic-ai/agent-setup.txt",
		"/agnostic-ai/docs/agent-setup/",
		"/agnostic-ai/docs/getting-started/",
		"/agnostic-ai/docs/cli-reference/",
		"/agnostic-ai/docs/troubleshooting/",
	} {
		if !strings.Contains(index, required) {
			t.Errorf("documentation index is missing %q", required)
		}
	}
	for _, required := range []string{
		"Browse documentation",
		`aria-current="page"`,
		"https://chemaclass.github.io/agnostic-ai/docs/getting-started/",
		"On this page",
		"Edit this page on GitHub",
		"/agnostic-ai/docs/migration/",
	} {
		if !strings.Contains(guide, required) {
			t.Errorf("getting-started guide is missing %q", required)
		}
	}
	if strings.Contains(guide, "docs/user") || strings.Contains(guide, "@/docs/") {
		t.Error("getting-started guide exposes a source-only documentation path")
	}
	for _, required := range []string{
		"Set up agnostic-ai with a coding agent",
		"/agnostic-ai/docs/agent-setup/",
		"agnostic-ai agent setup",
		"/agnostic-ai/agent-setup.txt",
	} {
		if !strings.Contains(home, required) {
			t.Errorf("home page is missing %q", required)
		}
	}
}

func TestSiteDocs_BuildsPlainTextAgentEntryPoints(t *testing.T) {
	outputDir := t.TempDir()
	command := exec.Command("bash", "./scripts/build-llm-docs.sh", outputDir)
	command.Dir = "../.."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build LLM documentation: %v\n%s", err, output)
	}

	agentSetup := readBuiltFile(t, filepath.Join(outputDir, "agent-setup.txt"))
	fullDocs := readBuiltFile(t, filepath.Join(outputDir, "llms-full.txt"))
	for _, required := range []string{
		"# Set up agnostic-ai with a coding agent",
		"## Safety contract",
		"agnostic-ai init --from all",
		"agnostic-ai sync --check",
		"https://chemaclass.github.io/agnostic-ai/docs/installation/",
	} {
		if !strings.Contains(agentSetup, required) {
			t.Errorf("agent-setup.txt is missing %q", required)
		}
	}
	if !strings.Contains(fullDocs, "# Set up agnostic-ai with a coding agent") {
		t.Error("llms-full.txt does not include the agent setup guide")
	}
	for name, content := range map[string]string{"agent-setup.txt": agentSetup, "llms-full.txt": fullDocs} {
		if strings.Contains(content, "@/docs/") || strings.Contains(content, "\n+++\n") {
			t.Errorf("%s exposes Zola-only source syntax", name)
		}
	}
}
