"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  dismissesDropdown,
  filterEditions,
  matchesEdition,
  normalizeTargets,
  paginate,
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
  assert.deepEqual(parseURL(url), { targets: ["claude", "codex"], query: "skills hooks", page: 1 });
});

test("URL encoding round trips target and search values", function () {
  const url = serializeURL("https://example.com/updates/", { targets: ["claude code"], query: "MCP & skills" });
  assert.match(url.search, /target=claude\+code/);
  assert.match(url.search, /q=MCP\+%26\+skills/);
  assert.deepEqual(parseURL(url), { targets: ["claude code"], query: "MCP & skills", page: 1 });
});

test("the target dropdown closes on a click outside and stays open inside", function () {
  const inside = { id: "checkbox" };
  const details = { open: true, contains: function (node) { return node === inside; } };
  assert.equal(dismissesDropdown(details, {}), true);
  assert.equal(dismissesDropdown(details, inside), false);
  assert.equal(dismissesDropdown(details, null), true);
  details.open = false;
  assert.equal(dismissesDropdown(details, {}), false);
  assert.equal(dismissesDropdown(null, {}), false);
});

test("pages slice the matches and clamp an out-of-range page", function () {
  assert.deepEqual(paginate(13, 1, 6), { page: 1, pages: 3, start: 0, end: 6 });
  assert.deepEqual(paginate(13, 3, 6), { page: 3, pages: 3, start: 12, end: 13 });
  assert.deepEqual(paginate(13, 9, 6), { page: 3, pages: 3, start: 12, end: 13 });
  assert.deepEqual(paginate(13, "x", 6), { page: 1, pages: 3, start: 0, end: 6 });
  assert.deepEqual(paginate(0, 2, 6), { page: 1, pages: 1, start: 0, end: 0 });
});

test("the page travels in the URL beside the filters and page 1 stays implicit", function () {
  const url = serializeURL("https://example.com/updates/?page=4", { targets: ["codex"], query: "hooks", page: 2 });
  assert.equal(url.search, "?target=codex&q=hooks&page=2");
  assert.deepEqual(parseURL(url), { targets: ["codex"], query: "hooks", page: 2 });
  assert.equal(serializeURL(url, { targets: [], query: "", page: 1 }).search, "");
  assert.equal(parseURL("https://example.com/updates/?page=-3").page, 1);
});
