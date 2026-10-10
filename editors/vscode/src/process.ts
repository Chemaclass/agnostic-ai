import * as cp from "node:child_process";

import { ProcessResult } from "./provenance";

export function execCommand(binary: string, args: string[], cwd: string, signal?: AbortSignal): Promise<ProcessResult> {
  if (signal) return driftCommand(binary, args, cwd, signal);
  return new Promise(resolve => {
    cp.execFile(binary, args, { cwd, env: { ...process.env, PWD: cwd }, maxBuffer: 4 * 1024 * 1024 }, (err, stdout, stderr) => {
      const code = err?.code === "ENOENT" ? -1 : typeof err?.code === "number" ? err.code : err ? 1 : 0;
      resolve({ stdout: stdout.toString(), stderr: stderr.toString(), code });
    });
  });
}

function driftCommand(binary: string, args: string[], cwd: string, signal: AbortSignal): Promise<ProcessResult> {
  if (signal.aborted) return Promise.resolve({ stdout: "", stderr: "Check cancelled.", code: 1 });
  return new Promise(resolve => {
    const child = cp.spawn(binary, args, {
      cwd,
      env: { ...process.env, PWD: cwd },
      detached: process.platform !== "win32",
      windowsHide: true,
    });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    let stdoutBytes = 0;
    let stderrBytes = 0;
    let failure: Error | undefined;
    let cancellation: Promise<string | undefined> | undefined;
    const cancel = () => {
      if (!cancellation && child.pid) {
        cancellation = stopDriftTree(child.pid).then(() => undefined, error => String(error));
      }
    };
    child.stdout.on("data", (chunk: Buffer) => {
      stdoutBytes += chunk.length;
      if (stdoutBytes <= 4 * 1024 * 1024) stdout.push(chunk);
      else { failure = new Error("Drift check stdout exceeded 4 MiB."); cancel(); }
    });
    child.stderr.on("data", (chunk: Buffer) => {
      stderrBytes += chunk.length;
      if (stderrBytes <= 4 * 1024 * 1024) stderr.push(chunk);
      else { failure = new Error("Drift check stderr exceeded 4 MiB."); cancel(); }
    });
    child.once("error", error => { failure = error; });
    signal.addEventListener("abort", cancel, { once: true });
    child.once("close", code => {
      signal.removeEventListener("abort", cancel);
      // Cancellation must finish before a replacement check starts.
      void Promise.resolve(cancellation).then(cancelError => {
        const missing = failure && "code" in failure && failure.code === "ENOENT";
        resolve({
          stdout: Buffer.concat(stdout).toString(),
          stderr: cancelError ?? failure?.message ?? Buffer.concat(stderr).toString(),
          code: missing ? -1 : failure || cancelError ? 1 : code ?? 1,
        });
      });
    });
  });
}

function stopDriftTree(pid: number): Promise<void> {
  if (process.platform === "win32") {
    return new Promise((resolve, reject) => {
      cp.execFile("taskkill", ["/PID", String(pid), "/T", "/F"], { windowsHide: true }, (error, _stdout, stderr) => {
        if (error) reject(new Error(`stop drift process tree ${pid}: ${stderr.trim() || error.message}`));
        else resolve();
      });
    });
  }
  try {
    process.kill(-pid, "SIGKILL");
    return Promise.resolve();
  } catch (error) {
    if (error instanceof Error && "code" in error && error.code === "ESRCH") return Promise.resolve();
    return Promise.reject(new Error(`stop drift process group ${pid}: ${String(error)}`));
  }
}
