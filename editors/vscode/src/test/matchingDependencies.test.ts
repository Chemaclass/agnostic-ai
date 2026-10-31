import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { describe, it } from "node:test";

describe("matching dependencies", () => {
  it("keeps language-client constructor matching and regular expressions", () => {
    const load = createRequire(require.resolve("vscode-languageclient"));
    const { Minimatch } = load("minimatch");
    const matcher = new Minimatch(".agnostic-ai/**/*.{md,yaml}", { noext: true });
    assert.equal(matcher.match(".agnostic-ai/settings/voice.yaml"), true);
    assert.equal(matcher.match(".agnostic-ai/rules/style.md"), true);
    assert.equal(matcher.match(".agnostic-ai/rules/style.txt"), false);
    const expression = new Minimatch("**/*.YAML", { noext: true, nocase: true }).makeRe();
    assert.ok(expression instanceof RegExp);
    assert.match("settings/voice.yaml", expression);
  });

  it("keeps packaging filters matching hidden files and exclusions", () => {
    const load = createRequire(require.resolve("@vscode/vsce"));
    const { minimatch } = load("minimatch");
    assert.equal(minimatch(".agnostic-ai/settings/voice.yaml", "**/*.yaml", { dot: true }), true);
    assert.equal(minimatch("node_modules/example/index.js", "node_modules/**", { dot: true }), true);
    assert.equal(minimatch("out/extension.js", "node_modules/**", { dot: true }), false);
  });
});
