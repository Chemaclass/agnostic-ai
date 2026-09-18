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
  stem,
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

// A second corpus for the ranking rules. The fixture above asserts exact
// URL lists, so adding entries to it would churn those assertions.
const corpus = [
  {
    url: "https://agnostic-ai.org/docs/spec-format/",
    title: "Spec format",
    heading: "",
    group: "Docs",
    body: "Every spec kind lives under .agnostic-ai/. Rules, hooks, and skills all sync."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#hooks",
    title: "Spec format",
    heading: "Hooks",
    group: "Docs",
    body: "A hook runs a command on a lifecycle event."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#rules",
    title: "Spec format",
    heading: "Rules",
    group: "Docs",
    body: "A rule is a markdown file with frontmatter."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#commands",
    title: "Spec format",
    heading: "Commands",
    group: "Docs",
    body: "One markdown file per command."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#settings",
    title: "Spec format",
    heading: "Settings",
    group: "Docs",
    body: "Settings carry permissions and a model."
  },
  {
    url: "https://agnostic-ai.org/docs/spec-format/#agent-policy-support-by-target",
    title: "Spec format",
    heading: "permissionMode and agent hooks support by target",
    group: "Docs",
    body: "Hooks reach some targets. Hooks drop on others. Hooks vary. Hooks differ."
  },
  {
    url: "https://agnostic-ai.org/docs/git-hooks/",
    title: "Git hooks",
    heading: "",
    group: "Docs",
    body: "Regenerate outputs on commit."
  },
  {
    url: "https://agnostic-ai.org/docs/git-hooks/#lefthook",
    title: "Git hooks",
    heading: "lefthook",
    group: "Docs",
    body: "Add a pre-commit hook that runs sync --check."
  },
  {
    url: "https://agnostic-ai.org/docs/migration/",
    title: "Import existing tool configuration",
    heading: "",
    group: "Docs",
    body: "Move an existing setup into .agnostic-ai/."
  },
  {
    url: "https://agnostic-ai.org/docs/errors/",
    title: "Error codes",
    heading: "",
    group: "Docs",
    body: "Every AAI diagnostic and its fix."
  },
  {
    url: "https://agnostic-ai.org/docs/installation/",
    title: "Installation",
    heading: "",
    group: "Docs",
    body: "Download the binary."
  },
  {
    url: "https://agnostic-ai.org/docs/getting-started/#install",
    title: "Getting started",
    heading: "Install",
    group: "Docs",
    body: "Install the CLI first."
  },
  {
    url: "https://agnostic-ai.org/docs/targets/claude/",
    title: "Claude Code",
    heading: "",
    group: "Targets",
    body: "How agnostic-ai emits Claude Code configuration."
  },
  {
    url: "https://agnostic-ai.org/docs/targets/claude/#claude-settings",
    title: "Claude Code",
    heading: "Claude settings",
    group: "Targets",
    body: "Claude settings hold permissions. Claude reads them. Claude merges them."
  },
  {
    url: "https://agnostic-ai.org/updates/2026-09-18-v0.61.0/",
    title: "agnostic-ai v0.61.0: Cline rules land where Cline reads them",
    heading: "",
    group: "Updates",
    body: "Cline rules move to .clinerules."
  },
  {
    url: "https://agnostic-ai.org/updates/2026-09-18-v0.61.0/#fixed",
    title: "agnostic-ai v0.61.0: Cline rules land where Cline reads them",
    heading: "Fixed",
    group: "Updates",
    body: "Cline rules now land in .clinerules."
  }
];

const ranked = prepare(corpus);

function firstUrl(query) {
  const found = search(ranked, query);
  return found.length === 0 ? "" : found[0].url;
}

function rankOf(query, url) {
  return urls(search(ranked, query)).indexOf(url) + 1;
}

test("a heading the query covers outranks a longer one with more body hits", function () {
  const found = urls(search(ranked, "hooks"));
  assert.equal(found[0], "https://agnostic-ai.org/docs/spec-format/#hooks");
  const table = found.indexOf("https://agnostic-ai.org/docs/spec-format/#agent-policy-support-by-target");
  assert.ok(table > 0, "the six-word heading still matches");
  assert.ok(found.indexOf("https://agnostic-ai.org/docs/git-hooks/") < table);
});

test("a page the query names beats its own sections", function () {
  const found = urls(search(ranked, "claude"));
  assert.equal(found[0], "https://agnostic-ai.org/docs/targets/claude/");
  assert.equal(found[1], "https://agnostic-ai.org/docs/targets/claude/#claude-settings");
});

test("the slug carries the topic when the title does not", function () {
  assert.equal(firstUrl("migration"), "https://agnostic-ai.org/docs/migration/");
});

test("plurals fold onto the singular in both directions", function () {
  assert.equal(stem("hooks"), "hook");
  assert.equal(stem("class"), "class");
  assert.equal(stem("as"), "as");
  assert.equal(stem("v0.61.0"), "v0.61.0");
  assert.equal(firstUrl("errors"), "https://agnostic-ai.org/docs/errors/");
  assert.equal(firstUrl("hook"), firstUrl("hooks"));
});

test("a snippet highlights a result that matched on the stem", function () {
  const segments = teaser("Hooks run after each tool call.", ["hooks"].map(stem));
  assert.deepEqual(segments.filter(function (segment) {
    return segment.mark;
  }), [{ text: "Hook", mark: true }]);
});

test("a section inherits only a little of its page title", function () {
  const found = urls(search(ranked, "rules"));
  assert.equal(found[0], "https://agnostic-ai.org/docs/spec-format/#rules");
  const briefing = found.indexOf("https://agnostic-ai.org/updates/2026-09-18-v0.61.0/");
  assert.ok(briefing > 0, "the briefing whose title says rules still matches");
  assert.ok(found.indexOf("https://agnostic-ai.org/docs/spec-format/#rules") < briefing);
});

test("Updates rank below Docs and Targets on equal evidence", function () {
  const twins = prepare([
    {
      url: "https://agnostic-ai.org/updates/2026-09-18-v0.61.0/#fixed",
      title: "Release",
      heading: "Telemetry",
      group: "Updates",
      body: "Telemetry never leaves the machine."
    },
    {
      url: "https://agnostic-ai.org/docs/telemetry/",
      title: "Release",
      heading: "Telemetry",
      group: "Docs",
      body: "Telemetry never leaves the machine."
    }
  ]);
  assert.ok(scoreEntry(twins[1], ["telemetry"]) > scoreEntry(twins[0], ["telemetry"]));
  assert.equal(search(twins, "telemetry")[0].group, "Docs", "the Docs twin wins despite coming second");
});

test("one page yields slots to other pages before filling the rest", function () {
  const sections = ["one", "two", "three", "four", "five"].map(function (name) {
    return {
      url: "https://agnostic-ai.org/docs/alpha/#" + name,
      title: "Alpha",
      heading: "Widget " + name,
      group: "Docs",
      body: "A widget section."
    };
  });
  const many = prepare([{
    url: "https://agnostic-ai.org/docs/alpha/",
    title: "Alpha",
    heading: "",
    group: "Docs",
    body: "Widget overview."
  }].concat(sections, [
    {
      url: "https://agnostic-ai.org/docs/beta/",
      title: "Beta",
      heading: "",
      group: "Docs",
      body: "Another widget guide."
    },
    {
      url: "https://agnostic-ai.org/docs/gamma/",
      title: "Gamma",
      heading: "",
      group: "Docs",
      body: "A third widget guide."
    }
  ]));

  const found = urls(search(many, "widget"));
  const fromAlpha = found.filter(function (url) {
    return url.indexOf("/docs/alpha/") !== -1;
  });
  const fourth = found.indexOf(fromAlpha[3]);
  assert.equal(fromAlpha.length, 6, "held-back entries still reach empty slots");
  assert.ok(found.indexOf("https://agnostic-ai.org/docs/beta/") < fourth);
  assert.ok(found.indexOf("https://agnostic-ai.org/docs/gamma/") < fourth);
  assert.equal(found.length, 8, "the cap does not shrink the result list");
});

test("typing a prefix keeps the same top result", function () {
  assert.equal(firstUrl("instal"), "https://agnostic-ai.org/docs/installation/");
  assert.equal(firstUrl("install"), "https://agnostic-ai.org/docs/installation/");
  assert.equal(rankOf("install", "https://agnostic-ai.org/docs/getting-started/#install"), 2);
});

test("scoreEntry stems the terms it is handed", function () {
  const hooks = ranked.find(function (item) {
    return item.entry.url === "https://agnostic-ai.org/docs/spec-format/#hooks";
  });
  assert.equal(scoreEntry(hooks, tokenize("hooks")), scoreEntry(hooks, tokenize("hook")));
  assert.equal(scoreEntry(hooks, tokenize("xyzzy")), 0);
});
