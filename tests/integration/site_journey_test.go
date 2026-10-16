package integration

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
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
	intro := readBuiltFile(t, filepath.Join(outputDir, "docs", "why-agnostic-ai", "index.html"))
	guide := readBuiltFile(t, filepath.Join(outputDir, "docs", "getting-started", "index.html"))
	guide = strings.ReplaceAll(guide, "&#x2F;", "/")
	memory := readBuiltFile(t, filepath.Join(outputDir, "docs", "memory", "index.html"))

	for _, text := range []string{"Understand the setup", "/docs/why-agnostic-ai/", "Build your first setup"} {
		if !strings.Contains(index, text) {
			t.Errorf("documentation entry has no beginner route %q", text)
		}
	}
	for _, text := range []string{"Project instructions", "MCP connection", "What to add, and when", "When you do not need it"} {
		if !strings.Contains(intro, text) {
			t.Errorf("introduction does not explain %q", text)
		}
	}
	for _, text := range []string{"CLAUDE.md", "AGENTS.md", ".claude/rules/conventional-commits.md", "aria-label=\"Next lesson\""} {
		if !strings.Contains(guide, text) {
			t.Errorf("first setup is missing %q", text)
		}
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
