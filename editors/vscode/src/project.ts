// Pure project helpers shared by every extension surface.
//
// Free of the `vscode` module so it runs under `node --test`. A project
// is a directory holding `agnostic-ai.yaml` or the legacy
// `agnostic.config.yaml`; when both exist the first wins, matching the
// CLI's own lookup order.

import * as path from "path";

export const CONFIG_FILE_NAMES = ["agnostic-ai.yaml", "agnostic.config.yaml"];

/** The per-developer file the CLI merges over the base config. */
export const LOCAL_OVERRIDE_FILE_NAME = "agnostic-ai.local.yaml";

/** The config file in dir, preferring agnostic-ai.yaml like the CLI. */
export function findConfigFile(
  dir: string,
  exists: (p: string) => boolean,
): string | undefined {
  return CONFIG_FILE_NAMES.map((n) => path.join(dir, n)).find(exists);
}

/**
 * The first workspace folder holding a config. Falls back to the first
 * folder so commands run from a clean tree still launch (the binary
 * will surface its own error).
 */
export function pickWorkspaceRoot(
  folders: string[],
  exists: (p: string) => boolean,
): string | undefined {
  const withConfig = folders.find(
    (f) => findConfigFile(f, exists) !== undefined,
  );
  return withConfig ?? folders[0];
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
    if (findConfigFile(dir, exists) !== undefined) return dir;
    if (workspaceFolder !== undefined && dir === workspaceFolder) return undefined;
    const parent = path.dirname(dir);
    if (parent === dir) return undefined;
    dir = parent;
  }
}

/**
 * The entries of the top-level `targets:` list, in block or flow style.
 * Undefined when the file sets no targets or uses a shape this line
 * reader cannot follow, so a caller falls back instead of showing none.
 */
export function parseTargetList(text: string): string[] | undefined {
  const all = text.split(/\r?\n/);
  // The CLI reads only the first YAML document.
  const docEnd = all.findIndex((l, i) => i > 0 && /^(---|\.\.\.)(\s|$)/.test(l));
  const lines = (docEnd < 0 ? all : all.slice(0, docEnd)).map((l) =>
    l.replace(/(^|\s)#.*$/, "").trimEnd(),
  );
  const start = lines.findIndex((l) => l.startsWith("targets:"));
  if (start < 0) return undefined;
  const value = lines[start].slice("targets:".length).trim();
  if (value === "") return blockItems(lines.slice(start + 1));
  if (value === "null" || value === "~") return [];
  if (!value.startsWith("[")) return undefined;
  const flow = [value, ...lines.slice(start + 1)].join(" ");
  const end = flow.indexOf("]");
  if (end < 0) return undefined;
  return targetNames(
    flow
      .slice(1, end)
      .split(",")
      .map(unquote)
      .filter((t) => t !== ""),
  );
}

// An alias, escape, or nested collection is not a plain target name, so
// the whole list is unreadable here.
function targetNames(items: string[]): string[] | undefined {
  return items.every((t) => /^[A-Za-z0-9][A-Za-z0-9_.-]*$/.test(t))
    ? items
    : undefined;
}

function blockItems(lines: string[]): string[] | undefined {
  const items: string[] = [];
  for (const line of lines) {
    if (line === "") continue;
    const m = /^\s*-\s+(\S+)$/.exec(line);
    if (m) {
      items.push(unquote(m[1]));
      continue;
    }
    if (/^[^\s-]/.test(line)) break;
    return undefined;
  }
  return targetNames(items);
}

/** The entries of the top-level `targets:` list in a config file. */
export function parseTargets(text: string): string[] {
  return parseTargetList(text) ?? [];
}

/**
 * The targets sync uses: a `targets:` list in the local override replaces
 * the base list, as the CLI's merge does.
 */
export function configuredTargets(base: string, local?: string): string[] {
  return (
    (local === undefined ? undefined : parseTargetList(local)) ??
    parseTargets(base)
  );
}

function unquote(value: string): string {
  return value.trim().replace(/^(["'])(.*)\1$/, "$2");
}
