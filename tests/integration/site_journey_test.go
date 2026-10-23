package integration

import (
	"encoding/json"
	"html"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSiteDocs_BeginnerJourneyAndMovedTopics(t *testing.T) {
	outputDir := t.TempDir()
	command := exec.Command(lookupPinnedZola(t), "--root", "../../docs/site", "build", "--force", "--output-dir", outputDir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build beginner journey: %v\n%s", err, output)
	}

	index := readBuiltFile(t, filepath.Join(outputDir, "docs", "index.html"))
	home := readBuiltFile(t, filepath.Join(outputDir, "index.html"))
	home = strings.ReplaceAll(home, "&#x2F;", "/")
	intro := readBuiltFile(t, filepath.Join(outputDir, "docs", "why-agnostic-ai", "index.html"))
	guide := readBuiltFile(t, filepath.Join(outputDir, "docs", "getting-started", "index.html"))
	guide = strings.ReplaceAll(guide, "&#x2F;", "/")
	memory := readBuiltFile(t, filepath.Join(outputDir, "docs", "memory", "index.html"))

	for _, text := range []string{"Why agnostic-ai?", "/docs/why-agnostic-ai/", "Set up your project"} {
		if !strings.Contains(index, text) {
			t.Errorf("documentation entry has no beginner route %q", text)
		}
	}
	for _, text := range []string{"Project instructions", "MCP connection", "What to add, and when", "When you do not need it"} {
		if !strings.Contains(intro, text) {
			t.Errorf("introduction does not explain %q", text)
		}
	}
	for _, text := range []string{"CLAUDE.md", "AGENTS.md", `id="install-agnostic-ai"`, `id="daily-use"`, "data-install-picker", `href="https://agnostic-ai.org/docs/agent-setup/"`, "aria-label=\"Next lesson\""} {
		if !strings.Contains(guide, text) {
			t.Errorf("first setup is missing %q", text)
		}
	}
	// The agent prompt lives on its own page; a second copy here would drift.
	if strings.Contains(guide, "Copy agent setup prompt") {
		t.Error("setup guide must link to the agent setup page instead of repeating its prompt")
	}
	proofStart := strings.Index(home, `id="first-sync"`)
	explorerStart := strings.Index(home, `id="source-explorer"`)
	if proofStart < 0 || explorerStart < 0 || proofStart >= explorerStart {
		t.Fatal("first-sync proof must precede the full source explorer")
	}
	proof := home[proofStart : strings.Index(home[proofStart:], "</section>")+proofStart]
	if !strings.Contains(proof, "agnostic-ai sync") {
		t.Error("static first-sync proof is missing the sync command")
	}
	if strings.Contains(proof, " hidden") || strings.Contains(proof, "data-source-panel") {
		t.Error("first-sync proof must show its source and every output without JavaScript")
	}
	assertFirstSyncProof(t, proof)
	if !strings.Contains(home[:proofStart], `class="button button-primary" href="https://agnostic-ai.org/docs/agent-setup/"`) {
		t.Error("homepage primary action must open the agent setup page")
	}
	for _, text := range []string{"<details", "How each tool loads it", "Save and recall a fact", `parent.open = true`, `addEventListener("hashchange"`} {
		if !strings.Contains(memory, text) {
			t.Errorf("memory detail or direct-fragment access is missing %q", text)
		}
	}
	payload, err := json.Marshal(map[string]string{"guide": guide, "memory": memory})
	if err != nil {
		t.Fatalf("encode rendered journey: %v", err)
	}
	behavior := exec.Command("node", "-e", journeyFragmentChecks)
	behavior.Stdin = strings.NewReader(string(payload))
	if output, err := behavior.CombinedOutput(); err != nil {
		t.Fatalf("check rendered fragment behavior: %v\n%s", err, output)
	}

}

// assertFirstSyncProof checks the hero renders landing.toml as written.
// TestSiteDocs_LandingHeroMatchesSync checks those lines against a real sync.
func assertFirstSyncProof(t *testing.T, proof string) {
	t.Helper()
	fanout := loadHeroFanout(t)
	source := regexp.MustCompile(`(?s)<div class="file-name">([^<]+)</div>\s*<pre><code>(.*?)</code></pre>`).FindStringSubmatch(proof)
	if source == nil || html.UnescapeString(source[1]) != fanout.Source || html.UnescapeString(source[2]) != fanout.Line {
		t.Errorf("homepage does not show %s with the line %q", fanout.Source, fanout.Line)
	}
	rows := regexp.MustCompile(`<li style="--i: \d+"( class="is-drifted")?><span class="fanout-head"><code class="fanout-path">([^<]+)</code>.*?<code class="fanout-line">([^<]*)</code><code class="fanout-drift">([^<]*)</code></li>`).FindAllStringSubmatch(proof, -1)
	if len(rows) != len(fanout.Outputs) {
		t.Fatalf("homepage shows %d generated files, landing.toml lists %d", len(rows), len(fanout.Outputs))
	}
	for index, row := range rows {
		want := fanout.Outputs[index]
		path, line, drift := html.UnescapeString(row[2]), html.UnescapeString(row[3]), html.UnescapeString(row[4])
		if path != want.Path || line != want.Line || drift != want.Drift {
			t.Errorf("homepage card %d shows %s %q / %q, landing.toml has %s %q / %q", index, path, line, drift, want.Path, want.Line, want.Drift)
		}
		if drifted := row[1] != ""; drifted != (want.Drift != want.Line) {
			t.Errorf("homepage card %s marks drift %v, want %v", want.Path, drifted, want.Drift != want.Line)
		}
	}
}

const journeyFragmentChecks = `
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const pages = JSON.parse(fs.readFileSync(0, "utf8"));
function script(html) {
  const match = [...html.matchAll(/<script>([\s\S]*?)<\/script>/g)]
    .find(item => item[1].includes("function revealTarget()"));
  assert.ok(match, "page has no fragment handler");
  return match[1];
}
function run(html, hash, target) {
  let replaced;
  let lookups = 0;
  const handlers = {};
  const window = {
    location: { hash, replace(url) { replaced = url; } },
    addEventListener(event, handler) { handlers[event] = handler; }
  };
  vm.runInNewContext(script(html), {
    window,
    document: { getElementById() { lookups++; return target; } }
  });
  return { window, handlers, destination: () => replaced, lookups: () => lookups };
}
for (const [fragment, expected] of [
  ["shell-completion", "https://agnostic-ai.org/docs/cli-reference/maintain/#completion"],
  ["roll-back-a-sync", "https://agnostic-ai.org/docs/cli-reference/maintain/#revert"],
  ["upgrade", "https://agnostic-ai.org/docs/installation/#upgrade"]
]) {
  const legacy = { parentElement: null, scrollIntoView() { assert.fail("scrolled to a legacy marker"); } };
  const result = run(pages.guide, "#" + fragment, legacy);
  assert.equal(result.destination(), expected, "moved fragment must win over an existing DOM ID");
  assert.equal(result.lookups(), 0);
}
const outer = { tagName: "DETAILS", open: false, parentElement: null };
const inner = { tagName: "DETAILS", open: false, parentElement: outer };
let scrolls = 0;
const heading = { parentElement: inner, scrollIntoView() { scrolls++; } };
const revealed = run(pages.memory, "#how-each-tool-loads-it", heading);
assert.equal(inner.open, true);
assert.equal(outer.open, true);
assert.equal(scrolls, 1);
inner.open = false;
outer.open = false;
revealed.window.location.hash = "#one-store-per-repository";
revealed.handlers.hashchange();
assert.equal(inner.open, true);
assert.equal(outer.open, true);
assert.equal(scrolls, 2);
const malformed = run(pages.guide, "#%E0%A4%A", heading);
assert.equal(malformed.destination(), undefined);
assert.equal(malformed.lookups(), 0);
`
