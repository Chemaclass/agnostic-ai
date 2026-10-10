import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { DriftChecks, DriftClock, DriftContext } from "../driftChecks";
import { updateDriftStatus } from "../drift";
import { ProcessResult } from "../provenance";

const clean: ProcessResult = { stdout: JSON.stringify({ version: "1", command: "sync --check", writes: [], errors: [] }), stderr: "", code: 0 };
const drift: ProcessResult = { ...clean, stdout: JSON.stringify({ version: "1", command: "sync --check", writes: [{ target: "claude", path: "CLAUDE.md", action: "stale", bytes: 12 }], errors: [] }), code: 1 };

class FakeClock implements DriftClock {
  private callback: (() => void) | undefined;
  every(callback: () => void): () => void {
    this.callback = callback;
    return () => { this.callback = undefined; };
  }
  tick(): void { this.callback?.(); }
}

function fixture() {
  let context: DriftContext | undefined = { binary: "agnostic-ai", cwd: "/project" };
  const calls: { context: DriftContext; signal: AbortSignal; resolve(result: ProcessResult): void; reject(error: Error): void }[] = [];
  const published: ProcessResult[] = [];
  const status = { text: "", tooltip: "", show() {} };
  const timer = new FakeClock();
  const checks = new DriftChecks({
    context: () => context,
    run: (context, signal) => new Promise<ProcessResult>((resolve, reject) => calls.push({ context, signal, resolve, reject })),
    publish: result => { published.push(result); updateDriftStatus(status, result); },
    pollMilliseconds: 5000,
    clock: timer,
  });
  return { checks, calls, published, status, timer, setContext(value: DriftContext | undefined) { context = value; } };
}

async function completeTurn(): Promise<void> {
  await new Promise<void>(resolve => setImmediate(resolve));
}

describe("production drift checks", () => {
  it("bounds a save burst and poll to one active check and one final refresh", async () => {
    const f = fixture();
    f.checks.request();
    for (let i = 0; i < 10; i++) f.checks.request();
    f.timer.tick();
    assert.equal(f.calls.length, 1);
    f.calls[0].resolve(clean);
    await completeTurn();
    assert.equal(f.published.length, 0);
    assert.equal(f.calls.length, 2);
    f.calls[1].resolve(drift);
    await completeTurn();
    assert.equal(f.status.text, "$(warning) agnostic-ai: 1 drifted");
    assert.equal(f.calls.length, 2);
    f.checks.dispose();
  });

  it("publishes slow results despite poll ticks and retains one follow-up", async () => {
    const f = fixture();
    f.checks.request();
    f.timer.tick(); f.timer.tick(); f.timer.tick();
    assert.equal(f.calls.length, 1);
    f.calls[0].resolve(clean);
    await completeTurn();
    assert.equal(f.status.text, "$(check) agnostic-ai: in sync");
    assert.equal(f.calls.length, 2);
    f.calls[1].resolve(clean);
    await completeTurn();
    f.checks.dispose();
  });

  it("retains saves during the follow-up without publishing its obsolete result", async () => {
    const f = fixture();
    f.checks.request(); f.checks.request();
    f.calls[0].resolve(clean);
    await completeTurn();
    f.checks.request();
    f.calls[1].resolve(clean);
    await completeTurn();
    assert.equal(f.published.length, 0);
    assert.equal(f.calls.length, 3);
    f.calls[2].resolve(drift);
    await completeTurn();
    assert.equal(f.published.length, 1);
    f.checks.dispose();
  });

  it("handles failures and runs pending work and later recovery", async () => {
    const f = fixture();
    f.checks.request(); f.timer.tick();
    f.calls[0].reject(new Error("check could not start"));
    await completeTurn();
    assert.match(f.status.text, /check failed/);
    assert.match(f.status.tooltip, /check could not start/);
    assert.equal(f.calls.length, 2);
    f.calls[1].resolve({ stdout: "", stderr: "config invalid", code: 1 });
    await completeTurn();
    assert.match(f.status.tooltip, /config invalid/);
    f.checks.request();
    f.calls[2].resolve(clean);
    await completeTurn();
    assert.match(f.status.text, /in sync/);
    f.checks.dispose();
  });

  it("waits for cancellation to complete before checking a replacement workspace", async () => {
    const f = fixture();
    f.checks.request();
    f.setContext({ binary: "other-binary", cwd: "/replacement" });
    f.checks.request();
    assert.equal(f.calls[0].signal.aborted, true);
    assert.equal(f.calls.length, 1);
    f.calls[0].resolve(clean);
    await completeTurn();
    assert.equal(f.published.length, 0);
    assert.deepEqual(f.calls[1].context, { binary: "other-binary", cwd: "/replacement" });
    f.calls[1].resolve(drift);
    await completeTurn();
    f.checks.dispose();
  });

  it("cancels when the last workspace closes and ignores completion", async () => {
    const f = fixture();
    f.checks.request(); f.checks.request();
    f.setContext(undefined);
    f.checks.request();
    assert.equal(f.calls[0].signal.aborted, true);
    f.calls[0].resolve(clean);
    await completeTurn();
    f.timer.tick();
    assert.equal(f.calls.length, 1);
    assert.equal(f.published.length, 0);
    f.checks.dispose();
  });

  it("disposes active and pending work and ignores later requests and results", async () => {
    const f = fixture();
    f.checks.request(); f.checks.request();
    f.checks.dispose(); f.checks.dispose();
    assert.equal(f.calls[0].signal.aborted, true);
    f.timer.tick(); f.checks.request();
    f.calls[0].resolve(clean);
    await completeTurn();
    assert.equal(f.calls.length, 1);
    assert.equal(f.published.length, 0);
  });
});
