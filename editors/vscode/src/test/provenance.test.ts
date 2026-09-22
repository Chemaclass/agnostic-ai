// Tests for the pure provenance helpers behind "Open canonical source".
// Fixtures under test/fixtures/why/ are real `agnostic-ai why --format json`
// output captured from a synced project whose source directories are
// configured (`ai specs/rules`, `ai specs/skills`) and contain spaces.

import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { describe, it } from "node:test";

import {
  WhyOutputError,
  describeWhyFailure,
  findProjectRoot,
  parseWhyOutput,
  planNavigation,
  whyArgs,
} from "../provenance";

const fixtures = path.join(__dirname, "..", "..", "test", "fixtures", "why");

function fixture(name: string): string {
  return fs.readFileSync(path.join(fixtures, name), "utf8");
}

const root = path.join(path.sep, "work", "my project");

function existsAll(): boolean {
  return true;
}

describe("parseWhyOutput", () => {
  it("accepts a single-source skill report", () => {
    const report = parseWhyOutput(fixture("skill-single.json"));
    assert.equal(report.file, ".claude/skills/release-notes/SKILL.md");
    assert.deepEqual(report.sources, [
      {
        kind: "skill",
        name: "release-notes",
        path: "ai specs/skills/release notes/SKILL.md",
        mode: "full",
      },
    ]);
  });

  it("accepts a merged entry-point report", () => {
    const report = parseWhyOutput(fixture("agents-merged.json"));
    assert.equal(report.sources.length, 2);
  });

  it("rejects output that is not JSON", () => {
    assert.throws(() => parseWhyOutput("adapter: claude\n"), WhyOutputError);
  });

  it("rejects an unknown envelope version", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    raw.version = "2";
    assert.throws(() => parseWhyOutput(JSON.stringify(raw)), WhyOutputError);
  });

  it("rejects output from another command", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    raw.command = "explain";
    assert.throws(() => parseWhyOutput(JSON.stringify(raw)), WhyOutputError);
  });

  it("rejects a missing sources array", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    delete raw.sources;
    assert.throws(() => parseWhyOutput(JSON.stringify(raw)), WhyOutputError);
  });

  it("rejects a source without a usable path", () => {
    for (const bad of ["", 42, null]) {
      const raw = JSON.parse(fixture("rule-single.json"));
      raw.sources[0].path = bad;
      assert.throws(
        () => parseWhyOutput(JSON.stringify(raw)),
        WhyOutputError,
        `path ${JSON.stringify(bad)}`,
      );
    }
  });

  it("rejects a source path with a NUL byte", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    raw.sources[0].path = "ai specs/rules/tabs.md\u0000.png";
    assert.throws(() => parseWhyOutput(JSON.stringify(raw)), WhyOutputError);
  });
});

describe("planNavigation", () => {
  it("opens a single source directly, resolved against the project root", () => {
    const report = parseWhyOutput(fixture("skill-single.json"));
    const plan = planNavigation(report, root, existsAll);
    assert.deepEqual(plan, {
      kind: "open",
      path: path.join(root, "ai specs", "skills", "release notes", "SKILL.md"),
    });
  });

  it("offers a picker with names and paths for a merged file", () => {
    const report = parseWhyOutput(fixture("agents-merged.json"));
    const plan = planNavigation(report, root, existsAll);
    assert.equal(plan.kind, "pick");
    if (plan.kind !== "pick") return;
    assert.deepEqual(
      plan.items.map((i) => [i.label, i.description, i.path]),
      [
        [
          "no-console-log",
          "rule · ai specs/rules/no-console-log.md",
          path.join(root, "ai specs", "rules", "no-console-log.md"),
        ],
        [
          "tabs",
          "rule · ai specs/rules/tabs.md",
          path.join(root, "ai specs", "rules", "tabs.md"),
        ],
      ],
    );
  });

  it("orders picker items deterministically whatever the CLI order", () => {
    const raw = JSON.parse(fixture("agents-merged.json"));
    raw.sources.reverse();
    const plan = planNavigation(
      parseWhyOutput(JSON.stringify(raw)),
      root,
      existsAll,
    );
    assert.equal(plan.kind, "pick");
    if (plan.kind !== "pick") return;
    assert.deepEqual(
      plan.items.map((i) => i.label),
      ["no-console-log", "tabs"],
    );
  });

  it("keeps an absolute source path from another layer as is", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    const abs = path.join(path.sep, "home", "me", ".agnostic-ai", "rules", "tabs.md");
    raw.sources[0].path = abs;
    const plan = planNavigation(
      parseWhyOutput(JSON.stringify(raw)),
      root,
      existsAll,
    );
    assert.deepEqual(plan, { kind: "open", path: abs });
  });

  it("refuses to open a source that is missing on disk", () => {
    const report = parseWhyOutput(fixture("rule-single.json"));
    const plan = planNavigation(report, root, () => false);
    assert.equal(plan.kind, "error");
    if (plan.kind !== "error") return;
    assert.match(plan.message, /ai specs\/rules\/tabs\.md/);
    assert.match(plan.message, /sync/);
  });

  it("drops missing sources from the picker and reports them", () => {
    const report = parseWhyOutput(fixture("agents-merged.json"));
    const plan = planNavigation(
      report,
      root,
      (p) => !p.endsWith("tabs.md"),
    );
    assert.deepEqual(plan, {
      kind: "open",
      path: path.join(root, "ai specs", "rules", "no-console-log.md"),
      missing: ["ai specs/rules/tabs.md"],
    });
  });

  it("refuses a report whose file resolved outside the project", () => {
    // What `why` prints when its working directory is reached through a
    // symlink: the file no longer matches exactly and sources are a
    // basename guess, so nothing may open.
    const raw = JSON.parse(fixture("rule-single.json"));
    raw.file = "../../private/tmp/my project/.claude/rules/tabs.md";
    const plan = planNavigation(
      parseWhyOutput(JSON.stringify(raw)),
      root,
      existsAll,
    );
    assert.equal(plan.kind, "error");
    if (plan.kind !== "error") return;
    assert.match(plan.message, /outside the project/);
  });

  it("explains a file with no contributing source", () => {
    const raw = JSON.parse(fixture("rule-single.json"));
    raw.sources = [];
    const plan = planNavigation(
      parseWhyOutput(JSON.stringify(raw)),
      root,
      existsAll,
    );
    assert.equal(plan.kind, "error");
    if (plan.kind !== "error") return;
    assert.match(plan.message, /no source spec/);
  });
});

describe("describeWhyFailure", () => {
  it("tells the user to sync when no sync state exists", () => {
    const msg = describeWhyFailure({
      code: 1,
      stdout: "",
      stderr: fixture("no-sync-state.stderr.txt"),
    });
    assert.match(msg, /no sync state/i);
    assert.match(msg, /agnostic-ai sync/);
  });

  it("explains an untracked file", () => {
    const msg = describeWhyFailure({
      code: 1,
      stdout: "",
      stderr: fixture("untracked.stderr.txt"),
    });
    assert.match(msg, /not generated by agnostic-ai/);
  });

  it("asks for an upgrade when the CLI lacks why --format json", () => {
    for (const stderr of [
      'Error: unknown command "why" for "agnostic-ai"\n',
      "Error: unknown flag: --format\n",
    ]) {
      assert.match(
        describeWhyFailure({ code: 1, stdout: "", stderr }),
        /upgrade/i,
      );
    }
  });

  it("falls back to the first stderr line", () => {
    assert.match(
      describeWhyFailure({ code: 2, stdout: "", stderr: "boom\nstack\n" }),
      /boom$/,
    );
  });
});

describe("whyArgs", () => {
  it("passes the document path relative to the root as one argument", () => {
    const doc = path.join(root, ".claude", "skills", "release-notes", "SKILL.md");
    assert.deepEqual(whyArgs(root, doc), [
      "why",
      path.join(".claude", "skills", "release-notes", "SKILL.md"),
      "--format",
      "json",
    ]);
  });
});

describe("findProjectRoot", () => {
  const second = path.join(path.sep, "work", "second folder");
  const configs = new Set([
    path.join(root, "agnostic-ai.yaml"),
    path.join(second, "agnostic.config.yaml"),
  ]);
  const exists = (p: string): boolean => configs.has(p);

  it("finds the config in the document's workspace folder", () => {
    const doc = path.join(root, ".claude", "rules", "tabs.md");
    assert.equal(findProjectRoot(doc, root, exists), root);
  });

  it("uses the second workspace folder for a document inside it", () => {
    const doc = path.join(second, "AGENTS.md");
    assert.equal(findProjectRoot(doc, second, exists), second);
  });

  it("finds a nested project below the workspace folder", () => {
    const nested = path.join(root, "packages", "app");
    const nestedExists = (p: string): boolean =>
      p === path.join(nested, "agnostic-ai.yaml");
    const doc = path.join(nested, "AGENTS.md");
    assert.equal(findProjectRoot(doc, root, nestedExists), nested);
  });

  it("does not climb above the workspace folder", () => {
    const folder = path.join(root, "sub");
    const doc = path.join(folder, "AGENTS.md");
    assert.equal(findProjectRoot(doc, folder, exists), undefined);
  });

  it("climbs to the filesystem root without a workspace folder", () => {
    const doc = path.join(root, "deep", "AGENTS.md");
    assert.equal(findProjectRoot(doc, undefined, exists), root);
  });
});
