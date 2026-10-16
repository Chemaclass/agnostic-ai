package integration

import (
	"encoding/json"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
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

	for _, text := range []string{"Understand the setup", "/docs/why-agnostic-ai/", "Set up your project"} {
		if !strings.Contains(index, text) {
			t.Errorf("documentation entry has no beginner route %q", text)
		}
	}
	for _, text := range []string{"Project instructions", "MCP connection", "What to add, and when", "When you do not need it"} {
		if !strings.Contains(intro, text) {
			t.Errorf("introduction does not explain %q", text)
		}
	}
	for _, text := range []string{"CLAUDE.md", "AGENTS.md", `id="with-your-coding-agent"`, `id="in-your-terminal"`, `id="daily-use"`, "data-install-picker", "Copy agent setup prompt", "aria-label=\"Next lesson\""} {
		if !strings.Contains(guide, text) {
			t.Errorf("first setup is missing %q", text)
		}
	}
	proofStart := strings.Index(home, `id="first-sync"`)
	explorerStart := strings.Index(home, `id="source-explorer"`)
	if proofStart < 0 || explorerStart < 0 || proofStart >= explorerStart {
		t.Fatal("first-sync proof must precede the full source explorer")
	}
	proof := home[proofStart : strings.Index(home[proofStart:], "</section>")+proofStart]
	for _, text := range []string{".agnostic-ai/AGNOSTIC_AI.md", "CLAUDE.md", "AGENTS.md", "agnostic-ai sync"} {
		if !strings.Contains(proof, text) {
			t.Errorf("static first-sync proof is missing %q", text)
		}
	}
	if strings.Contains(proof, " hidden") || strings.Contains(proof, "data-source-panel") {
		t.Error("first-sync proof must show its source and both outputs without JavaScript")
	}
	assertFirstSyncProof(t, proof)
	if !strings.Contains(home[:proofStart], `class="button button-primary" href="https://agnostic-ai.org/docs/getting-started/"`) {
		t.Error("homepage primary action must open the setup guide")
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

func assertFirstSyncProof(t *testing.T, proof string) {
	t.Helper()
	var landing struct {
		Proof struct{ Note string }
	}
	if _, err := toml.DecodeFile("../../docs/site/data/landing.toml", &landing); err != nil {
		t.Fatalf("decode first-sync example: %v", err)
	}
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	t.Setenv("AGNOSTIC_AI_HOME", filepath.Join(dir, "global"))
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"agnostic-ai.yaml":            "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/AGNOSTIC_AI.md": landing.Proof.Note + "\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	command := cli.NewRootCmd("test")
	command.SetArgs([]string{"sync", "--gitignore=off"})
	command.SetOut(io.Discard)
	command.SetErr(io.Discard)
	if err := command.Execute(); err != nil {
		t.Fatalf("sync first-note example: %v", err)
	}
	shown := map[string]string{}
	for _, match := range regexp.MustCompile(`(?s)<div class="file-name">([^<]+)</div>\s*<pre><code>(.*?)</code></pre>`).FindAllStringSubmatch(proof, -1) {
		shown[html.UnescapeString(match[1])] = strings.TrimSpace(html.UnescapeString(match[2]))
	}
	for _, path := range []string{".agnostic-ai/AGNOSTIC_AI.md", "CLAUDE.md", "AGENTS.md"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read first-note output %s: %v", path, err)
		}
		if want := strings.TrimSpace(withoutProvenanceBanner(string(body))); shown[path] != want {
			t.Errorf("homepage shows %s as %q, sync writes %q", path, shown[path], want)
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
