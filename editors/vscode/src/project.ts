// Pure project helpers shared by every extension surface.
//
// Free of the `vscode` module so it runs under `node --test`. A project
// is a directory holding `agnostic-ai.yaml` or the legacy
// `agnostic.config.yaml`; when both exist the first wins, matching the
// CLI's own lookup order.

import * as path from "path";

export const CONFIG_FILE_NAMES = ["agnostic-ai.yaml", "agnostic.config.yaml"];

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
  return folders.find((f) => findConfigFile(f, exists) !== undefined) ?? folders[0];
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

/** The entries of the top-level `targets:` list in a config file. */
export function parseTargets(text: string): string[] {
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
