+++
title = "Error codes"
description = "Find an AAI error code, its cause, and the command or edit that fixes it."
weight = 150

[extra]
group = "Reference"
+++

# Error codes

Each agnostic-ai error starts with an `[AAI-NNN]` code and ends with its fix. Find the code below for the cause. For a symptom without a code, use [Troubleshooting](@/docs/troubleshooting.md).

```
[AAI-003] read config: no agnostic-ai.yaml or agnostic.config.yaml in /path/to/project
  fix: Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.
```

`agnostic-ai explain AAI-003` prints the same cause and fix as the entry below. Pass `--json` for JSON output.

## Numbering

| Range     | Area                       |
| --------- | -------------------------- |
| `001-099` | reading specs and config |
| `100-199` | writing output (collisions, hooks) |
| `200-299` | import                     |
| `300-399` | sync and validate |

Codes keep their number across releases. New codes are added at the end.

## Codes

### AAI-001: Spec parse failed

A spec file, or a review `@path` include, could not be read. Markdown specs use YAML frontmatter; hooks and MCPs are plain YAML. The error gives the path and, when known, the line and column.

**Fix:** at the reported position, check that `---` lines wrap the frontmatter and that the YAML is valid: correct indentation, no tabs, quotes where needed. For an include, create the file or use a path inside the project.

### AAI-002: Spec kind not supported by target

Your project has a spec kind (hook, mcp, command, ...) that the tool does not support. This is a warning by default, and a failure with `on-unsupported: error`.

**Fix:** remove the spec, use a tool that supports the kind, or set `on-unsupported: warn` (or `silent`) in `agnostic-ai.yaml`.

### AAI-003: Config file missing

Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

**Fix:** run `agnostic-ai init` to create a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-004: Config decode failed

The config file is not valid YAML, has keys the config does not accept, or holds a value of the wrong shape. Each finding names its file, line, and dotted path, such as `agnostic-ai.yaml:4: unknown key "sync.collsion-policy" (did you mean collision-policy?)` or `agnostic-ai.yaml:2: gitignore: expected a map, got the string "yes-please"`. A mistake in `agnostic-ai.local.yaml` names that file and its own line. A `version` other than `1` fails here too, as does a [`requires`](@/docs/configuration.md#requires) value that is not a version constraint, such as `latest` or `>=0.73.0,<0.74.0`, fails here too.

**Fix:** go to the file and line the message names. Give the key the shape it expects, rename or remove an unknown key (using its did-you-mean suggestion when there is one), or set `version: 1`. Check indentation, or check the file against `docs/schemas/config.schema.json`. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-005: Installed version outside requires

The installed binary is outside the releases the config's [`requires`](@/docs/configuration.md#requires) key allows. Every command that reads the specs stops before it reads them or writes files, including `sync`, `lint`, `validate`, `doctor`, `revert`, and `cleanup`. The message names the file, the required version, and the installed one:

```
[AAI-005] agnostic-ai.yaml requires agnostic-ai >=0.71.0, but 0.70.0 is installed; run `agnostic-ai upgrade`
```

**Fix:** to adopt a newer installed release, run `agnostic-ai upgrade --requires` from the project root (through your package manager for a project install, such as `pnpm exec agnostic-ai upgrade --requires`). It sets `requires` and the schema tag to the installed release, then syncs.

- **Installed release is older:** run `agnostic-ai upgrade`, or `upgrade --version vX.Y.Z` for an exact pin or range. For a package-manager install, the message names the install command, such as `pnpm install`.
- **You want to keep an older pin:** install the release it names through your package manager or `upgrade --version`, which also downgrades a standalone binary.
- **Global home config:** edit `requires` in the named file to adopt the installed release. `upgrade --requires` changes project config only.
- **The version does not change:** `agnostic-ai upgrade --check` shows which binary runs and any older copy that shadows it on PATH.

### AAI-102: Tools write to the same file {#aai-102-targets-emit-to-the-same-output-path}

Two or more enabled targets would write different content to the same path, so sync stops instead of letting the last one win.

**Fix:** drop one of the colliding targets from `targets:` in agnostic-ai.yaml, or override the matching `outputs.<target>` path setting, such as `file`, `rules-file`, or `skills-dir`.

<a id="aai-103-hand-authored-ignore-file-cannot-be-safely-overwritten"></a>

### AAI-103: Hand-written ignore file would lose patterns {#aai-103-hand-authored-ignore-file-would-lose-patterns}

Replacing a tool's ignore file that has no agnostic-ai header would remove or reorder patterns, or add a negation. That can make excluded files readable, so sync refuses. The error names a risky negation or up to five missing or reordered patterns, with a count for the rest.

**Fix:** run `agnostic-ai import <target>` to copy the file's patterns into an ignore spec. When a tool reads several ignore files, combine their patterns in the spec, in the same order, before syncing. Deleting the file also clears the error, at the cost of those patterns.

See [ignore overwrite behavior](@/docs/spec-format/ignore.md#overwrite-behaviour).

### AAI-202: Import source name unknown

The tool name passed to `agnostic-ai import` is not supported.

**Fix:** run `agnostic-ai import --help` for the supported list, and check the spelling.

### AAI-203: Import would replace an existing spec

`import`, `init --from`, or `use` would replace a spec with content the importing tool never read: a hand-written spec, one edited since the last sync or import (a comment counts), one the last sync never wrote for that tool, or one another tool's import wrote.

It also stops when a tool edit would replace a spec that holds `::target` blocks, or a key or comment it could not carry back. The import writes no spec, and `import --dry-run` fails the same way. The message lists each spec, the tool its current content came from, the tool that wanted it, and what the spec would lose.

**Fix:** rename the existing spec to keep both and import again, or run `agnostic-ai import <tool> --overwrite` to replace it. When the message names what the spec would lose, make the tool's edit in the spec instead to keep it.

### AAI-301: Unknown sync target

A target named in `--target`, `--only`, or the config is not built in, and no `agnostic-ai-adapter-<name>` binary is on PATH.

A name passed with `--target` fails the run. A config target that does not resolve only warns, so a teammate without an external adapter can still sync. When the name is close to a built-in name, the message suggests it: `unknown target: claud (did you mean claude? no agnostic-ai-adapter-claud on PATH)`.

A known target that is not in this run (`sync --only codex` with `targets: [claude]`) reports `codex is not in this run's targets (claude)`.

**Fix:** check the spelling. If the target exists but is not in this run, add it to `targets` or pass it with `-t`. Use the names in [Supported tools](@/docs/targets/_index.md). External adapters live on PATH as `agnostic-ai-adapter-<name>`.

### AAI-302: Mutually exclusive flags

You passed two conflicting flags, such as `--only` with `--except` or `--watch` with `--check`, or a flag without the one it needs, such as `--diff` without `--dry-run`.

**Fix:** the message names both flags. Drop one when they conflict, or add the missing one.
