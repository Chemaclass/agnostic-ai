"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  decodeEntities,
  normalize,
  parseIndex,
  prepare,
  scoreEntry,
  search,
  shortcutHint,
  teaser,
  tokenize
} = require("./search.js");

const entries = [
  {
    url: "https://agnostic-ai.org/docs/spec-format/",
    title: "Spec format",
    heading: "",
    group: "Docs",
    body: "Every spec kind lives under .agnostic-ai/ and syncs to each target."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#hooks",
    title: "Spec format",
    heading: "Hooks",
    group: "Docs",
    body: "Pure YAML. A hook runs a command on a lifecycle event."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#skills",
    title: "Spec format",
    heading: "Skills",
    group: "Docs",
    body: "Skills bundle instructions. Run sync --check in CI so hooks never drift."
  },
  {
    url: "https://agnostic-ai.org/docs/targets/cursor/",
    title: "Cursor",
    heading: "",
    group: "Targets",
    body: "Cursor reads rules from .cursor/rules and MCP servers from .cursor/mcp.json."
  },
  {
    url: "https://agnostic-ai.org/updates/2026-09-16-v0.59.0/#shipped",
    title: "v0.59.0",
    heading: "Shipped",
    group: "Updates",
    body: "Cursor rules keep their globs &amp; the &lt;scope&gt; stays intact."
  }
];

const prepared = prepare(entries);

function urls(results) {
  return results.map(function (entry) {
    return entry.url;
  });
}

test("every query term must match", function () {
  assert.deepEqual(urls(search(prepared, "cursor rules")), [
    "https://agnostic-ai.org/docs/targets/cursor/",
    "https://agnostic-ai.org/updates/2026-09-16-v0.59.0/#shipped"
  ]);
  assert.deepEqual(search(prepared, "cursor hooks"), []);
});

test("a heading match outranks a body match", function () {
  const found = urls(search(prepared, "hooks"));
  assert.equal(found[0], "https://agnostic-ai.org/docs/spec-format/#hooks");
  assert.ok(found.includes("https://agnostic-ai.org/docs/spec-format/#skills"));
});

test("terms match word prefixes and unknown words match nothing", function () {
  assert.equal(urls(search(prepared, "hook"))[0], "https://agnostic-ai.org/docs/spec-format/#hooks");
  assert.deepEqual(search(prepared, "xyzzy"), []);
  assert.equal(scoreEntry(prepared[3], tokenize("xyzzy")), 0);
});

test("a page entry beats its own sections on an exact title match", function () {
  assert.equal(urls(search(prepared, "spec format"))[0], "https://agnostic-ai.org/docs/spec-format/");
});

test("normalizing decodes entities and keeps CLI flags and paths", function () {
  assert.equal(decodeEntities("&lt;a&gt; &amp; &quot;b&quot; &#39;c&#39;"), "<a> & \"b\" 'c'");
  assert.equal(normalize("Run `sync --check`, then .agnostic-ai/ and pre_commit"), "run sync --check then .agnostic-ai/ and pre_commit");
  assert.equal(urls(search(prepared, "--check"))[0], "https://agnostic-ai.org/docs/spec-format/#skills");
});

test("short queries return nothing and the limit is honoured", function () {
  assert.deepEqual(search(prepared, "c"), []);
  assert.deepEqual(search(prepared, "  "), []);
  assert.equal(search(prepared, "cursor", 1).length, 1);
});

test("teaser marks the term and truncates around it", function () {
  const body = "Intro words. ".repeat(30) + "Hooks run after tools. " + "Trailing words. ".repeat(30);
  const segments = teaser(body, ["hooks"], 60);
  const text = segments.map(function (segment) {
    return segment.text;
  }).join("");
  assert.ok(text.startsWith("…"));
  assert.ok(text.endsWith("…"));
  assert.ok(text.length <= 62);
  assert.deepEqual(segments.filter(function (segment) {
    return segment.mark;
  }), [{ text: "Hooks", mark: true }]);
  assert.deepEqual(teaser("Tom &amp; Jerry", ["zzz"]), [{ text: "Tom & Jerry", mark: false }]);
});

test("the shortcut hint follows the platform", function () {
  assert.equal(shortcutHint("MacIntel"), "⌘K");
  assert.equal(shortcutHint("macOS"), "⌘K");
  assert.equal(shortcutHint("Win32"), "Ctrl K");
});

test("parses the index with or without the dev server's live-reload script", function () {
  const json = '[{"url":"/docs/","title":"Docs [start]","heading":"","group":"Docs","body":"a ] b"}]';
  assert.equal(parseIndex(json)[0].title, "Docs [start]");
  const served = json + '\n<script src="/livereload.js?port=1111&amp;mindelay=10"></script>';
  assert.deepEqual(parseIndex(served), parseIndex(json));
});
