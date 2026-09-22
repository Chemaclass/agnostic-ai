// Pure helpers behind "agnostic-ai: Open canonical source".
//
// Everything here is free of the `vscode` module so it runs under
// `node --test`. The extension asks the installed CLI for provenance
// (`why <file> --format json`), validates the reply, and only then turns
// the reported source paths into files to open. Source locations always
// come from the CLI: default or configured directories are never guessed.

import * as path from "path";

export const CONFIG_FILE_NAMES = ["agnostic-ai.yaml", "agnostic.config.yaml"];

export interface WhySource {
  kind: string;
  name: string;
  path: string;
  mode: string;
}

export interface WhyReport {
  file: string;
  target: string;
  sources: WhySource[];
}

export interface ProcessResult {
  stdout: string;
  stderr: string;
  code: number;
}

export interface PickItem {
  label: string;
  description: string;
  path: string;
}

export type NavigationPlan =
  | { kind: "open"; path: string; missing?: string[] }
  | { kind: "pick"; items: PickItem[]; missing?: string[] }
  | { kind: "error"; message: string };

export class WhyOutputError extends Error {}

/** The argument array for `why`, with the document relative to root. */
export function whyArgs(projectRoot: string, documentPath: string): string[] {
  return [
    "why",
    path.relative(projectRoot, documentPath),
    "--format",
    "json",
  ];
}

/**
 * Parses and validates `why --format json` output. Throws WhyOutputError
 * for anything that does not match the version 1 envelope, so a
 * malformed reply never reaches the code that opens files.
 */
export function parseWhyOutput(stdout: string): WhyReport {
  let raw: unknown;
  try {
    raw = JSON.parse(stdout);
  } catch {
    throw new WhyOutputError("the CLI did not return JSON");
  }
  if (!isObject(raw)) throw new WhyOutputError("expected a JSON object");
  if (raw.version !== "1") {
    throw new WhyOutputError(
      `unsupported output version ${JSON.stringify(raw.version)}`,
    );
  }
  if (raw.command !== "why") {
    throw new WhyOutputError(
      `expected command "why", got ${JSON.stringify(raw.command)}`,
    );
  }
  if (typeof raw.file !== "string" || typeof raw.target !== "string") {
    throw new WhyOutputError("missing file or target");
  }
  if (!Array.isArray(raw.sources)) {
    throw new WhyOutputError("missing sources array");
  }
  const sources = raw.sources.map((s: unknown, i: number): WhySource => {
    if (
      !isObject(s) ||
      !isNonEmptyString(s.kind) ||
      !isNonEmptyString(s.name) ||
      !isNonEmptyString(s.path) ||
      s.path.includes("\u0000") ||
      typeof s.mode !== "string"
    ) {
      throw new WhyOutputError(`source ${i} is malformed`);
    }
    return { kind: s.kind, name: s.name, path: s.path, mode: s.mode };
  });
  return { file: raw.file, target: raw.target, sources };
}

/**
 * Decides what to open for a validated report. Relative source paths
 * resolve against the project root the CLI ran in; absolute ones (specs
 * from another layer) are kept. Sources missing on disk are never
 * offered.
 */
export function planNavigation(
  report: WhyReport,
  projectRoot: string,
  exists: (p: string) => boolean,
): NavigationPlan {
  if (report.file === ".." || report.file.startsWith("../")) {
    return {
      kind: "error",
      message: `the CLI resolved this file outside the project (${report.file}), so its sources cannot be trusted.`,
    };
  }
  if (report.sources.length === 0) {
    return {
      kind: "error",
      message: `${report.file} has no source spec: the ${report.target} adapter emits it unconditionally.`,
    };
  }
  const sorted = [...report.sources].sort(
    (a, b) =>
      compare(a.kind, b.kind) ||
      compare(a.name, b.name) ||
      compare(a.path, b.path),
  );
  const items: PickItem[] = [];
  const missing: string[] = [];
  for (const s of sorted) {
    const abs = path.isAbsolute(s.path)
      ? s.path
      : path.join(projectRoot, s.path);
    if (!exists(abs)) {
      missing.push(s.path);
      continue;
    }
    items.push({ label: s.name, description: `${s.kind} · ${s.path}`, path: abs });
  }
  if (items.length === 0) {
    return {
      kind: "error",
      message: `source not found: ${missing.join(", ")}. Run \`agnostic-ai sync\` if the spec moved or was deleted.`,
    };
  }
  const extra = missing.length > 0 ? { missing } : {};
  if (items.length === 1) return { kind: "open", path: items[0].path, ...extra };
  return { kind: "pick", items, ...extra };
}

/**
 * Turns a failed `why` run into an actionable message. A missing binary
 * (code -1) is handled by the caller, which offers install instructions.
 */
export function describeWhyFailure(res: ProcessResult): string {
  const stderr = res.stderr.trim();
  if (/unknown command "why"|unknown flag: --format/.test(stderr)) {
    return "this agnostic-ai version cannot report provenance as JSON. Upgrade the CLI.";
  }
  if (stderr.includes("no sync state found")) {
    return "no sync state found for this project. Run `agnostic-ai sync` first.";
  }
  if (stderr.includes("not synced or not tracked")) {
    return "this file is not generated by agnostic-ai, or the last sync did not emit it.";
  }
  const first = stderr.split("\n")[0];
  return first ? `why failed: ${first}` : `why failed with exit code ${res.code}.`;
}

/**
 * Finds the nearest directory holding an agnostic-ai config, walking up
 * from the document. Stops at the workspace folder when one is given, so
 * a document resolves inside its own folder in a multi-root workspace.
 */
export function findProjectRoot(
  documentPath: string,
  workspaceFolder: string | undefined,
  exists: (p: string) => boolean,
): string | undefined {
  let dir = path.dirname(documentPath);
  for (;;) {
    if (CONFIG_FILE_NAMES.some((n) => exists(path.join(dir, n)))) return dir;
    if (workspaceFolder !== undefined && dir === workspaceFolder) return undefined;
    const parent = path.dirname(dir);
    if (parent === dir) return undefined;
    dir = parent;
  }
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function isNonEmptyString(v: unknown): v is string {
  return typeof v === "string" && v !== "";
}

function compare(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}
