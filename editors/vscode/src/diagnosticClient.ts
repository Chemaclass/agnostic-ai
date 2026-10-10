import * as cp from "node:child_process";
import * as path from "node:path";
import * as vscode from "vscode";
import { CloseAction, ErrorAction, LanguageClient, State } from "vscode-languageclient/node";

import { stopOwnedProcessTree } from "./process";
import { DiagnosticContext, DiagnosticSession, isDiagnosticFile } from "./sourceDiagnostics";

class DiagnosticClient extends LanguageClient {
  override async start(): Promise<void> {
    const starting = super.start();
    // Keep the initialization rejection observed when connection close resets its promise.
    const initialized = super.start();
    await Promise.all([starting, initialized]);
  }

  override error(message: string, data?: unknown, _showNotification?: boolean | "force"): void {
    super.error(message, data, false);
  }

  override stop(timeout?: number): Promise<void> {
    if (this.state === State.Starting) return Promise.resolve();
    return super.stop(timeout);
  }
}

export function diagnosticFailure(context: DiagnosticContext, error: unknown): string {
  const reason = error instanceof Error ? error.message : String(error);
  return `agnostic-ai: cannot run ${context.binary} lsp in ${context.cwd}: ${reason}. Install the CLI on PATH or correct agnostic-ai.binaryPath, then reload the window.`;
}

export function createDiagnosticSession(
  context: DiagnosticContext,
  output: vscode.OutputChannel,
  failure: (error: unknown) => void,
): DiagnosticSession {
  let child: cp.ChildProcessWithoutNullStreams | undefined;
  let closed: Promise<void> = Promise.resolve();
  let exited = false;
  let stopped = false;
  let cancellation: Promise<void> | undefined;
  const terminate = (): Promise<void> => {
    if (!cancellation && child?.pid && (!exited || globalThis.process.platform !== "win32")) cancellation = stopOwnedProcessTree(child.pid);
    return cancellation ?? Promise.resolve();
  };
  const client = new DiagnosticClient("agnostic-ai", "agnostic-ai", async () => {
    if (stopped) throw new Error("Diagnostics startup cancelled.");
    const process = cp.spawn(context.binary, ["lsp"], {
      cwd: context.cwd,
      env: { ...globalThis.process.env, PWD: context.cwd },
      detached: globalThis.process.platform !== "win32",
      windowsHide: true,
    });
    child = process;
    closed = new Promise<void>(resolve => process.once("close", () => { exited = true; resolve(); }));
    process.stderr.on("data", (data: Buffer) => output.append(data.toString()));
    await new Promise<void>((resolve, reject) => {
      process.once("spawn", resolve);
      process.once("error", reject);
    });
    if (stopped) { await terminate(); throw new Error("Diagnostics startup cancelled."); }
    return { reader: process.stdout, writer: process.stdin, detached: true };
  }, {
    documentSelector: [{ scheme: "file", pattern: "**/*.{md,mdc,yaml,yml}" }],
    workspaceFolder: { uri: vscode.Uri.file(context.cwd), name: path.basename(context.cwd), index: 0 },
    outputChannel: output,
    diagnosticCollectionName: "agnostic-ai",
    initializationFailedHandler: () => false,
    middleware: {
      didChange: async () => undefined,
      handleDiagnostics(uri, diagnostics, next) {
        if (!stopped && isDiagnosticFile(uri)) next(uri, diagnostics);
      },
    },
    errorHandler: {
      error: () => ({ action: ErrorAction.Shutdown, handled: true }),
      closed: () => {
        if (!stopped && client.isRunning()) failure(new Error("The diagnostics process exited. Reload the window to restart it."));
        return { action: CloseAction.DoNotRestart, handled: true };
      },
    },
  });
  return {
    async start(signal) {
      const cancel = () => {
        stopped = true;
        client.diagnostics?.clear();
        void terminate().catch(error => output.appendLine(String(error)));
      };
      signal.addEventListener("abort", cancel, { once: true });
      try {
        if (signal.aborted) cancel();
        if (stopped) throw new Error("Diagnostics startup cancelled.");
        await client.start();
        if (stopped || signal.aborted) throw new Error("Diagnostics startup cancelled.");
        if (!client.isRunning()) throw new Error("The CLI exited before diagnostics initialization completed.");
      } finally {
        signal.removeEventListener("abort", cancel);
      }
    },
    async stop() {
      stopped = true;
      client.diagnostics?.clear();
      try {
        await terminate();
        await closed;
        try {
          if (client.isRunning()) await client.stop();
        } catch (error) {
          output.appendLine(`Stop diagnostics client: ${String(error)}`);
        }
      } finally {
        client.diagnostics?.dispose();
      }
    },
  };
}
