import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { DriftStatusBar, updateDriftStatus } from "../drift";
import { ProcessResult } from "../provenance";

const drift = { target: "claude", path: ".claude/rules/testing.md", action: "missing", bytes: 0 };
const failure = { target: "agnostic-ai", message: "rules/testing.md: unknown field alwaysApplies; use alwaysApply" };

function result(writes: unknown[] = [], errors: unknown[] = [], code = 0): ProcessResult {
  return { stdout: JSON.stringify({ version: "1", command: "sync --check", writes, skipped: [], errors, warnings: null, notes: null }), stderr: "", code };
}

function statusBar(): DriftStatusBar & { tooltip: string; shown: number } {
  return { text: "", tooltip: "agnostic-ai drift count", shown: 0, show() { this.shown++; } };
}

describe("production drift status presentation", () => {
  it("shows an actionable error with no writes instead of in sync", () => {
    const status = statusBar();
    updateDriftStatus(status, result([], [failure], 1));
    assert.match(status.text, /check failed/);
    assert.match(status.tooltip || "", /rules\/testing.md: unknown field alwaysApplies; use alwaysApply/);
    assert.equal(status.shown, 1);
  });

  it("prioritizes errors over drift records and over a successful exit", () => {
    const status = statusBar();
    updateDriftStatus(status, result([drift], [failure], 0));
    assert.match(status.text, /check failed/);
    assert.match(status.tooltip || "", /unknown field alwaysApplies/);
    assert.doesNotMatch(status.text, /in sync|drifted/);
  });

  it("keeps normal nonzero drift visible", () => {
    const status = statusBar();
    updateDriftStatus(status, result([drift], [], 1));
    assert.equal(status.text, "$(warning) agnostic-ai: 1 drifted");
    assert.match(status.tooltip || "", /1 generated file/);
  });

  it("shows clean results and clears the previous failure reason after recovery", () => {
    const status = statusBar();
    updateDriftStatus(status, result([], [failure], 1));
    updateDriftStatus(status, result());
    assert.equal(status.text, "$(check) agnostic-ai: in sync");
    assert.match(status.tooltip || "", /Generated files are in sync/);
    assert.doesNotMatch(status.tooltip || "", /alwaysApplies/);
    assert.equal(status.shown, 2);
  });

  it("rejects nonzero exits without drift or reported errors", () => {
    const status = statusBar();
    updateDriftStatus(status, { ...result([], [], 2), stderr: "config.yaml: permission denied\nRun with a readable config." });
    assert.match(status.text, /check failed/);
    assert.match(status.tooltip || "", /config.yaml: permission denied/);
  });

  it("does not classify an unexpected exit with writes as normal drift", () => {
    const status = statusBar();
    updateDriftStatus(status, { ...result([drift], [], 137), stderr: "check terminated" });
    assert.match(status.text, /check failed/);
    assert.match(status.tooltip || "", /check terminated/);
  });

  it("preserves stderr when JSON is malformed or absent", () => {
    for (const stdout of ["", "not JSON"]) {
      const status = statusBar();
      updateDriftStatus(status, { stdout, stderr: "agnostic-ai.yaml: invalid targets; fix the name", code: 1 });
      assert.match(status.text, /check failed/);
      assert.match(status.tooltip || "", /invalid targets; fix the name/);
    }
  });

  it("explains empty or invalid output when stderr has no reason", () => {
    const status = statusBar();
    updateDriftStatus(status, { stdout: "", stderr: "", code: 0 });
    assert.match(status.text, /check failed/);
    assert.match(status.tooltip || "", /no JSON output/);
    updateDriftStatus(status, { stdout: "not JSON", stderr: "", code: 0 });
    assert.match(status.tooltip || "", /invalid JSON/);
  });

  it("explains how to repair a missing binary", () => {
    const status = statusBar();
    updateDriftStatus(status, { stdout: "", stderr: "", code: -1 });
    assert.match(status.text, /not found/);
    assert.match(status.tooltip || "", /Install agnostic-ai|binaryPath/);
  });

  it("never reports malformed result contracts as clean", () => {
    const invalid: unknown[] = [null, [], {}, { writes: [], errors: [] },
      { version: "2", command: "sync --check", writes: [], errors: [] },
      { version: "1", command: "sync", writes: [], errors: [] },
      { version: "1", command: "sync --check", writes: null, errors: [] },
      { version: "1", command: "sync --check", writes: [], errors: "failed" },
      { version: "1", command: "sync --check", writes: [{}], errors: [] },
      { version: "1", command: "sync --check", writes: [], errors: [{}] }];
    for (const value of invalid) {
      const status = statusBar();
      assert.doesNotThrow(() => updateDriftStatus(status, { stdout: JSON.stringify(value), stderr: "", code: 0 }));
      assert.match(status.text, /check failed/);
      assert.match(status.tooltip || "", /invalid.*result|unsupported.*result/i);
    }
  });
});
