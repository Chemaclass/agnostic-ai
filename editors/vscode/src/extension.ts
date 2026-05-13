// agnostic-ai VS Code extension entry point.
//
// Shells out to the user's installed `agnostic-ai` binary; ships no
// bundled binary, matching the v1 acceptance criteria. Three surfaces:
//
//   - Command palette entries for sync, sync --check, doctor --fix,
//     status, and "render current spec".
//   - Codelens above each spec with one "Render to <target>" action per
//     configured target.
//   - Status bar item that polls `sync --check --json` and shows the
//     current drift count.
//
// Schema-backed YAML editing is contributed declaratively via
// package.json -> contributes.yamlValidation, so that part requires no
// runtime code here.

import * as cp from "child_process";
import * as fs from "fs";
import * as path from "path";
import * as vscode from "vscode";

let statusBar: vscode.StatusBarItem | undefined;
let driftTimer: NodeJS.Timeout | undefined;

export function activate(context: vscode.ExtensionContext): void {
  const out = vscode.window.createOutputChannel("agnostic-ai");

  context.subscriptions.push(
    vscode.commands.registerCommand("agnostic-ai.sync", () =>
      runInTerminal("sync"),
    ),
    vscode.commands.registerCommand("agnostic-ai.syncCheck", () =>
      runInTerminal("sync --check"),
    ),
    vscode.commands.registerCommand("agnostic-ai.doctorFix", () =>
      runInTerminal("doctor --fix"),
    ),
    vscode.commands.registerCommand("agnostic-ai.status", () =>
      runInTerminal("status"),
    ),
    vscode.commands.registerCommand(
      "agnostic-ai.renderCurrent",
      async (target?: string, specPath?: string) =>
        renderCurrent(out, target, specPath),
    ),
  );

  const provider = new RenderCodeLensProvider();
  context.subscriptions.push(
    vscode.languages.registerCodeLensProvider(
      [
        { language: "markdown", scheme: "file" },
        { language: "yaml", scheme: "file" },
      ],
      provider,
    ),
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("agnostic-ai")) provider.refresh();
    }),
  );

  initStatusBar(context);
}

export function deactivate(): void {
  if (driftTimer) clearInterval(driftTimer);
  statusBar?.dispose();
}

// ---------------------------------------------------------------------------
// Process helpers
// ---------------------------------------------------------------------------

function binary(): string {
  return (
    vscode.workspace
      .getConfiguration("agnostic-ai")
      .get<string>("binaryPath") || "agnostic-ai"
  );
}

function projectRoot(): string | undefined {
  const folders = vscode.workspace.workspaceFolders;
  if (!folders || folders.length === 0) return undefined;
  for (const f of folders) {
    if (fs.existsSync(path.join(f.uri.fsPath, "agnostic.config.yaml"))) {
      return f.uri.fsPath;
    }
  }
  // Fall back to the first folder so commands run from a clean tree
  // still launch (the binary will surface its own error).
  return folders[0].uri.fsPath;
}

function runInTerminal(args: string): void {
  const cwd = projectRoot();
  if (!cwd) {
    vscode.window.showErrorMessage(
      "agnostic-ai: open a workspace folder first.",
    );
    return;
  }
  const term = vscode.window.createTerminal({
    name: "agnostic-ai",
    cwd,
  });
  term.sendText(`${binary()} ${args}`, true);
  term.show();
}

interface ExecResult {
  stdout: string;
  stderr: string;
  code: number;
}

function exec(args: string[], cwd: string): Promise<ExecResult> {
  return new Promise((resolve) => {
    cp.execFile(
      binary(),
      args,
      { cwd, maxBuffer: 4 * 1024 * 1024 },
      (err, stdout, stderr) => {
        let code = 0;
        if (err) {
          const errno = err as NodeJS.ErrnoException;
          if (errno.code === "ENOENT") {
            code = -1;
          } else {
            code = typeof err.code === "number" ? err.code : 1;
          }
        }
        resolve({
          stdout: stdout.toString(),
          stderr: stderr.toString(),
          code,
        });
      },
    );
  });
}

// ---------------------------------------------------------------------------
// Render current spec
// ---------------------------------------------------------------------------

async function renderCurrent(
  out: vscode.OutputChannel,
  target?: string,
  specPath?: string,
): Promise<void> {
  const cwd = projectRoot();
  if (!cwd) return;
  let active = specPath;
  if (!active) {
    const editor = vscode.window.activeTextEditor;
    if (!editor) {
      vscode.window.showErrorMessage(
        "agnostic-ai: open a spec file before rendering.",
      );
      return;
    }
    active = editor.document.uri.fsPath;
  }
  const rel = path.relative(cwd, active);
  const targets = await resolveTargets(cwd, target);
  if (!targets) return;
  out.show(true);
  out.appendLine(`# render ${rel} → ${targets.join(", ")}`);
  const res = await exec(["render", rel, "--target", targets.join(",")], cwd);
  if (res.code === -1) {
    showBinaryMissingError();
    return;
  }
  if (res.stdout) out.appendLine(res.stdout.replace(/\n$/, ""));
  if (res.stderr) out.appendLine(res.stderr.replace(/\n$/, ""));
}

async function resolveTargets(
  cwd: string,
  preselected?: string,
): Promise<string[] | undefined> {
  if (preselected) return [preselected];
  const targets = await listConfiguredTargets(cwd);
  if (targets.length === 0) {
    vscode.window.showErrorMessage(
      "agnostic-ai: no targets configured in agnostic.config.yaml.",
    );
    return undefined;
  }
  const pick = await vscode.window.showQuickPick(targets, {
    placeHolder: "Pick target(s) to render",
    canPickMany: true,
  });
  if (!pick || pick.length === 0) return undefined;
  return pick;
}

async function listConfiguredTargets(cwd: string): Promise<string[]> {
  const cfg = path.join(cwd, "agnostic.config.yaml");
  if (!fs.existsSync(cfg)) return [];
  const text = fs.readFileSync(cfg, "utf8");
  const targets: string[] = [];
  let inTargets = false;
  for (const line of text.split("\n")) {
    if (/^targets:\s*$/.test(line)) {
      inTargets = true;
      continue;
    }
    if (inTargets) {
      const m = /^\s+-\s+(\S+)/.exec(line);
      if (m) {
        targets.push(m[1]);
        continue;
      }
      if (/^\S/.test(line)) inTargets = false;
    }
  }
  return targets;
}

// ---------------------------------------------------------------------------
// Codelens
// ---------------------------------------------------------------------------

class RenderCodeLensProvider implements vscode.CodeLensProvider {
  private emitter = new vscode.EventEmitter<void>();
  readonly onDidChangeCodeLenses = this.emitter.event;

  refresh(): void {
    this.emitter.fire();
  }

  async provideCodeLenses(
    document: vscode.TextDocument,
  ): Promise<vscode.CodeLens[]> {
    const enabled = vscode.workspace
      .getConfiguration("agnostic-ai")
      .get<boolean>("codeLens.enabled", true);
    if (!enabled) return [];
    const cwd = projectRoot();
    if (!cwd) return [];
    const docPath = document.uri.fsPath;
    if (!isInsideSpecSources(cwd, docPath)) return [];
    const targets = await listConfiguredTargets(cwd);
    if (targets.length === 0) return [];
    const range = new vscode.Range(0, 0, 0, 0);
    return targets.map(
      (t) =>
        new vscode.CodeLens(range, {
          title: `Render to ${t}`,
          command: "agnostic-ai.renderCurrent",
          arguments: [t, docPath],
        }),
    );
  }
}

function isInsideSpecSources(root: string, file: string): boolean {
  const rel = path.relative(root, file);
  if (rel.startsWith("..") || path.isAbsolute(rel)) return false;
  // Match either the default `.agnostic-ai/<kind>/...` layout or the
  // top-level `<kind>/...` layout the legacy `init .` produces.
  const segs = rel.split(path.sep);
  const kinds = ["agents", "skills", "rules", "hooks", "mcps"];
  if (segs.length >= 2 && kinds.includes(segs[0])) return true;
  if (segs.length >= 3 && segs[0].startsWith(".") && kinds.includes(segs[1]))
    return true;
  return false;
}

// ---------------------------------------------------------------------------
// Status bar drift indicator
// ---------------------------------------------------------------------------

interface SyncCheckJSON {
  writes: { path: string; action: string }[];
  errors: { target: string; message: string }[];
}

function initStatusBar(context: vscode.ExtensionContext): void {
  statusBar = vscode.window.createStatusBarItem(
    vscode.StatusBarAlignment.Right,
    100,
  );
  statusBar.command = "agnostic-ai.syncCheck";
  statusBar.tooltip =
    "agnostic-ai drift count (click to run `sync --check`).";
  context.subscriptions.push(statusBar);
  refreshDrift();

  const seconds =
    vscode.workspace
      .getConfiguration("agnostic-ai")
      .get<number>("driftPollSeconds") || 30;
  driftTimer = setInterval(refreshDrift, Math.max(5, seconds) * 1000);
  context.subscriptions.push(
    new vscode.Disposable(() => {
      if (driftTimer) clearInterval(driftTimer);
    }),
    vscode.workspace.onDidSaveTextDocument(refreshDrift),
  );
}

async function refreshDrift(): Promise<void> {
  if (!statusBar) return;
  const cwd = projectRoot();
  if (!cwd) return;
  const res = await exec(["sync", "--check", "--json"], cwd);
  if (res.code === -1) {
    statusBar.text = "$(alert) agnostic-ai not found";
    statusBar.show();
    return;
  }
  let parsed: SyncCheckJSON;
  try {
    parsed = JSON.parse(res.stdout);
  } catch {
    statusBar.text = "$(alert) agnostic-ai parse error";
    statusBar.show();
    return;
  }
  const drift = parsed.writes ? parsed.writes.length : 0;
  statusBar.text =
    drift === 0
      ? "$(check) agnostic-ai: in sync"
      : `$(warning) agnostic-ai: ${drift} drifted`;
  statusBar.show();
}

function showBinaryMissingError(): void {
  vscode.window
    .showErrorMessage(
      "agnostic-ai: binary not found on PATH. Install it from the project README, then reload the window.",
      "Open install instructions",
    )
    .then((pick) => {
      if (pick) {
        vscode.env.openExternal(
          vscode.Uri.parse(
            "https://github.com/Chemaclass/agnostic-ai#install",
          ),
        );
      }
    });
}
