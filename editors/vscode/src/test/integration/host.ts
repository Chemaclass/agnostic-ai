import assert from "node:assert/strict";
import * as path from "node:path";
import * as fs from "node:fs";
import { createDiagnosticSession } from "../../diagnosticClient";
import * as vscode from "vscode";

function awaitDiagnostics(uri: vscode.Uri, matches: (diagnostics: readonly vscode.Diagnostic[]) => boolean): Promise<readonly vscode.Diagnostic[]> {
  return new Promise((resolve, reject) => {
    const check = () => {
      const found = vscode.languages.getDiagnostics(uri);
      if (matches(found)) { subscription.dispose(); clearTimeout(timeout); resolve(found); }
    };
    const subscription = vscode.languages.onDidChangeDiagnostics(check);
    const timeout = setTimeout(() => { subscription.dispose(); reject(new Error(`Timed out waiting for diagnostics at ${uri.fsPath}: ${JSON.stringify(vscode.languages.getDiagnostics(uri))}`)); }, 30000);
    check();
  });
}
async function replace(document: vscode.TextDocument, text: string): Promise<void> {
  const edit = new vscode.WorkspaceEdit();
  edit.replace(document.uri, new vscode.Range(document.positionAt(0), document.positionAt(document.getText().length)), text);
  assert.equal(await vscode.workspace.applyEdit(edit), true);
}

export async function run(): Promise<void> {
  const root = process.env.AGNOSTIC_AI_TEST_ROOT!;
  const external = process.env.AGNOSTIC_AI_TEST_EXTERNAL!;
  if (process.env.AGNOSTIC_AI_TEST_MODE === "cancellation") return cancellation(root);
  const extension = vscode.extensions.getExtension("Chemaclass.agnostic-ai");
  assert.ok(extension);
  await extension.activate();
  const rule = await vscode.workspace.openTextDocument(vscode.Uri.file(path.join(external, "empty.md")));
  const finding = await awaitDiagnostics(rule.uri, found => found.some(d => d.code === "LINT001"));
  assert.equal(finding[0].source, "agnostic-ai");
  console.log(`Problems open: ${rule.uri.toString()} ${finding[0].code} severity=${finding[0].severity} line=${finding[0].range.start.line}`);
  assert.equal(rule.languageId, "markdown");
  await replace(rule, "---\nname: empty\n---\nUse plain words.\n");
  assert.equal(vscode.languages.getDiagnostics(rule.uri).some(d => d.code === "LINT001"), true);
  const cleanRule = awaitDiagnostics(rule.uri, found => found.length === 0);
  await rule.save();
  await cleanRule;
  console.log("Problems save: Markdown finding cleared; unsaved fix retained previous finding.");

  const hooks = vscode.Uri.file(path.join(root, "custom hooks", "broken.yaml"));
  await vscode.workspace.fs.writeFile(hooks, Buffer.from("name: broken\non: stop\n: : :\n"));
  const hook = await vscode.workspace.openTextDocument(hooks);
  assert.equal(hook.languageId, "yaml");
  await awaitDiagnostics(hooks, found => found.some(d => d.code === "AAI-001"));
  console.log(`Problems YAML open: ${hooks.toString()} AAI-001`);
  await replace(hook, "name: broken\ndescription: Run after completion.\non: stop\ncommand: echo done\n");
  const cleanHook = awaitDiagnostics(hooks, found => found.length === 0);
  await hook.save();
  await cleanHook;

  const config = await vscode.workspace.openTextDocument(vscode.Uri.file(path.join(root, "agnostic-ai.yaml")));
  const goodConfig = config.getText();
  await replace(config, goodConfig + ": : :\n");
  const configError = awaitDiagnostics(config.uri, found => found.some(d => d.code === "AAI-004"));
  await config.save();
  await configError;
  console.log(`Problems config save: ${config.uri.toString()} AAI-004`);
  await replace(config, goodConfig);
  const cleanConfig = awaitDiagnostics(config.uri, found => found.length === 0);
  await config.save();
  await cleanConfig;

  const originalError = vscode.window.showErrorMessage;
  let reportFailure!: (message: string) => void;
  let failureTimeout: NodeJS.Timeout;
  const failedStartup = new Promise<string>((resolve, reject) => {
    reportFailure = resolve;
    failureTimeout = setTimeout(() => reject(new Error("Missing CLI did not report an actionable startup failure.")), 30000);
  });
  Object.defineProperty(vscode.window, "showErrorMessage", { configurable: true, value: (message: string, ...items: unknown[]) => {
    if (message.includes("missing CLI") && message.includes("binaryPath")) reportFailure(message);
    return Reflect.apply(originalError, vscode.window, [message, ...items]);
  } });
  try {
    await vscode.workspace.getConfiguration("agnostic-ai").update("binaryPath", path.join(root, "missing CLI"), vscode.ConfigurationTarget.Workspace);
    const reason = await failedStartup;
    assert.match(reason, /ENOENT/);
    assert.match(reason, /Install the CLI on PATH or correct agnostic-ai.binaryPath/);
    console.log(`Problems startup failure: ${reason}`);
  } finally {
    clearTimeout(failureTimeout!);
    Object.defineProperty(vscode.window, "showErrorMessage", { configurable: true, value: originalError });
  }
  await vscode.workspace.getConfiguration("agnostic-ai").update("binaryPath", process.env.AGNOSTIC_AI_TEST_BINARY!, vscode.ConfigurationTarget.Workspace);
  await replace(rule, "---\nname: empty\n---\n");
  const recovered = awaitDiagnostics(rule.uri, found => found.some(d => d.code === "LINT001"));
  await rule.save();
  await recovered;
  console.log("Problems restart: configured binary restored, findings recovered.");
  const entry: typeof import("../../extension") = require("../../extension");
  await entry.deactivate();
  await awaitDiagnostics(rule.uri, found => found.length === 0);
  console.log(`Problems shutdown: cleared through production deactivate, VS Code ${vscode.version}.`);
  await cancellation(root);
}

async function cancellation(root: string): Promise<void> {
  const dir = path.join(root, "cancel startup");
  fs.mkdirSync(dir);
  const parentPID = path.join(dir, "parent.pid");
  const childPID = path.join(dir, "child.pid");
  const readyPath = path.join(dir, "ready");
  const node = process.env.AGNOSTIC_AI_TEST_NODE!;
  const descendant = `process.on('SIGTERM',()=>{}); require('fs').writeFileSync(${JSON.stringify(childPID)},String(process.pid)); require('fs').writeFileSync(${JSON.stringify(readyPath)},'ready'); setInterval(()=>{},1000)`;
  fs.writeFileSync(path.join(dir, "lsp"), `process.on('SIGTERM',()=>{}); require('fs').writeFileSync(${JSON.stringify(parentPID)},String(process.pid)); require('child_process').spawn(${JSON.stringify(node)},['-e',${JSON.stringify(descendant)}],{stdio:'ignore'}); process.stdin.resume(); setInterval(()=>{},1000)`);
  const ready = new Promise<void>((resolve, reject) => {
    const watcher = fs.watch(dir, () => {
      if (fs.existsSync(readyPath)) { watcher.close(); clearTimeout(timeout); resolve(); }
    });
    const timeout = setTimeout(() => { watcher.close(); reject(new Error("Cancellation fixture never became ready.")); }, 30000);
  });
  const output = vscode.window.createOutputChannel("agnostic-ai cancellation test");
  const session = createDiagnosticSession({ binary: node, cwd: dir }, output, error => { throw error; });
  const unexpectedNotifications: string[] = [];
  const unhandled: unknown[] = [];
  const recordRejection = (reason: unknown) => { unhandled.push(reason); };
  process.on("unhandledRejection", recordRejection);
  const originalError = vscode.window.showErrorMessage;
  Object.defineProperty(vscode.window, "showErrorMessage", { configurable: true, value: (message: string) => {
    unexpectedNotifications.push(message);
    return Promise.resolve(undefined);
  } });
  const controller = new AbortController();
  let initialized = false;
  const pending = session.start(controller.signal).then(() => { initialized = true; }, () => undefined);
  try {
    await ready;
    const pids = [parentPID, childPID].map(file => Number(fs.readFileSync(file, "utf8")));
    controller.abort();
    await pending;
    await session.stop();
    for (const pid of pids) {
      assert.throws(() => process.kill(pid, 0), `owned process ${pid} survived cancellation`);
    }
    assert.deepEqual(unexpectedNotifications, [], "cancelled startup showed a stale raw error notification");
    assert.equal(initialized, false, "cancelled startup reported successful initialization");
    await new Promise<void>(resolve => setImmediate(resolve));
    for (const rejection of unhandled) console.error(rejection);
    assert.deepEqual(unhandled, [], "cancelled initialization leaked an unhandled rejection");
    console.log(`Production startup cancellation: parent ${pids[0]} and descendant ${pids[1]} exited despite ignoring SIGTERM.`);
  } finally {
    controller.abort();
    await pending;
    await session.stop();
    output.dispose();
    process.removeListener("unhandledRejection", recordRejection);
    Object.defineProperty(vscode.window, "showErrorMessage", { configurable: true, value: originalError });
  }
}
