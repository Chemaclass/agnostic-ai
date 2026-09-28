"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  chooseTarget,
  compareRows,
  parseSelection,
  resultText,
  serializeSelection,
  summarize
} = require("./compare-targets.js");

const data = {
  states: [
    { id: "native", label: "Native" },
    { id: "mapped", label: "Mapped" },
    { id: "opt-in", label: "Opt-in" },
    { id: "source", label: "Source only" },
    { id: "no", label: "No output" }
  ],
  features: [
    { id: "agent", label: "Agents", href: "/docs/spec-format/#agents" },
    { id: "command", label: "Commands", href: "/docs/spec-format/#commands" },
    { id: "review", label: "Code review", href: "/docs/spec-format/#reviews" },
    { id: "ignore", label: "Ignored files", href: "/docs/spec-format/#ignore" }
  ],
  targets: [
    { id: "claude", name: "Claude Code", href: "/docs/targets/claude/", statuses: ["native", "native", "no", "no"], paths: [[".claude/agents/a.md"], [".claude/commands/c.md"], [], []] },
    { id: "codex", name: "Codex", href: "/docs/targets/codex/", statuses: ["native", "opt-in", "no", "no"], paths: [[".codex/agents/a.toml"], [], [], []] },
    { id: "cursor", name: "Cursor", href: "/docs/targets/cursor/", statuses: ["native", "native", "native", "native"], paths: [["a"], ["b"], ["c"], ["d"]] },
    { id: "aider", name: "Aider", href: "/docs/targets/aider/", statuses: ["source", "no", "no", "native"], paths: [[], [], [], [".aiderignore"]] }
  ]
};
const known = data.targets.map(function (target) { return target.id; });

test("the URL picks two or three distinct targets", function () {
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=cursor&b=aider", known), { ids: ["cursor", "aider"], unknown: [] });
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=CODEX&b=claude&c=aider", known), { ids: ["codex", "claude", "aider"], unknown: [] });
});

test("a missing or unknown target falls back to the default pair and is reported", function () {
  assert.deepEqual(parseSelection("https://example.com/docs/compare/", known), { ids: ["claude", "codex"], unknown: [] });
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=nope&b=cursor", known), { ids: ["claude", "cursor"], unknown: ["nope"] });
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=codex", known), { ids: ["codex", "claude"], unknown: [] });
});

test("a repeated target never fills two columns", function () {
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=codex&b=codex&c=codex", known).ids, ["codex", "claude"]);
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=claude&b=codex&c=claude", known).ids, ["claude", "codex"]);
});

test("the URL keeps other parameters and drops c when two targets remain", function () {
  const three = serializeSelection("https://example.com/docs/compare/?ref=nav&a=old#top", ["codex", "cursor", "aider"]);
  assert.equal(three.href, "https://example.com/docs/compare/?ref=nav&a=codex&b=cursor&c=aider#top");
  const two = serializeSelection(three, ["codex", "cursor"]);
  assert.equal(two.search, "?ref=nav&a=codex&b=cursor");
  assert.deepEqual(parseSelection(two, known).ids, ["codex", "cursor"]);
});

test("choosing a target shown in another column swaps the two", function () {
  assert.deepEqual(chooseTarget(["claude", "codex"], 0, "codex"), ["codex", "claude"]);
  assert.deepEqual(chooseTarget(["claude", "codex", "cursor"], 2, "claude"), ["cursor", "codex", "claude"]);
  assert.deepEqual(chooseTarget(["claude", "codex"], 1, "aider"), ["claude", "aider"]);
});

test("the third column is optional and never repeats another column", function () {
  assert.deepEqual(chooseTarget(["claude", "codex"], 2, "cursor"), ["claude", "codex", "cursor"]);
  assert.deepEqual(chooseTarget(["claude", "codex", "cursor"], 2, ""), ["claude", "codex"]);
  assert.deepEqual(chooseTarget(["claude", "codex"], 2, "claude"), ["claude", "codex"]);
  assert.deepEqual(chooseTarget(["claude", "codex"], 0, ""), ["claude", "codex"]);
});

test("a row differs when any state differs, not when only paths differ", function () {
  const rows = compareRows(data, ["claude", "codex"]);
  assert.deepEqual(rows.map(function (row) { return row.differs; }), [false, true, false, false]);
  assert.deepEqual(rows[0].cells.map(function (cell) { return cell.paths; }), [[".claude/agents/a.md"], [".codex/agents/a.toml"]]);
  assert.equal(resultText(rows), "Claude Code and Codex: 1 of 4 spec kinds differ.");
  assert.equal(resultText(compareRows(data, ["claude", "codex", "aider"])), "Claude Code, Codex, and Aider: 3 of 4 spec kinds differ.");
});

test("the summary separates default output, opt-in, and gaps", function () {
  const report = summarize(data, ["claude", "codex"]);
  const ids = function (features) { return features.map(function (feature) { return feature.id; }); };
  assert.deepEqual(ids(report.targets[0].only), ["command"]);
  assert.deepEqual(ids(report.targets[1].only), []);
  assert.deepEqual(ids(report.targets[1].optIn), ["command"]);
  assert.deepEqual(ids(report.targets[1].missing), []);
  assert.deepEqual(ids(report.none), ["review", "ignore"]);
});

test("with three targets, only means no other column writes it by default", function () {
  const report = summarize(data, ["claude", "cursor", "aider"]);
  const ids = function (features) { return features.map(function (feature) { return feature.id; }); };
  assert.deepEqual(ids(report.targets[0].only), []);
  assert.deepEqual(ids(report.targets[1].only), ["review"]);
  assert.deepEqual(ids(report.targets[2].missing), ["agent", "command", "review"]);
  assert.deepEqual(ids(report.targets[0].missing), ["review", "ignore"]);
  assert.deepEqual(ids(report.none), []);
});
