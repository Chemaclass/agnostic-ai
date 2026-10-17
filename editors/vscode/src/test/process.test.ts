import assert from "node:assert/strict";
import { describe, it } from "node:test";
import * as fs from "node:fs";
import * as os from "node:os";
import * as path from "node:path";

import { execCommand } from "../process";

function onFile(watcher: fs.FSWatcher, file: string): Promise<void> {
  return new Promise(resolve => {
    if (fs.existsSync(file)) { resolve(); return; }
    watcher.on("change", () => { if (fs.existsSync(file)) resolve(); });
  });
}

function kill(pid: number): void {
  try { process.kill(pid, "SIGKILL"); } catch {}
}

async function exited(pid: number): Promise<void> {
  for (;;) {
    try { process.kill(pid, 0); } catch { return; }
    await new Promise<void>(resolve => setImmediate(resolve));
  }
}

describe("production process runner", () => {
  it("preserves stdout, stderr, exit status, and the working directory", async () => {
    const result = await execCommand(process.execPath, ["-e", "process.stdout.write(JSON.stringify({cwd: process.cwd(), pwd: process.env.PWD})); process.stderr.write('failure reason'); process.exitCode = 7"], process.cwd());
    assert.equal(result.code, 7);
    assert.equal(result.stderr, "failure reason");
    assert.deepEqual(JSON.parse(result.stdout), { cwd: process.cwd(), pwd: process.cwd() });
  });

  it("reports a missing binary using the existing failure contract", async () => {
    const result = await execCommand("agnostic-ai-nonexistent-test-binary", [], process.cwd());
    assert.equal(result.code, -1);
    assert.equal(result.stdout, "");
  });

  it("does not start a command whose signal is already cancelled", async () => {
    const controller = new AbortController();
    controller.abort();
    const result = await execCommand(process.execPath, ["-e", "process.stdout.write('should not run')"], process.cwd(), controller.signal);
    assert.notEqual(result.code, 0);
    assert.equal(result.stdout, "");
  });

  it("terminates an active real child before resolving cancellation", { timeout: 30000 }, async t => {
    const controller = new AbortController();
    const fs = await import("node:fs");
    const os = await import("node:os");
    const path = await import("node:path");
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "agnostic-process-"));
    const ready = path.join(root, "ready");
    const watcher = fs.watch(root);
    t.after(() => {
      controller.abort();
      if (fs.existsSync(ready)) {
        try { process.kill(Number(fs.readFileSync(ready, "utf8"))); } catch {}
      }
    });
    try {
      const started = new Promise<void>(resolve => watcher.on("change", () => { if (fs.existsSync(ready)) resolve(); }));
      const result = execCommand(process.execPath, ["-e", "const fs = require('node:fs'); fs.writeFileSync(process.argv[1] + '.tmp', String(process.pid)); fs.renameSync(process.argv[1] + '.tmp', process.argv[1]); setInterval(() => {}, 1000)", ready], root, controller.signal);
      await started;
      const pid = Number(fs.readFileSync(ready, "utf8"));
      controller.abort();
      const completed = await result;
      assert.notEqual(completed.code, 0);
      assert.throws(() => process.kill(pid, 0), /ESRCH|not found/i);
    } finally {
      watcher.close();
      fs.rmSync(root, { recursive: true });
    }
  });

  it("stops a real child that ignores SIGTERM", { timeout: 30000 }, async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "agnostic-resistant-child-"));
    const ready = path.join(root, "ready");
    const ignored = path.join(root, "ignored");
    const watcher = fs.watch(root);
    const controller = new AbortController();
    let pid: number | undefined;
    const result = execCommand(process.execPath, ["-e", "const fs = require('node:fs'); process.on('SIGTERM', () => fs.writeFileSync(process.argv[2], 'ignored')); fs.writeFileSync(process.argv[1] + '.tmp', String(process.pid)); fs.renameSync(process.argv[1] + '.tmp', process.argv[1]); setInterval(() => {}, 1000)", ready, ignored], root, controller.signal);
    try {
      await onFile(watcher, ready);
      pid = Number(fs.readFileSync(ready, "utf8"));
      controller.abort();
      const outcome = await Promise.race([result.then(() => "closed"), onFile(watcher, ignored).then(() => "ignored")]);
      assert.equal(outcome, "closed", "cancellation left a child running after it ignored SIGTERM");
      await exited(pid);
    } finally {
      if (pid) kill(pid);
      await result;
      watcher.close();
      fs.rmSync(root, { recursive: true });
    }
  });

  it("stops a real descendant instead of leaving it after its parent exits", { timeout: 30000 }, async () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "agnostic-descendant-"));
    const ready = path.join(root, "ready");
    const orphan = path.join(root, "orphan");
    const watcher = fs.watch(root);
    const controller = new AbortController();
    const descendant = "const fs = require('node:fs'); process.on('SIGTERM', () => {}); process.on('disconnect', () => fs.writeFileSync(process.argv[2], 'orphan')); fs.writeFileSync(process.argv[1] + '.tmp', JSON.stringify({parent: Number(process.argv[3]), child: process.pid})); fs.renameSync(process.argv[1] + '.tmp', process.argv[1]); setInterval(() => {}, 1000)";
    const parent = "require('node:child_process').spawn(process.execPath, ['-e', process.argv[1], process.argv[2], process.argv[3], String(process.pid)], {stdio: ['ignore', 'inherit', 'inherit', 'ipc']}); setInterval(() => {}, 1000)";
    let pids: { parent: number; child: number } | undefined;
    const result = execCommand(process.execPath, ["-e", parent, descendant, ready, orphan], root, controller.signal);
    try {
      await onFile(watcher, ready);
      pids = JSON.parse(fs.readFileSync(ready, "utf8"));
      controller.abort();
      const outcome = await Promise.race([result.then(() => "closed"), onFile(watcher, orphan).then(() => "orphan")]);
      assert.equal(outcome, "closed", "cancellation left a descendant running after its parent exited");
      if (pids) await exited(pids.child);
    } finally {
      if (pids) { kill(pids.child); kill(pids.parent); }
      await result;
      watcher.close();
      fs.rmSync(root, { recursive: true });
    }
  });
});
