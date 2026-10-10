package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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
		if !strings.Contains(page, `<svg class="brand-mark" viewBox="0 0 64 64" aria-hidden="true" focusable="false"><path class="brand-spokes"`) {
			t.Errorf("%s does not use the hub brand mark", path)
		}
		if !strings.Contains(page, `%3Ccircle cx='32' cy='32' r='8' fill='%23f0a35e'/%3E`) {
			t.Errorf("%s favicon does not use the hub brand mark", path)
		}
	}
}

func TestSiteDocs_TargetCapabilityMatrixMatchesAdapters(t *testing.T) {
	var matrix struct {
		Features []struct {
			ID string `toml:"id"`
		} `toml:"features"`
		Targets []struct {
			ID       string   `toml:"id"`
			Statuses []string `toml:"statuses"`
		} `toml:"targets"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/capabilities.toml", &matrix); err != nil {
		t.Fatalf("decode target capability data: %v", err)
	}
	wantFeatures := []string{"agent", "skill", "rule", "hook", "mcp", "command", "settings", "review", "environment", "ignore"}
	if len(matrix.Features) != len(wantFeatures) {
		t.Fatalf("target capability matrix has %d features, want %d", len(matrix.Features), len(wantFeatures))
	}
	for index, feature := range matrix.Features {
		if feature.ID != wantFeatures[index] {
			t.Fatalf("target capability feature %d = %q, want %q", index, feature.ID, wantFeatures[index])
		}
	}

	kinds := map[string]spec.Kind{
		"agent": spec.KindAgent, "skill": spec.KindSkill, "rule": spec.KindRule,
		"hook": spec.KindHook, "mcp": spec.KindMCP, "command": spec.KindCommand,
		"settings": spec.KindSettings, "review": spec.KindReview,
		"environment": spec.KindEnvironment, "ignore": spec.KindIgnore,
	}
	declared := map[string]map[spec.Kind]bool{}
	for _, target := range adapters.CapabilityMatrix() {
		declared[target.Name] = map[spec.Kind]bool{}
		for _, kind := range target.Supports {
			declared[target.Name][kind] = true
		}
	}
	if len(matrix.Targets) != len(declared) {
		t.Fatalf("target capability matrix has %d targets, adapters declare %d", len(matrix.Targets), len(declared))
	}

	validStatuses := map[string]bool{"native": true, "mapped": true, "opt-in": true, "source": true, "no": true}
	seen := map[string]bool{}
	for _, target := range matrix.Targets {
		if seen[target.ID] {
			t.Errorf("target capability matrix repeats %s", target.ID)
		}
		seen[target.ID] = true
		supported, ok := declared[target.ID]
		if !ok {
			t.Errorf("target capability matrix contains unknown target %s", target.ID)
			continue
		}
		if len(target.Statuses) != len(matrix.Features) {
			t.Errorf("%s has %d status cells, want %d", target.ID, len(target.Statuses), len(matrix.Features))
			continue
		}
		for index, status := range target.Statuses {
			if !validStatuses[status] {
				t.Errorf("%s has unknown status %q", target.ID, status)
				continue
			}
			kind, ok := kinds[matrix.Features[index].ID]
			if !ok {
				t.Errorf("target capability matrix has unknown feature %q", matrix.Features[index].ID)
				continue
			}
			if got, want := status != "no", supported[kind]; got != want {
				t.Errorf("%s %s status = %q, adapter support = %t", target.ID, kind, status, want)
			}
		}
	}
	for target := range declared {
		if !seen[target] {
			t.Errorf("target capability matrix is missing %s", target)
		}
	}
}

func TestSiteDocs_PlaygroundUsesSharedNavigation(t *testing.T) {
	page := readBuiltFile(t, "../../docs/playground/index.html")
	for _, required := range []string{
		`href="../assets/styles/base.css"`,
		`src="../assets/scripts/theme.js"`,
		`property="og:url" content="https://agnostic-ai.org/playground/"`,
		`property="og:site_name" content="agnostic-ai.org"`,
		`name="twitter:domain" content="agnostic-ai.org"`,
		`name="twitter:url" content="https://agnostic-ai.org/playground/"`,
		`class="site-header"`,
		`class="site-nav"`,
		`class="theme-toggle"`,
		`class="signal-rule"`,
		`href="./" aria-current="page">Playground</a>`,
		`data-search-open`,
		`data-search-index="../search-index/"`,
		`src="../assets/scripts/search.js"`,
	} {
		if !strings.Contains(page, required) {
			t.Errorf("playground navigation is missing %q", required)
		}
	}
	site := readBuiltFile(t, "../../docs/site/templates/base.html")
	if got, want := siteNavLabels(t, page), siteNavLabels(t, site); !slices.Equal(got, want) {
		t.Errorf("playground navigation = %v, want the site's %v", got, want)
	}
	if strings.Contains(page, `class="topbar"`) || strings.Contains(page, `class="topbar-links"`) {
		t.Error("playground still carries its separate navigation implementation")
	}
	if strings.Contains(page, "chemaclass.github.io/agnostic-ai") {
		t.Error("playground still references the legacy GitHub Pages project URL")
	}
}

// The playground offers the kinds a newcomer meets first; the rest live in the spec format.
var playgroundKinds = []string{"agent", "skill", "rule", "hook", "mcp", "command", "settings"}

var siteNavLink = regexp.MustCompile(`>([^<>]+)</a>`)

func siteNavLabels(t *testing.T, page string) []string {
	t.Helper()
	_, nav, ok := strings.Cut(page, `<nav class="site-nav"`)
	if !ok {
		t.Fatal("page has no site navigation")
	}
	nav, _, _ = strings.Cut(nav, "</nav>")
	var labels []string
	for _, m := range siteNavLink.FindAllStringSubmatch(nav, -1) {
		labels = append(labels, m[1])
	}
	return labels
}

// The playground is a demo, so it offers a short list of well-known targets
// and points to the full list instead of showing every registered target.
func TestSiteDocs_PlaygroundDemoesWellKnownTargets(t *testing.T) {
	script := readBuiltFile(t, "../../docs/playground/playground.js")
	match := regexp.MustCompile(`const DEMO_TARGETS = \[([^\]]*)\];`).FindStringSubmatch(script)
	if match == nil {
		t.Fatal("playground has no DEMO_TARGETS list")
	}
	var demo []string
	for _, m := range regexp.MustCompile(`"([a-z]+)"`).FindAllStringSubmatch(match[1], -1) {
		demo = append(demo, m[1])
	}
	if want := []string{"claude", "codex", "copilot", "gemini", "cursor"}; !slices.Equal(demo, want) {
		t.Errorf("playground demoes %v, want %v", demo, want)
	}
	registered := map[string]bool{}
	for _, target := range adapters.CapabilityMatrix() {
		registered[target.Name] = true
	}
	for _, name := range demo {
		if !registered[name] {
			t.Errorf("demo target %q is not a registered adapter", name)
		}
	}
	for _, required := range []string{`"../docs/targets/"`, "more-targets"} {
		if !strings.Contains(script, required) {
			t.Errorf("playground does not point to the full target list: missing %q", required)
		}
	}
}

func TestSiteDocs_PlaygroundSurfacesAdapterCapabilities(t *testing.T) {
	page := readBuiltFile(t, "../../docs/playground/index.html")
	script := readBuiltFile(t, "../../docs/playground/playground.js")
	if !strings.Contains(page, `<option value="agent" selected>agent</option>`) {
		t.Error("playground does not default to the agent spec kind")
	}
	for _, kind := range playgroundKinds {
		if !strings.Contains(page, `value="`+kind+`"`) {
			t.Errorf("playground kind picker is missing %s", kind)
		}
		if !strings.Contains(script, kind+": `") {
			t.Errorf("playground samples are missing %s", kind)
		}
	}
	for _, required := range []string{
		"window.agnosticAICapabilities()",
		"supportsKind(target, kind)",
		"sampleKind(els.source.value)",
		"buildSampleAction()",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("playground capability UI is missing %q", required)
		}
	}
	if !strings.Contains(page, `class="kind-control"`) || !strings.Contains(page, `id="sample" class="ghost sample-action"`) {
		t.Error("playground does not group the contextual sample action with the kind selector")
	}
	if strings.Contains(page, `<select id="sample"`) {
		t.Error("playground still exposes an independent sample selector")
	}
}

// The kind picker is where most people meet the full list of spec kinds, so
// every kind needs a one-line description and a link that lands on a real
// spec format page. A renamed page has to fail here rather than ship a
// dropdown full of dead links (#980).
func TestSiteDocs_PlaygroundKindPickerExplainsEveryKind(t *testing.T) {
	page := readBuiltFile(t, "../../docs/playground/index.html")
	script := readBuiltFile(t, "../../docs/playground/playground.js")
	for _, required := range []string{
		`id="kind-summary"`,
		`id="kind-doc"`,
		`class="kind-hint"`,
	} {
		if !strings.Contains(page, required) {
			t.Errorf("playground kind picker is missing %q", required)
		}
	}
	if !strings.Contains(script, "updateKindHint()") {
		t.Error("playground never fills in the kind description")
	}

	entry := regexp.MustCompile(`(?s)\n  (\w+): \{(.*?)\n  \},`)
	found := make(map[string]bool)
	for _, match := range entry.FindAllStringSubmatch(script, -1) {
		kind, body := match[1], match[2]
		found[kind] = true
		page := regexp.MustCompile(`page: "([^"]+)"`).FindStringSubmatch(body)
		if page == nil {
			t.Errorf("playground kind %s has no docs page", kind)
			continue
		}
		if _, err := os.Stat(filepath.Join(siteDocsContentDir, "spec-format", page[1]+".md")); err != nil {
			t.Errorf("playground kind %s links to %s/, which is not a page under spec-format/", kind, page[1])
		}
		if !regexp.MustCompile(`summary: "[^"]{20,}"`).MatchString(body) {
			t.Errorf("playground kind %s has no usable one-line description", kind)
		}
	}
	for _, kind := range playgroundKinds {
		if !found[kind] {
			t.Errorf("playground kind %s has no description entry", kind)
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
				Group string `toml:"group"`
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
	}
	var siteConfig struct {
		Extra struct {
			AgentSetupPrompt string `toml:"agent_setup_prompt"`
		} `toml:"extra"`
	}
	if _, err := toml.DecodeFile("../../docs/site/config.toml", &siteConfig); err != nil {
		t.Fatalf("parse site config: %v", err)
	}
	agentSetupPrompt := siteConfig.Extra.AgentSetupPrompt
	if agentSetupPrompt == "" {
		t.Fatal("site config needs an extra.agent_setup_prompt value")
	}
	if readme := readBuiltFile(t, "../../README.md"); !strings.Contains(readme, agentSetupPrompt) {
		t.Error("README agent setup prompt differs from the canonical guide prompt")
	}

	if _, err := os.Stat("../../docs/user"); !os.IsNotExist(err) {
		t.Errorf("docs/user still exists; public documentation must have one canonical source")
	}
}

func TestSiteDocs_BuildsBrowsablePublicGuides(t *testing.T) {
	zolaPath := lookupPinnedZola(t)

	outputDir := t.TempDir()
	command := exec.Command(zolaPath, "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build documentation site: %v\n%s", err, output)
	}

	index := readBuiltFile(t, filepath.Join(outputDir, "docs", "index.html"))
	guide := readBuiltFile(t, filepath.Join(outputDir, "docs", "getting-started", "index.html"))
	agentSetupGuide := readBuiltFile(t, filepath.Join(outputDir, "docs", "agent-setup", "index.html"))
	targets := readBuiltFile(t, filepath.Join(outputDir, "docs", "targets", "index.html"))
	targetBehavior := readBuiltFile(t, filepath.Join(outputDir, "docs", "target-behavior", "index.html"))
	home := readBuiltFile(t, filepath.Join(outputDir, "index.html"))
	normalizedHome := strings.ReplaceAll(home, "&#x2F;", "/")
	normalizedIndex := strings.ReplaceAll(index, "&#x2F;", "/")
	normalizedAgentSetupGuide := strings.ReplaceAll(agentSetupGuide, "&#x2F;", "/")
	if domain := strings.TrimSpace(readBuiltFile(t, filepath.Join(outputDir, "CNAME"))); domain != "agnostic-ai.org" {
		t.Errorf("built CNAME = %q, want agnostic-ai.org", domain)
	}
	for _, required := range []string{
		"Documentation.",
		"Paste into your coding agent",
		"/agent-setup.txt",
		"/docs/agent-setup/",
		"/docs/installation/",
		"/docs/getting-started/",
		"/docs/cli-reference/",
		"/docs/troubleshooting/",
	} {
		if !strings.Contains(index, required) {
			t.Errorf("documentation index is missing %q", required)
		}
	}
	for _, required := range []string{
		`id="demo"`,
		"Watch it run.",
		`data-video-id="uEG6ITlqyHU"`,
		`href="https://www.youtube.com/watch?v=uEG6ITlqyHU"`,
		"assets/images/demo-poster.webp",
		"assets/scripts/video.js",
		`"@type": "VideoObject"`,
	} {
		if !strings.Contains(normalizedIndex, required) {
			t.Errorf("documentation index is missing the demo %q", required)
		}
	}
	if strings.Contains(index, "<iframe") {
		t.Error("documentation index loads the demo player before the visitor asks for it")
	}
	if directoryIndex, demoIndex := strings.Index(index, `class="docs-directory"`), strings.Index(index, `id="demo"`); directoryIndex < 0 || demoIndex < 0 || demoIndex < directoryIndex {
		t.Errorf("the demo does not close the documentation index: %d, %d", directoryIndex, demoIndex)
	}
	for _, required := range []string{
		"Browse documentation",
		`aria-current="page"`,
		"https://agnostic-ai.org/docs/getting-started/",
		"On this page",
		"Edit this page on GitHub",
		"/docs/migration/",
	} {
		if !strings.Contains(guide, required) {
			t.Errorf("getting-started guide is missing %q", required)
		}
	}
	if strings.Contains(guide, "docs/user") || strings.Contains(guide, "@/docs/") {
		t.Error("getting-started guide exposes a source-only documentation path")
	}
	for _, required := range []string{
		`data-capability-browser`,
		`data-capability-target="claude"`,
		`href="https://agnostic-ai.org/docs/targets/"`,
		`href="https://agnostic-ai.org/docs/targets/codex/#config-keys"`,
		`How to read it`,
		`Dedicated target format.`,
		`Another native format preserves the content.`,
		`Each column is a portable spec kind.`,
		`Why these columns?`,
		`Tool-specific kinds:`,
		`href="https://agnostic-ai.org/docs/spec-format/reviews/"`,
		`Compare targets`,
		`Clear filters`,
		`Related reference`,
		`href="https://agnostic-ai.org/docs/target-behavior/"`,
		`Native`,
		`Opt-in`,
		`Source only`,
		`No output`,
		`assets/scripts/capability-matrix.js`,
	} {
		if !strings.Contains(targets, required) {
			t.Errorf("targets guide is missing capability UI %q", required)
		}
	}
	for _, label := range []string{"Code review", "Dev setup"} {
		if strings.Contains(targets, ">"+label+"</a></th>") {
			t.Errorf("targets matrix still has a %s column; tool-specific kinds belong under the table", label)
		}
	}
	cursorPage := readBuiltFile(t, filepath.Join(outputDir, "docs", "targets", "cursor", "index.html"))
	if !strings.Contains(cursorPage, `class="docs-nav-sub"`) || !strings.Contains(cursorPage, `aria-current="page" href="https://agnostic-ai.org/docs/targets/cursor/">Cursor</a>`) {
		t.Error("a target page must list every target under Targets in the sidebar and mark itself current")
	}
	if strings.Contains(guide, `class="docs-nav-sub"`) {
		t.Error("pages outside the targets section must keep the target submenu collapsed")
	}
	if strings.Contains(targets, `class="target-grid"`) || strings.Contains(targets, `id="global-output"`) {
		t.Error("targets guide still contains the duplicated target directory or advanced output reference")
	}
	for _, required := range []string{
		`Cross-target behavior`,
		`id="entry-point-files"`,
		`id="memory-and-local-state"`,
		`id="global-output"`,
	} {
		if !strings.Contains(targetBehavior, required) {
			t.Errorf("cross-target guide is missing %q", required)
		}
	}
	for _, required := range []string{
		"Set up agnostic-ai with a coding agent",
		"Install and start",
		"Recommended installer",
		`data-installer-os="macos"`,
		`data-installer-os="windows"`,
		`data-installer-os="linux"`,
		"brew install --cask Chemaclass/tap/agnostic-ai",
		"agnostic-ai init --from all",
		"agnostic-ai sync --plan",
		"Keep the setup current.",
		"Read the latest briefing",
		`href="https://agnostic-ai.org/docs/targets/"`,
		`"installUrl": "https://agnostic-ai.org/#quickstart"`,
		"/docs/agent-setup/",
		"agnostic-ai agent setup",
		"/agent-setup.txt",
		"Why not just symlink one file?",
		`href="https://agnostic-ai.org/docs/why-agnostic-ai/"`,
	} {
		if !strings.Contains(normalizedHome, required) {
			t.Errorf("home page is missing %q", required)
		}
	}
	assertRenderedQuickstart(t, home)

	// The explorer has to read correctly with the script missing: both tab
	// lists ship inert rather than as buttons nothing is listening to, one
	// entry is open, and each entry shows exactly one generated file.
	for _, inert := range []string{`data-source-tablist inert`, `data-output-tablist inert`} {
		if !strings.Contains(normalizedHome, inert) {
			t.Errorf("the home page offers %q as controls before landing.js can wire them", inert)
		}
	}
	if !strings.Contains(normalizedHome, "assets/scripts/landing.js") {
		t.Error("the home page never loads the script that makes the explorer interactive")
	}
	entries := strings.Count(normalizedHome, "data-source-panel")
	if open := entries - strings.Count(normalizedHome, "data-source-panel hidden"); entries < 2 || open != 1 {
		t.Errorf("the explorer opens %d of %d entries at rest, want exactly 1", open, entries)
	}
	outputs := strings.Count(normalizedHome, "data-output-panel")
	if shown := outputs - strings.Count(normalizedHome, "data-output-panel hidden"); shown != entries {
		t.Errorf("the explorer shows %d generated files at rest across %d entries, want one per entry", shown, entries)
	}

	// The hero plays the explainer video through the same click-to-load
	// poster as the docs demo, so nothing loads from YouTube before a click.
	for _, required := range []string{
		`data-video-id="oHeKVk6bidA"`,
		`href="https://www.youtube.com/watch?v=oHeKVk6bidA"`,
		"assets/images/hero-video-poster.webp",
		"assets/scripts/video.js",
		`"@type": "VideoObject"`,
	} {
		if !strings.Contains(normalizedHome, required) {
			t.Errorf("home page is missing the hero video %q", required)
		}
	}
	if strings.Contains(home, "<iframe") {
		t.Error("home page loads the video player before the visitor asks for it")
	}
	// The talk demo lives at the foot of the documentation index.
	for _, moved := range []string{`id="demo"`, "demo-poster.webp"} {
		if strings.Contains(normalizedHome, moved) {
			t.Errorf("home page still carries the demo %q", moved)
		}
	}
	for _, poster := range []string{"demo-poster.webp", "hero-video-poster.webp"} {
		if _, err := os.Stat(filepath.Join(outputDir, "assets", "images", poster)); err != nil {
			t.Errorf("video poster is not published: %v", err)
		}
	}
	missionIndex := strings.Index(home, `id="mission"`)
	explorerIndex := strings.Index(home, `id="explorer"`)
	quickstartIndex := strings.Index(home, `id="quickstart"`)
	updatesIndex := strings.Index(home, `id="updates"`)
	if missionIndex < 0 || explorerIndex < 0 || quickstartIndex < 0 || updatesIndex < 0 || missionIndex >= explorerIndex || explorerIndex >= quickstartIndex || quickstartIndex >= updatesIndex {
		t.Errorf("home sections are not ordered mission, explorer, quickstart, updates: %d, %d, %d, %d", missionIndex, explorerIndex, quickstartIndex, updatesIndex)
	}
	for _, assetURL := range []string{
		"https://agnostic-ai.org/assets/styles/base.css",
		"https://agnostic-ai.org/assets/styles/landing.css",
		"https://agnostic-ai.org/assets/scripts/theme.js",
	} {
		if !strings.Contains(home, assetURL) {
			t.Errorf("home page is missing custom-domain asset %q", assetURL)
		}
	}
	if strings.Contains(home, "chemaclass.github.io/agnostic-ai") {
		t.Error("home page still references the legacy GitHub Pages project URL")
	}
	for _, required := range []string{
		"TL;DR · Paste into your coding agent",
		`data-copy aria-label="Copy agent setup prompt"`,
		"Follow https://agnostic-ai.org/agent-setup.txt exactly.",
		"You do not need to read the rest of this page",
	} {
		if !strings.Contains(normalizedAgentSetupGuide, required) {
			t.Errorf("agent setup guide is missing quick handoff %q", required)
		}
	}
	sharingHome := normalizedHome
	for _, metadata := range []string{
		`property="og:site_name" content="agnostic-ai.org"`,
		`property="og:url" content="https://agnostic-ai.org/"`,
		`property="og:image:secure_url" content="https://agnostic-ai.org/og-v2.png"`,
		`name="twitter:domain" content="agnostic-ai.org"`,
		`name="twitter:url" content="https://agnostic-ai.org/"`,
	} {
		if !strings.Contains(sharingHome, metadata) {
			t.Errorf("home page is missing sharing metadata %q", metadata)
		}
	}
}

func TestSiteDocs_BuildsSiteSearchIndex(t *testing.T) {
	zolaPath := lookupPinnedZola(t)

	outputDir := t.TempDir()
	command := exec.Command(zolaPath, "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build documentation site: %v\n%s", err, output)
	}

	var entries []struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Heading string `json:"heading"`
		Group   string `json:"group"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal([]byte(readBuiltFile(t, filepath.Join(outputDir, "search-index", "index.html"))), &entries); err != nil {
		t.Fatalf("search index is not valid JSON: %v", err)
	}
	if len(entries) < 200 {
		t.Errorf("search index has %d entries, want at least 200", len(entries))
	}

	bodies := map[string]string{}
	for _, entry := range entries {
		if entry.URL == "" || entry.Title == "" {
			t.Errorf("search entry lacks a URL or title: %+v", entry)
		}
		if entry.Group != "Docs" && entry.Group != "Targets" && entry.Group != "Updates" {
			t.Errorf("%s has unexpected group %q", entry.URL, entry.Group)
		}
		if _, seen := bodies[entry.URL]; seen {
			t.Errorf("search index repeats %s", entry.URL)
		}
		if strings.Contains(entry.Body, "<") || strings.Contains(entry.Body, "&quot;") {
			t.Errorf("%s body carries markup or an HTML entity that minification corrupts", entry.URL)
		}
		if entry.URL == "https://agnostic-ai.org/" || strings.Contains(entry.URL, "/playground/") || strings.Contains(entry.URL, "/search-index/") {
			t.Errorf("search index includes excluded page %s", entry.URL)
		}
		bodies[entry.URL] = entry.Body
	}
	for _, url := range []string{
		"https://agnostic-ai.org/docs/",
		"https://agnostic-ai.org/docs/targets/",
		"https://agnostic-ai.org/docs/targets/claude/",
		"https://agnostic-ai.org/docs/spec-format/hooks/",
		"https://agnostic-ai.org/docs/configuration/#verify",
		"https://agnostic-ai.org/updates/",
		"https://agnostic-ai.org/updates/2026-09-16-v0.59.0/",
	} {
		if _, ok := bodies[url]; !ok {
			t.Errorf("search index is missing %s", url)
		}
	}
	lefthook := bodies["https://agnostic-ai.org/docs/git-hooks/#lefthook"]
	husky := bodies["https://agnostic-ai.org/docs/git-hooks/#husky-lint-staged"]
	if strings.TrimSpace(lefthook) == "" || strings.TrimSpace(husky) == "" || lefthook == husky {
		t.Error("sibling sections on the git hooks guide do not get their own search text")
	}

	// A page entry indexes the page's first 3000 characters, not only the
	// text above its first heading, so a query naming the page reaches the
	// page itself rather than one of its sections.
	cursor := bodies["https://agnostic-ai.org/docs/targets/cursor/"]
	if !strings.Contains(cursor, ".cursor/rules") {
		t.Error("the cursor page entry does not carry text from below its first heading")
	}

	// Equal-scoring search results fall back to index order, so briefings
	// have to be indexed newest first for the newest one to win that tie.
	// These two published on the same day, which is the case that orders
	// by time rather than by date alone.
	later, earlier := -1, -1
	for i, entry := range entries {
		switch entry.URL {
		case "https://agnostic-ai.org/updates/2026-09-18-v0.61.0/":
			later = i
		case "https://agnostic-ai.org/updates/2026-09-18-v0.60.0/":
			earlier = i
		}
	}
	if later < 0 || earlier < 0 || later > earlier {
		t.Errorf("same-day briefings are not indexed newest first: v0.61.0 at %d, v0.60.0 at %d", later, earlier)
	}

	home := readBuiltFile(t, filepath.Join(outputDir, "index.html"))
	for _, required := range []string{
		"data-search-open",
		`id="site-search"`,
		`role="combobox"`,
		`data-search-index="https://agnostic-ai.org/search-index/"`,
		"assets/scripts/search.js",
		`class="theme-toggle"`,
	} {
		if !strings.Contains(home, required) {
			t.Errorf("home page is missing search UI %q", required)
		}
	}
}

func TestSiteDocs_LandingCopyAvoidsDashesAndExclamations(t *testing.T) {
	var landing map[string]any
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("parse landing data: %v", err)
	}
	// The docs section renders its own template copy out of frontmatter, the
	// way the updates section does, so the same house style applies to it.
	var docsSection map[string]any
	if _, err := toml.Decode(frontmatter(t, "../../docs/site/content/docs/_index.md"), &docsSection); err != nil {
		t.Fatalf("parse documentation section frontmatter: %v", err)
	}
	for _, source := range []map[string]any{landing, docsSection} {
		for _, violation := range landingCopyViolations(landingProse(source, "")) {
			t.Error(violation)
		}
	}
}

var (
	cssCommentRE       = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssReducedMotionRE = regexp.MustCompile(`^@media\b.*prefers-reduced-motion\s*:\s*reduce`)
	cssAnimationRE     = regexp.MustCompile(`(?:^|[;{\s])animation(?:-name)?\s*:\s*([^;}]*)`)
)

// TestSiteDocs_LandingMotionRestsWhenAsked reads the two stylesheets the
// landing page loads and requires every selector that starts an animation to
// be stopped again inside a `prefers-reduced-motion: reduce` block. The rest
// state itself was verified by screenshot when the hero was built; what this
// guards is the next animation somebody adds without one.
func TestSiteDocs_LandingMotionRestsWhenAsked(t *testing.T) {
	t.Parallel()

	animations := 0
	for _, path := range []string{
		"../../docs/site/static/assets/styles/base.css",
		"../../docs/site/static/assets/styles/landing.css",
	} {
		started, stopped := cssAnimationRules(readBuiltFile(t, path))
		animations += len(started)
		for _, selector := range started {
			if !stopped[selector] {
				t.Errorf("%s animates %s but never stops it under prefers-reduced-motion: reduce", path, selector)
			}
		}
	}
	// base.css keeps one animation, which proves the parser still finds them.
	if animations == 0 {
		t.Error("the landing stylesheets start no animation, so this guard is watching nothing")
	}
}

// cssAnimationRules walks a stylesheet and returns the selectors that start an
// animation outside a reduced-motion block, plus the set of selectors that set
// `animation: none` inside one. Keyframe steps are not selectors, so the
// blocks inside an `@keyframes` are skipped.
func cssAnimationRules(sheet string) ([]string, map[string]bool) {
	sheet = cssCommentRE.ReplaceAllString(sheet, "")
	started := []string{}
	stopped := map[string]bool{}
	var open []string
	var prelude strings.Builder
	for index := 0; index < len(sheet); index++ {
		switch sheet[index] {
		case ';':
			prelude.Reset()
		case '}':
			if len(open) > 0 {
				open = open[:len(open)-1]
			}
			prelude.Reset()
		case '{':
			head := strings.Join(strings.Fields(prelude.String()), " ")
			prelude.Reset()
			if strings.HasPrefix(head, "@") {
				open = append(open, head)
				continue
			}
			body, end := cssRuleBody(sheet, index)
			index = end
			if cssStackHas(open, "@keyframes") {
				continue
			}
			reduced := false
			for _, frame := range open {
				if cssReducedMotionRE.MatchString(frame) {
					reduced = true
				}
			}
			for _, match := range cssAnimationRE.FindAllStringSubmatch(body, -1) {
				none := strings.HasPrefix(strings.TrimSpace(match[1]), "none")
				for _, selector := range strings.Split(head, ",") {
					selector = strings.Join(strings.Fields(selector), " ")
					if selector == "" {
						continue
					}
					switch {
					case reduced && none:
						stopped[selector] = true
					case !reduced && !none:
						started = append(started, selector)
					}
				}
			}
		default:
			prelude.WriteByte(sheet[index])
		}
	}
	sort.Strings(started)
	return started, stopped
}

// cssRuleBody returns the declarations of the rule whose block opens at start,
// and the index of the closing brace.
func cssRuleBody(sheet string, start int) (string, int) {
	depth := 0
	for index := start; index < len(sheet); index++ {
		switch sheet[index] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return sheet[start+1 : index], index
			}
		}
	}
	return sheet[start+1:], len(sheet) - 1
}

func cssStackHas(open []string, prefix string) bool {
	for _, frame := range open {
		if strings.HasPrefix(frame, prefix) {
			return true
		}
	}
	return false
}

// landingCopyViolations reports every authored string that breaks the house
// style: no em dash, no en dash, no exclamation mark.
func landingCopyViolations(prose map[string]string) []string {
	violations := []string{}
	for path, copy := range prose {
		for _, forbidden := range []string{"\u2014", "\u2013", "!"} {
			if strings.Contains(copy, forbidden) {
				violations = append(violations, fmt.Sprintf("site copy at %s contains %q", path, forbidden))
			}
		}
	}
	sort.Strings(violations)
	return violations
}

// frontmatter returns the TOML block a Zola content file opens with.
func frontmatter(t *testing.T, path string) string {
	t.Helper()
	parts := strings.SplitN(readBuiltFile(t, path), "+++", 3)
	if len(parts) < 3 {
		t.Fatalf("%s has no TOML frontmatter", path)
	}
	return parts[1]
}

// landingProse flattens every authored string in the landing data, keyed by its
// dotted path. Generated file bodies live in explorer.toml, not here.
func landingProse(value any, path string) map[string]string {
	prose := map[string]string{}
	switch typed := value.(type) {
	case string:
		prose[path] = typed
	case []any:
		for index, item := range typed {
			for key, copy := range landingProse(item, fmt.Sprintf("%s[%d]", path, index)) {
				prose[key] = copy
			}
		}
	case map[string]any:
		for name, item := range typed {
			child := name
			if path != "" {
				child = path + "." + name
			}
			for key, copy := range landingProse(item, child) {
				prose[key] = copy
			}
		}
	}
	return prose
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
		"## What the agent does",
		"### Setup rules",
		"agnostic-ai init --from all",
		"agnostic-ai sync --check",
		"https://agnostic-ai.org/docs/installation/",
	} {
		if !strings.Contains(agentSetup, required) {
			t.Errorf("agent-setup.txt is missing %q", required)
		}
	}
	if strings.Contains(agentSetup, "You do not need to read the rest") {
		t.Error("agent-setup.txt carries the line meant for the person")
	}
	if !strings.Contains(fullDocs, "# Set up agnostic-ai with a coding agent") {
		t.Error("llms-full.txt does not include the agent setup guide")
	}
	for name, content := range map[string]string{"agent-setup.txt": agentSetup, "llms-full.txt": fullDocs} {
		if strings.Contains(content, "@/docs/") || strings.Contains(content, "\n+++\n") || strings.Contains(content, "agent_setup_prompt") {
			t.Errorf("%s exposes Zola-only source syntax", name)
		}
	}
}

func TestSiteDocs_FooterPublishesTheReleasedVersion(t *testing.T) {
	t.Parallel()

	var siteConfig struct {
		Extra struct {
			Version string `toml:"version"`
		} `toml:"extra"`
	}
	if _, err := toml.DecodeFile("../../docs/site/config.toml", &siteConfig); err != nil {
		t.Fatalf("parse site config: %v", err)
	}
	published := siteConfig.Extra.Version
	if published == "" {
		t.Fatal("the site config publishes no version")
	}

	binary := regexp.MustCompile(`var version = "([^"]+)"`).FindStringSubmatch(readBuiltFile(t, "../../cmd/agnostic-ai/main.go"))
	if binary == nil {
		t.Fatal("cmd/agnostic-ai/main.go declares no version")
	}
	if want := "v" + binary[1]; published != want {
		t.Errorf("footer version = %q, binary version = %q", published, want)
	}

	released := regexp.MustCompile(`(?m)^## (v\S+) - \d{4}-\d{2}-\d{2}$`).FindStringSubmatch(readBuiltFile(t, "../../CHANGELOG.md"))
	if released == nil {
		t.Fatal("CHANGELOG.md has no dated release section")
	}
	if published != released[1] {
		t.Errorf("footer version = %q, latest changelog section = %q", published, released[1])
	}

	project := readBuiltFile(t, "../../agnostic-ai.yaml")
	var pins struct {
		Requires string `yaml:"requires"`
	}
	if err := yaml.Unmarshal([]byte(project), &pins); err != nil {
		t.Fatalf("parse project config: %v", err)
	}
	releasedMinimum, err := config.ParseRequirement(">=" + strings.TrimPrefix(published, "v"))
	if err != nil {
		t.Fatal(err)
	}
	if allowed, _ := releasedMinimum.Allows(pins.Requires); !allowed {
		t.Errorf("project requires = %q, want an exact pin at least %s", pins.Requires, published)
	}
	schema := regexp.MustCompile(`(?m)^# yaml-language-server: \$schema=(\S+)`).FindStringSubmatch(project)
	wantSchema := "https://raw.githubusercontent.com/Chemaclass/agnostic-ai/" + published + "/docs/schemas/config.schema.json"
	if schema == nil || schema[1] != wantSchema {
		t.Errorf("project schema directive must pin %s", wantSchema)
	}

	footer := readBuiltFile(t, "../../docs/site/templates/base.html")
	if !strings.Contains(footer, `{{ config.extra.repository_url }}/releases/tag/{{ config.extra.version }}`) {
		t.Error("the footer does not link its version to the matching GitHub release")
	}
}

// TestSiteDocs_EveryLandingInstallerReachesThePage keeps `workflow.installers`
// from collecting routes no page renders.
//
// The installation page's picker renders every entry as a tab, and the
// landing hero pins its command to the entry whose id is `script`. An entry
// once sat in this list while the built site came out byte-identical (#991),
// so the test checks the picker is still on the page and every entry carries
// what the picker shows.
func TestSiteDocs_EveryLandingInstallerReachesThePage(t *testing.T) {
	var landing struct {
		Workflow struct {
			Installers []struct {
				ID      string `toml:"id"`
				Tab     string `toml:"tab"`
				Label   string `toml:"label"`
				Command string `toml:"command"`
			} `toml:"installers"`
		} `toml:"workflow"`
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode landing data: %v", err)
	}
	if len(landing.Workflow.Installers) == 0 {
		t.Fatal("no installers in the landing data")
	}

	installation := readRepoFile(t, "docs/site/content/docs/installation.md")
	if !strings.Contains(installation, "{{ <install_picker /> }}") {
		t.Fatal("the installation page no longer renders the installer picker, so installers without recommend_for reach no page")
	}
	picker := readRepoFile(t, "docs/site/templates/components/install_picker.html")
	if !strings.Contains(picker, "for installer in landing.workflow.installers") {
		t.Fatal("the installer picker no longer iterates workflow.installers; update this test to match")
	}

	// Read the hero's default out of the template rather than repeating it,
	// so renaming it there fails here instead of silently orphaning an entry.
	index := readRepoFile(t, "docs/site/templates/index.html")
	defaultID := regexp.MustCompile(`installer\.id == "([a-z0-9-]+)"`).FindStringSubmatch(index)
	if defaultID == nil {
		t.Fatal("index.html no longer pins the hero command to an installer id; update this test to match")
	}

	var seenDefault bool
	for _, installer := range landing.Workflow.Installers {
		if installer.ID == defaultID[1] {
			seenDefault = true
		}
		if installer.Tab == "" || installer.Label == "" || installer.Command == "" {
			t.Errorf("installer %q needs a tab, label, and command for the installation page picker", installer.ID)
		}
	}
	if !seenDefault {
		t.Errorf("no installer has id %q, which index.html pins the hero command to", defaultID[1])
	}
}

// TestSiteDocs_TocEscapesHeadingTitles keeps a heading's literal markup out
// of the page's own DOM. Zola hands the table of contents plain-text titles,
// so the heading "`x-<target>` namespace" arrives as `x-<target>`; rendered
// unescaped, the browser opened a `<target>` element that the minified live
// build never closed, and it swallowed the rest of the page, site footer
// included, into the right-hand TOC column.
func TestSiteDocs_TocEscapesHeadingTitles(t *testing.T) {
	zolaPath := lookupPinnedZola(t)

	outputDir := t.TempDir()
	command := exec.Command(zolaPath, "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build documentation site: %v\n%s", err, output)
	}

	tocBlock := regexp.MustCompile(`(?s)<aside class="docs-toc">.*?</aside>`)
	allowedTag := regexp.MustCompile(`</?(?:p|nav|a)\b[^>]*>`)
	pages, err := filepath.Glob(filepath.Join(outputDir, "docs", "*", "index.html"))
	if err != nil || len(pages) == 0 {
		t.Fatalf("no built docs pages under %s: %v", outputDir, err)
	}
	topicPages, err := filepath.Glob(filepath.Join(outputDir, "docs", "*", "*", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	pages = append(pages, topicPages...)
	for _, page := range pages {
		toc := tocBlock.FindString(readBuiltFile(t, page))
		if toc == "" {
			continue
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(toc, `<aside class="docs-toc">`), "</aside>")
		if stray := allowedTag.ReplaceAllString(inner, ""); strings.Contains(stray, "<") {
			t.Errorf("%s: TOC emits raw markup from a heading title:\n%s", page, stray)
		}
	}

	specFormat := tocBlock.FindString(readBuiltFile(t, filepath.Join(outputDir, "docs", "spec-format", "index.html")))
	if !strings.Contains(specFormat, "x-&lt;target&gt;") {
		t.Errorf("spec-format TOC does not show the escaped `x-<target>` heading:\n%s", specFormat)
	}
}

// TestSiteDocs_HeaderKeepsOnlySiteNavigation pins the header to the pages
// of this site plus search, the GitHub link, and the theme toggle, in one
// order across the Zola shell and the hand-written playground page. The
// release version lives in the footer; repeating it up top was noise.
func TestSiteDocs_HeaderKeepsOnlySiteNavigation(t *testing.T) {
	t.Parallel()

	header := regexp.MustCompile(`(?s)<header class="site-header">.*?</header>`)
	label := regexp.MustCompile(`>(Home|Docs|Updates|Playground)</a>`)
	for _, path := range []string{"../../docs/site/templates/base.html", "../../docs/playground/index.html"} {
		block := header.FindString(readBuiltFile(t, path))
		if block == "" {
			t.Fatalf("%s: no site header", path)
		}
		var order []string
		for _, m := range label.FindAllStringSubmatch(block, -1) {
			order = append(order, m[1])
		}
		if got, want := strings.Join(order, ","), "Home,Docs,Updates,Playground"; got != want {
			t.Errorf("%s: nav order = %s, want %s", path, got, want)
		}
		search := strings.Index(block, "data-search-open")
		github := strings.Index(block, `class="header-icon-link"`)
		theme := strings.Index(block, `class="theme-toggle"`)
		if search < 0 || github < 0 || theme < 0 || search > github || github > theme {
			t.Errorf("%s: header must end with search, the GitHub link, then the theme toggle", path)
		}
		for _, noise := range []string{"brand-version", "config.extra.version"} {
			if strings.Contains(block, noise) {
				t.Errorf("%s: header still carries %q; the footer already shows it", path, noise)
			}
		}
	}
}
