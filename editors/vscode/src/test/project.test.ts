// Tests for the pure project helpers every extension surface shares:
// which config file names count, where the project root is, and which
// targets the config lists.

import assert from "node:assert/strict";
import * as fs from "node:fs";
import * as path from "node:path";
import { describe, it } from "node:test";

import {
  CONFIG_FILE_NAMES,
  findConfigFile,
  findProjectRoot,
  parseTargets,
  pickWorkspaceRoot,
} from "../project";

const root = path.join(path.sep, "work", "my project");

describe("CONFIG_FILE_NAMES", () => {
  it("lists the current name before the legacy one, like the CLI", () => {
    assert.deepEqual(CONFIG_FILE_NAMES, [
      "agnostic-ai.yaml",
      "agnostic.config.yaml",
    ]);
  });
});

describe("package.json", () => {
  const manifest = JSON.parse(
    fs.readFileSync(path.join(__dirname, "..", "..", "package.json"), "utf8"),
  );

  it("activates on every config file name", () => {
    assert.deepEqual(
      manifest.activationEvents,
      CONFIG_FILE_NAMES.map((n) => `workspaceContains:${n}`),
    );
  });

  it("validates every config file name against the schema", () => {
    assert.deepEqual(manifest.contributes.yamlValidation[0].fileMatch, [
      ...CONFIG_FILE_NAMES,
    ]);
  });
});

describe("findConfigFile", () => {
  it("finds agnostic-ai.yaml", () => {
    const cfg = path.join(root, "agnostic-ai.yaml");
    assert.equal(findConfigFile(root, (p) => p === cfg), cfg);
  });

  it("finds the legacy agnostic.config.yaml", () => {
    const cfg = path.join(root, "agnostic.config.yaml");
    assert.equal(findConfigFile(root, (p) => p === cfg), cfg);
  });

  it("prefers agnostic-ai.yaml when both exist", () => {
    assert.equal(
      findConfigFile(root, () => true),
      path.join(root, "agnostic-ai.yaml"),
    );
  });

  it("returns undefined without a config", () => {
    assert.equal(findConfigFile(root, () => false), undefined);
  });
});

describe("pickWorkspaceRoot", () => {
  const first = path.join(path.sep, "work", "first");
  const second = path.join(path.sep, "work", "second");

  it("picks the folder holding agnostic-ai.yaml", () => {
    const exists = (p: string): boolean =>
      p === path.join(second, "agnostic-ai.yaml");
    assert.equal(pickWorkspaceRoot([first, second], exists), second);
  });

  it("picks the folder holding the legacy config", () => {
    const exists = (p: string): boolean =>
      p === path.join(second, "agnostic.config.yaml");
    assert.equal(pickWorkspaceRoot([first, second], exists), second);
  });

  it("falls back to the first folder without a config", () => {
    assert.equal(pickWorkspaceRoot([first, second], () => false), first);
  });

  it("returns undefined without folders", () => {
    assert.equal(pickWorkspaceRoot([], () => true), undefined);
  });
});

describe("parseTargets", () => {
  it("lists the targets block", () => {
    const text = [
      "# yaml-language-server: $schema=x",
      "targets:",
      "  - claude",
      "  - cursor",
      "sources:",
      "  - ai specs",
      "",
    ].join("\n");
    assert.deepEqual(parseTargets(text), ["claude", "cursor"]);
  });

  it("returns nothing without a targets block", () => {
    assert.deepEqual(parseTargets("version: 1\n"), []);
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
