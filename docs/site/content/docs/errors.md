+++
title = "Error codes"
description = "Find an AAI error code, its cause, and the command or edit that fixes it."
weight = 150

[extra]
group = "Reference"
+++

# Error codes

Each error starts with an `[AAI-NNN]` code and ends with its fix. Find the code below. For a symptom without a code, see [Troubleshooting](@/docs/troubleshooting.md).

```
[AAI-003] read config: no agnostic-ai.yaml or agnostic.config.yaml in /path/to/project
  fix: Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.
```

`agnostic-ai explain AAI-003` prints the same cause and fix (`--json` for JSON).

## Numbering

| Range     | Area                       |
| --------- | -------------------------- |
| `001-099` | reading specs and config |
| `100-199` | writing output (collisions, hooks) |
| `200-299` | import                     |
| `300-399` | sync and validate |

Codes keep their number across releases; new ones are added at the end.

## Codes

### AAI-001: Spec parse failed

A spec file, or a review `@path` include, could not be read. Markdown specs use YAML frontmatter; hooks and MCPs are plain YAML. The error gives the path and, when known, line and column.

**Fix:** at that position, check that `---` lines wrap the frontmatter and the YAML is valid (indentation, no tabs, quotes). For an include, create the file or use a path inside the project.

### AAI-002: Spec kind not supported by target

A spec kind (hook, mcp, command, ...) is not supported by the tool. A warning by default; a failure with `on-unsupported: error`.

**Fix:** remove the spec, use a tool that supports the kind, or set `on-unsupported: warn` (or `silent`) in `agnostic-ai.yaml`.

### AAI-003: Config file missing

Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

**Fix:** run `agnostic-ai init` to create a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-004: Config decode failed

The config has invalid YAML, an unknown key, or a value of the wrong type. Each finding names its file, line, and dotted path, such as `agnostic-ai.yaml:4: unknown key "sync.collsion-policy" (did you mean collision-policy?)`. A mistake in `agnostic-ai.local.yaml` names that file and its own line.

<details>
<summary>More causes and message examples</summary>

A type error reads like `agnostic-ai.yaml:2: gitignore: expected a map, got the string "yes-please"`. A `version` other than `1` fails here too, as does a [`requires`](@/docs/configuration.md#requires) value that is not a version constraint, such as `latest` or `>=0.92.0,<0.93.0`.

</details>

**Fix:** at the reported line, correct the value type, rename or remove the unknown key (use the suggestion), or set `version: 1`. Check indentation, or validate against `docs/schemas/config.schema.json`. `agnostic-ai doctor` gives a full diagnosis.

### AAI-005: Installed version outside requires

The installed release does not satisfy the config's [`requires`](@/docs/configuration.md#requires) value (an exact pin blocks newer releases too). Every command that reads the specs, such as `sync`, `lint`, `validate`, `doctor`, `revert`, and `cleanup`, stops before it reads them or writes files:

```
[AAI-005] agnostic-ai.yaml + agnostic-ai.local.yaml requires agnostic-ai <required>, but <installed> is installed.
  The installed release is newer than this project allows.
  fix: run `agnostic-ai upgrade --requires` to adopt installed agnostic-ai <installed>
       This updates requires and the schema URL, then runs sync.
  To accept newer releases when generated files are git-ignored, set `requires: ">=<required>"` in the file that pins it.
```

**Fix:** to adopt a newer installed release, run `agnostic-ai upgrade --requires` from the project root (for a project install, through your package manager, such as `pnpm exec agnostic-ai upgrade --requires`). It raises `requires` to the installed release, keeps its form (a minimum stays a minimum, an exact release stays exact, a range becomes a minimum), updates the schema tag, then syncs. `upgrade` alone updates the tool, not the project. With git-ignored generated files, prefer `requires: ">=X.Y.Z"` over an exact release. `agnostic-ai explain AAI-005` lists every fix.

| Situation | What to do |
| --- | --- |
| Installed release is older | Run `agnostic-ai upgrade`, or `upgrade --version vX.Y.Z` for an exact pin or range. For a package-manager install, the message names the install command, such as `pnpm install`. |
| You want to keep an older pin | Install the release it names through your package manager or `upgrade --version`, which also downgrades a standalone binary. |
| Global home config | Edit `requires` in the named file. `upgrade --requires` changes project config only. |
| The version does not change | `agnostic-ai upgrade --check` shows which binary runs and any older copy that shadows it on PATH. |

<details>
<summary>The interactive menu after a version mismatch</summary>

In an interactive terminal, a project version mismatch offers a menu:

```text
Choose a next step:
  1. Apply suggested fix (update project requires and schema, then sync)
  2. Open Claude Code with a fix prompt
  3. Open Codex with a fix prompt
  4. Show the fix prompt to copy
  0. Exit
Choice [0]:
```

Option 1 appears when the installed release is newer than the project allows, and runs `upgrade --requires`. Options 2 and 3 appear when that tool is in the project's merged `targets` and on `PATH`; they open an interactive session in the project directory with the error and fix, under the tool's normal permissions. Use option 4 for another tool.

Enter or `0` exits without action. Every option keeps the original error status and does not rerun the failed command, so `agnostic-ai sync && git push` stops either way. Review the changes and rerun your command after the fix. The menu is skipped for redirected input or output, CI, quiet or JSON output, global commands, checks, previews, watch mode, hooks, and editor services.

</details>

### AAI-102: Tools write to the same file {#aai-102-targets-emit-to-the-same-output-path}

Two or more enabled targets would write different content to the same path, so sync stops instead of letting the last one win.

<details>
<summary>Other collisions that stop sync</summary>

- An adapter writes a file that sync then writes as another target's instructions file (such as an external adapter's `AGENTS.md`), or a merged `rules-file` loses rules there (`outputs.codex.rules-file: AGENTS.md` while Cline reads `AGENTS.md`). `sync.collision-policy` does not settle these; the instructions file is written last.
- One output sits at another output's `<path>.bak`, such as `outputs.claude.rules-file: RULES.md.bak` next to `outputs.codex.rules-file: RULES.md`. Sync saves a hand edit to `RULES.md` there. `sync` and `validate` stop.
- An output sits on a `.bak` that sync saved earlier and that still exists. `sync`, `sync --check` and `status` report it; `validate` does not read the sync ledger.
- Case is ignored for `.bak` matches, since some filesystems store `rules.md.BAK` and `RULES.md.bak` as one file. Folder settings such as `skills-dir` get no `.bak`.
- On filesystems that ignore case (macOS and Windows by default), two outputs that differ only in case also stop `sync`, `status` and `validate`, such as `outputs.codex.rules-file: agents.md` next to Amp's `AGENTS.md`.
- Paths are compared as written, case ignored. A path reached through a symlinked folder or another Unicode spelling is not caught.

</details>

**Fix:** drop one of the colliding targets from `targets:` in agnostic-ai.yaml, or override the matching `outputs.<target>` path setting, such as `file`, `rules-file`, or `skills-dir`. For a shared instructions file, point the `rules-file` elsewhere, remove it, or set `outputs.<target>.file` for the other tool. For a saved `.bak`, keep what you need from it, then move or delete it, or point the output elsewhere. For outputs that differ only in case, spell both paths the same way.

<a id="aai-103-hand-authored-ignore-file-cannot-be-safely-overwritten"></a>

### AAI-103: Hand-written ignore file would lose patterns {#aai-103-hand-authored-ignore-file-would-lose-patterns}

Replacing a tool's ignore file that has no agnostic-ai header would remove or reorder patterns, or add a negation, which can make excluded files readable. Sync refuses and names a risky negation or up to five missing or reordered patterns, plus a count for the rest.

**Fix:** run `agnostic-ai import <target>` to copy the file's patterns into an ignore spec. When a tool reads several ignore files, combine their patterns in the spec, in the same order, before syncing. Deleting the file also clears the error, at the cost of those patterns.

See [ignore overwrite behavior](@/docs/spec-format/ignore.md#overwrite-behaviour).

### AAI-202: Import source name unknown

The tool name passed to `agnostic-ai import` is not supported.

**Fix:** run `agnostic-ai import --help` for the supported list, and check the spelling.

### AAI-203: Import would replace an existing spec

`import`, `init --from`, or `use` would replace a spec with content the tool never read: a hand-written spec, one edited since the last sync or import (a comment counts), one the last sync never wrote for that tool, or one another tool's import wrote.

<details>
<summary>Other cases and what the message lists</summary>

It also stops when a tool edit would replace a spec that holds `::target` blocks, or a key or comment it could not carry back. The import writes no spec, and `import --dry-run` fails the same way. The message lists each spec, the tool its current content came from, the tool that wanted it, and what the spec would lose.

</details>

**Fix:** rename the existing spec to keep both and import again, or run `agnostic-ai import <tool> --overwrite` to replace it. When the message names what the spec would lose, make the tool's edit in the spec instead to keep it.

### AAI-301: Unknown sync target

A target named in `--target`, `--only`, or the config is not built in, and no `agnostic-ai-adapter-<name>` binary is on PATH.

A name passed with `--target` fails the run. A config target that does not resolve only warns, so a teammate without an external adapter can still sync.

<details>
<summary>Messages and what sync keeps</summary>

A sync with an unresolved config target keeps the missing target's files and prints `~ skipped removing stale files: <target> did not load; a sync that loads every target removes them`. When the name is close to a built-in name, the message suggests it: `unknown target: claud (did you mean claude? no agnostic-ai-adapter-claud on PATH)`.

A known target that is not in this run (`sync --only codex` with `targets: [claude]`) reports `codex is not in this run's targets (claude)`.

</details>

**Fix:** check the spelling. If the target exists but is not in this run, add it to `targets` or pass it with `-t`. Use the names in [Supported tools](@/docs/targets/_index.md). External adapters live on PATH as `agnostic-ai-adapter-<name>`.

### AAI-302: Mutually exclusive flags

You passed two conflicting flags, such as `--only` with `--except` or `--watch` with `--check`, or a flag without the one it needs, such as `--diff` without `--dry-run`.

**Fix:** the message names both flags. Drop one when they conflict, or add the missing one.

### AAI-303: Empty targets list

`sync --check` ran while `targets` lists no tool. Sync keeps every generated file in that case, so the check cannot say whether they are current and fails instead of passing.

**Fix:** add a tool to `targets`, or run `agnostic-ai sync --allow-empty-targets` to remove the generated files. Add `--dry-run` first to see which files go.
