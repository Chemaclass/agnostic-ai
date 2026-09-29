"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  chooseTarget,
  takenElsewhere,
  compareRows,
  parseSelection,
  pathFormat,
  pathFormats,
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
    { id: "agent", label: "Agents", href: "/docs/spec-format/agent-specs/#agents" },
    { id: "command", label: "Commands", href: "/docs/spec-format/skills-rules-commands/#commands" },
    { id: "review", label: "Code review", href: "/docs/spec-format/settings/#reviews" },
    { id: "ignore", label: "Ignored files", href: "/docs/spec-format/settings/#ignore" }
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
  assert.deepEqual(parseSelection("https://example.com/docs/compare/?a=Nope&b=nope", known), { ids: ["claude", "codex"], unknown: ["Nope"] });
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

test("a column cannot pick a target another column shows", function () {
  assert.deepEqual(takenElsewhere(["claude", "codex"], 0), ["codex"]);
  assert.deepEqual(takenElsewhere(["claude", "codex"], 2), ["claude", "codex"]);
  assert.deepEqual(takenElsewhere(["claude", "codex", "cursor"], 1), ["claude", "cursor"]);
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
  assert.equal(resultText(rows), "Claude Code and Codex: 1 of 4 spec kinds differ in support, 1 in file format.");
  assert.equal(resultText(compareRows(data, ["claude", "codex", "aider"])), "Claude Code, Codex, and Aider: 3 of 4 spec kinds differ in support, 0 in file format.");
});

test("a path's extension names its file format", function () {
  assert.equal(pathFormat(".claude/agents/a.md"), "Markdown");
  assert.equal(pathFormat(".cursor/rules/a.mdc"), "Markdown");
  assert.equal(pathFormat(".codex/config.toml"), "TOML");
  assert.equal(pathFormat(".mcp.json"), "JSON");
  assert.equal(pathFormat("a/b.YAML"), "YAML");
  assert.equal(pathFormat("a/b.yml"), "YAML");
  assert.equal(pathFormat(".vscode/settings.jsonc"), "jsonc");
  assert.equal(pathFormat(".aiignore"), "");
  assert.equal(pathFormat("bin/setup"), "");
});

test("a cell lists each format once, in path order", function () {
  assert.deepEqual(pathFormats([".junie/AGENTS.md", "AGENTS.md"]), ["Markdown"]);
  assert.deepEqual(pathFormats(["a.toml", ".aiignore", "b.md", "c.toml"]), ["TOML", "Markdown"]);
  assert.deepEqual(pathFormats([]), []);
});

test("a row differs in format only when the states agree and the formats do not", function () {
  const formats = {
    states: data.states,
    features: [
      { id: "agent", label: "Agents", href: "#" },
      { id: "rule", label: "Rules", href: "#" },
      { id: "command", label: "Commands", href: "#" },
      { id: "ignore", label: "Ignored files", href: "#" }
    ],
    targets: [
      { id: "one", name: "One", href: "#", statuses: ["native", "native", "native", "native"], paths: [["a.md"], ["x.md", "y.md"], ["c.md"], [".oneignore"]] },
      { id: "two", name: "Two", href: "#", statuses: ["native", "native", "opt-in", "native"], paths: [["a.toml"], ["z.md"], [], [".twoignore"]] },
      { id: "three", name: "Three", href: "#", statuses: ["native", "mapped", "native", "native"], paths: [["a.md"], ["z.json"], ["c.toml"], []] }
    ]
  };
  const flags = function (ids) {
    return compareRows(formats, ids).map(function (row) { return [row.differs, row.formatDiffers]; });
  };
  assert.deepEqual(flags(["one", "two"]), [[false, true], [false, false], [true, false], [false, false]]);
  assert.deepEqual(flags(["one", "three"]), [[false, false], [true, false], [false, true], [false, false]]);
  assert.deepEqual(flags(["one", "two", "three"]), [[false, true], [true, false], [true, false], [false, false]]);
  assert.deepEqual(compareRows(formats, ["one", "two"])[1].cells[0].formats, ["Markdown"]);
});

test("the summary names who is ahead and who falls behind on each kind", function () {
  const report = summarize(data, ["claude", "codex"]);
  const kinds = function (gaps) { return gaps.map(function (gap) { return gap.feature.id; }); };
  const whom = function (gap) { return gap.others.map(function (other) { return other.target.id + ":" + other.status; }); };
  assert.deepEqual(kinds(report.targets[0].ahead), ["command"]);
  assert.deepEqual(whom(report.targets[0].ahead[0]), ["codex:opt-in"]);
  assert.deepEqual(kinds(report.targets[0].behind), []);
  assert.deepEqual(kinds(report.targets[1].ahead), []);
  assert.deepEqual(kinds(report.targets[1].behind), ["command"]);
  assert.equal(report.targets[1].behind[0].status, "opt-in");
  assert.deepEqual(whom(report.targets[1].behind[0]), ["claude:native"]);
  assert.deepEqual(report.none.map(function (feature) { return feature.id; }), ["review", "ignore"]);
});

test("with three targets, a kind counts when any other column differs", function () {
  const report = summarize(data, ["claude", "cursor", "aider"]);
  const view = function (gaps) {
    return gaps.map(function (gap) {
      return gap.feature.id + "<" + gap.others.map(function (other) { return other.target.id; }).join(",");
    });
  };
  assert.deepEqual(view(report.targets[0].ahead), ["agent<aider", "command<aider"]);
  assert.deepEqual(view(report.targets[0].behind), ["review<cursor", "ignore<cursor,aider"]);
  assert.deepEqual(view(report.targets[1].ahead), ["agent<aider", "command<aider", "review<claude,aider", "ignore<claude"]);
  assert.deepEqual(view(report.targets[1].behind), []);
  assert.deepEqual(view(report.targets[2].ahead), ["ignore<claude"]);
  assert.deepEqual(view(report.targets[2].behind), ["agent<claude,cursor", "command<claude,cursor", "review<cursor"]);
  assert.deepEqual(report.none, []);
});

