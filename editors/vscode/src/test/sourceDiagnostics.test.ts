import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { DiagnosticContext, DiagnosticSession, SourceDiagnostics, isDiagnosticFile } from "../sourceDiagnostics";

function fixture() {
  const sessions: { context: DiagnosticContext; signal?: AbortSignal; start: { resolve(): void; reject(error: Error): void }; stops: number }[] = [];
  const failures: { context: DiagnosticContext; error: unknown }[] = [];
  const diagnostics = new SourceDiagnostics({
    create(context): DiagnosticSession {
      let resolve!: () => void;
      let reject!: (error: Error) => void;
      const ready = new Promise<void>((yes, no) => { resolve = yes; reject = no; });
      const state = { context, signal: undefined as AbortSignal | undefined, start: { resolve, reject }, stops: 0 };
      sessions.push(state);
      return {
        start(signal) { state.signal = signal; return ready; },
        async stop() { state.stops++; },
      };
    },
    failure(context, error) { failures.push({ context, error }); },
  });
  return { diagnostics, sessions, failures };
}

async function turn(): Promise<void> { await new Promise<void>(resolve => setImmediate(resolve)); }
const project = { binary: "/installed tools/agnostic-ai", cwd: "/configured project" };

describe("production source diagnostic lifecycle", () => {
  it("starts the configured binary and project once", async () => {
    const f = fixture();
    const ready = f.diagnostics.update(project);
    await turn();
    assert.equal(f.sessions.length, 1);
    assert.deepEqual(f.sessions[0].context, project);
    f.sessions[0].start.resolve();
    await ready;
    await f.diagnostics.update({ ...project });
    assert.equal(f.sessions.length, 1);
    await f.diagnostics.dispose();
    assert.equal(f.sessions[0].stops, 1);
  });

  it("cancels obsolete startup and starts only the latest replacement", async () => {
    const f = fixture();
    const first = f.diagnostics.update(project);
    await turn();
    assert.equal(f.sessions.length, 1);
    const second = f.diagnostics.update({ ...project, binary: "/replacement" });
    const latest = { ...project, binary: "/final binary", cwd: "/new project" };
    const third = f.diagnostics.update(latest);
    assert.equal(f.sessions[0].signal?.aborted, true);
    f.sessions[0].start.reject(new Error("obsolete startup cancelled"));
    await turn();
    assert.equal(f.sessions[0].stops, 1);
    assert.equal(f.sessions.length, 2);
    assert.deepEqual(f.sessions[1].context, latest);
    assert.equal(f.failures.length, 0);
    f.sessions[1].start.resolve();
    await Promise.all([first, second, third]);
    await f.diagnostics.dispose();
  });

  it("stops a running service when the workspace loses its config", async () => {
    const f = fixture();
    const ready = f.diagnostics.update(project);
    await turn();
    assert.equal(f.sessions.length, 1);
    f.sessions[0].start.resolve();
    await ready;
    await f.diagnostics.update(undefined);
    assert.equal(f.sessions[0].stops, 1);
    await f.diagnostics.dispose();
    assert.equal(f.sessions[0].stops, 1);
  });

  it("reports startup failure once and recovers with a changed binary", async () => {
    const f = fixture();
    const failed = f.diagnostics.update(project);
    await turn();
    assert.equal(f.sessions.length, 1);
    f.sessions[0].start.reject(new Error("ENOENT"));
    await failed;
    assert.equal(f.failures.length, 1);
    assert.equal(f.sessions[0].stops, 1);
    await f.diagnostics.update(project);
    assert.equal(f.sessions.length, 1);
    const recovered = f.diagnostics.update({ ...project, binary: "/working CLI" });
    await turn();
    assert.equal(f.sessions.length, 2);
    f.sessions[1].start.resolve();
    await recovered;
    await f.diagnostics.dispose();
  });

  it("disposes during startup without a replacement or stale failure", async () => {
    const f = fixture();
    const ready = f.diagnostics.update(project);
    await turn();
    assert.equal(f.sessions.length, 1);
    const stopped = f.diagnostics.dispose();
    assert.equal(f.sessions[0].signal?.aborted, true);
    f.sessions[0].start.reject(new Error("cancelled"));
    await Promise.all([ready, stopped]);
    await f.diagnostics.update(project);
    assert.equal(f.sessions.length, 1);
    assert.equal(f.sessions[0].stops, 1);
    assert.equal(f.failures.length, 0);
  });
});

it("selects saved Markdown and YAML files without assuming a source directory", () => {
  for (const fsPath of ["/project/.agnostic-ai/rules/a.md", "/project/custom agents/a.md", "/project/custom skills/a.md", "/project/custom settings/a.yaml", "/project/custom mcps/a.yaml", "/project/custom hooks/a.yaml", "/outside/absolute sources/a.yml", "/project/rules/a.mdc", "/project/agnostic-ai.yaml", "/project/agnostic.config.yaml", "/project/agnostic-ai.local.yaml"]) {
    assert.equal(isDiagnosticFile({ scheme: "file", fsPath }), true, fsPath);
  }
  assert.equal(isDiagnosticFile({ scheme: "file", fsPath: "/project/src/main.go" }), false);
  assert.equal(isDiagnosticFile({ scheme: "untitled", fsPath: "a.md" }), false);
});
