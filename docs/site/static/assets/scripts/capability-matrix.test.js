"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  filterTargets,
  matchesTarget,
  normalizeTargets,
  parseURL,
  serializeURL
} = require("./capability-matrix.js");

const targets = [
  { id: "claude", search: "Claude Code claude" },
  { id: "codex", search: "Codex codex" },
  { id: "gemini", search: "Gemini CLI gemini" }
];

test("target selection and search narrow the matrix together", function () {
  assert.deepEqual(
    filterTargets(targets, { targets: ["claude", "codex"], query: "claude" }).map(function (target) { return target.id; }),
    ["claude"]
  );
  assert.deepEqual(
    filterTargets(targets, { targets: [], query: "cli" }).map(function (target) { return target.id; }),
    ["gemini"]
  );
});

test("empty selection shows all targets and unknown targets do not broaden", function () {
  assert.equal(filterTargets(targets, { targets: [], query: "" }).length, targets.length);
  assert.equal(matchesTarget(targets[0], { targets: ["unknown"], query: "" }), false);
  assert.equal(filterTargets(targets, { targets: ["unknown"], query: "" }).length, 0);
});

test("matching is case-insensitive and target selection is deduplicated", function () {
  assert.equal(matchesTarget(targets[0], { targets: ["CLAUDE", "claude"], query: "  CLAUDE\n CODE " }), true);
  assert.deepEqual(normalizeTargets([" codex ", "claude", "codex", ""]), ["claude", "codex"]);
});

test("URL state preserves unrelated parameters and points to the matrix", function () {
  const current = "https://example.com/docs/targets/?view=compact&target=old&q=old#top";
  const url = serializeURL(current, { targets: ["codex", "claude", "codex"], query: " code  tools " });
  assert.equal(url.href, "https://example.com/docs/targets/?view=compact&target=claude&target=codex&q=code+tools#capability-matrix");
  assert.deepEqual(parseURL(url), { targets: ["claude", "codex"], query: "code tools" });
});

test("URL encoding round trips target and search values", function () {
  const url = serializeURL("https://example.com/docs/targets/", { targets: ["claude code"], query: "MCP & skills" });
  assert.match(url.search, /target=claude\+code/);
  assert.match(url.search, /q=MCP\+%26\+skills/);
  assert.deepEqual(parseURL(url), { targets: ["claude code"], query: "MCP & skills" });
});
