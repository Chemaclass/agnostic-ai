"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  filterEditions,
  matchesEdition,
  normalizeTargets,
  parseURL,
  serializeURL
} = require("./updates.js");

const editions = [
  { id: "claude", targets: ["claude"], search: "Claude Code manual skills" },
  { id: "codex", targets: ["codex"], search: "Codex CLI nested skills" },
  { id: "multi", targets: ["claude", "codex"], search: "Shared hooks and skills" },
  { id: "other", targets: ["gemini"], search: "Gemini CLI hooks" },
  { id: "general", targets: [], search: "General release notes" }
];

test("target filters use OR while query terms use AND", function () {
  assert.deepEqual(
    filterEditions(editions, { targets: ["claude", "codex"], query: "skills" }).map(function (edition) { return edition.id; }),
    ["claude", "codex", "multi"]
  );
  assert.deepEqual(
    filterEditions(editions, { targets: ["claude", "codex"], query: "shared skills" }).map(function (edition) { return edition.id; }),
    ["multi"]
  );
});

test("empty filters include general editions and unknown targets do not broaden", function () {
  assert.equal(filterEditions(editions, { targets: [], query: "" }).length, editions.length);
  assert.equal(matchesEdition(editions[4], { targets: ["claude"], query: "" }), false);
  assert.equal(filterEditions(editions, { targets: ["unknown"], query: "" }).length, 0);
});

test("matching is case-insensitive and collapses query whitespace", function () {
  assert.equal(matchesEdition(editions[0], { targets: ["CLAUDE", "claude"], query: "  MANUAL\n skills " }), true);
  assert.deepEqual(normalizeTargets([" codex ", "claude", "codex", ""]), ["claude", "codex"]);
});

test("URL state deduplicates targets and preserves unrelated parameters", function () {
  const current = "https://example.com/updates/?view=compact&target=old&q=old#top";
  const url = serializeURL(current, { targets: ["codex", "claude", "codex"], query: " skills  hooks " });
  assert.equal(url.href, "https://example.com/updates/?view=compact&target=claude&target=codex&q=skills+hooks#archive-title");
  assert.deepEqual(parseURL(url), { targets: ["claude", "codex"], query: "skills hooks" });
});

test("URL encoding round trips target and search values", function () {
  const url = serializeURL("https://example.com/updates/", { targets: ["claude code"], query: "MCP & skills" });
  assert.match(url.search, /target=claude\+code/);
  assert.match(url.search, /q=MCP\+%26\+skills/);
  assert.deepEqual(parseURL(url), { targets: ["claude code"], query: "MCP & skills" });
});
