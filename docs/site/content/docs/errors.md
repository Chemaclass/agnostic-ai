+++
title = "Error codes"
description = "Every AAI-NNN diagnostic, what causes it, and the fix."
weight = 150

[extra]
group = "Reference"
+++

# Error codes


Every user-facing error starts with a stable code in square brackets, `[AAI-NNN]`. Its fix follows on the next line:

```
[AAI-003] read config: no agnostic-ai.yaml or agnostic.config.yaml in /path/to/project
  fix: Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.
```

Look up a code from the terminal:

```
$ agnostic-ai explain AAI-003
AAI-003: Config file missing

Cause:
  Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

Fix:
  Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.
```

Pass `--json` for machine-readable output.

## Numbering

| Range     | Area                       |
| --------- | -------------------------- |
| `001-099` | spec / config load + parse |
| `100-199` | emit (collisions, hooks)   |
| `200-299` | import                     |
| `300-399` | sync / validate            |

Codes are stable across releases. New codes are added at the end; existing codes never change number.

## Codes

### AAI-001: Spec parse failed

A spec file could not be parsed. Markdown specs use YAML frontmatter; hooks and MCPs are pure YAML. The error gives the path and, when available, the line:col of the offending byte. A review `@path` include that cannot be read reports here too.

**Fix:** open the file at the reported position. Check that the frontmatter delimiters (`---`) wrap the metadata and that the YAML is valid: correct indentation, no tabs, quoted strings where needed. For an include, create the file or use a path inside the project.

### AAI-002: Spec kind not supported by target

A spec kind (hook, mcp, command, ...) is in the bundle, but the target adapter does not emit it. By default this logs a warning. `on-unsupported: error` makes it a hard failure.

**Fix:** drop the spec, switch to a target that supports the kind, or set `on-unsupported: warn` (or `silent`) in `agnostic-ai.yaml`.

### AAI-003: Config file missing

Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

**Fix:** run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-004: Config decode failed

The config file was found, but it is not valid YAML or its keys do not match the schema. Each unknown key is named with its file, line, and dotted path, such as `agnostic-ai.yaml:4: unknown key "sync.collsion-policy" (did you mean collision-policy?)`. A [`requires`](@/docs/configuration.md#requires) value that is not a version constraint, such as `latest` or `>=0.73.0,<0.74.0`, fails here too.

**Fix:** rename or remove each unknown key the message names. Use its did-you-mean suggestion when there is one. Otherwise validate against `docs/schemas/config.schema.json`. Check indentation and that list keys (e.g. `targets:`) hold a YAML sequence. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-005: Installed version outside requires

The config's `requires` key names the agnostic-ai releases its specs work with: a minimum, one exact release, or a range. The installed binary is outside it. Every command that reads the specs stops before it reads specs or writes files. That includes `sync`, `lint`, `validate`, `doctor`, `revert`, and `cleanup`. The message names the file, the required version, and the installed one:

```
[AAI-005] agnostic-ai.yaml requires agnostic-ai >=0.71.0, but 0.70.0 is installed; run `agnostic-ai upgrade`
```

**Fix:** after installing a newer release, run `agnostic-ai upgrade --requires` from the project root. It aligns `requires` and the schema tag with the installed release, then syncs. Use the installed package-manager CLI, such as `pnpm exec agnostic-ai upgrade --requires`.

- **Older installed release:** run `agnostic-ai upgrade`, or `upgrade --version vX.Y.Z` for an exact pin or range. Project package-manager installs name their install command, such as `pnpm install`, after pulling a version bump.
- **Keep an intentional older project pin:** install the release it names through your package manager or `upgrade --version`. The latter also downgrades a standalone binary.
- **Global home config:** edit `requires` in the named file to adopt the installed release. `upgrade --requires` changes project config only.
- **The version does not change:** `agnostic-ai upgrade --check` shows which binary runs and any older copy that shadows it on PATH.

### AAI-102: Targets emit to the same output path

Two or more enabled targets would write different content to the same path. Last-writer-wins would mask drift.

**Fix:** Drop one of the colliding targets from `targets:` in agnostic-ai.yaml, or override the matching `outputs.<target>` path setting, such as `file`, `rules-file`, or `skills-dir`.

<a id="aai-103-hand-authored-ignore-file-cannot-be-safely-overwritten"></a>

### AAI-103: Hand-authored ignore file would lose patterns

Replacing a target's ignore file without an agnostic-ai header would remove or reorder existing patterns, or add a negation. These changes can make excluded files readable, so sync refuses the overwrite.

**Fix:** Run `agnostic-ai import <target>` to copy the imported file's patterns into an ignore spec. When a target reads several ignore files, combine their patterns in the spec and preserve their order before syncing. The error names a risky negation or up to five missing or reordered patterns, with a count for the rest. Deleting the file also clears the error, at the cost of those patterns.

See [ignore overwrite behavior](@/docs/spec-format/ignore.md#overwrite-behaviour).

### AAI-202: Import source name unknown

The argument to `agnostic-ai import` matches no registered source.

**Fix:** run `agnostic-ai import --help` for the supported list. Spelling counts.

### AAI-203: Import would replace an existing spec

`import`, `init --from`, or `use` would replace a spec under the source directories with content the importing tool never read: a hand-written spec, one edited since the last sync or import (a comment counts), one the last sync never wrote for that tool, or one another tool's import wrote. The message names the tool the current content came from. The import writes no spec. The message lists each spec and the tool that wanted it. `import --dry-run` fails the same way.

**Fix:** rename the existing spec to keep both and import again, or run `agnostic-ai import <tool> --overwrite` to replace it.

### AAI-301: Unknown sync target

A target requested via `--target`, `--only`, or the config is not a built-in adapter and no `agnostic-ai-adapter-<name>` binary is on PATH.

A name passed with `--target` fails the run. A config target that does not resolve only warns, so a teammate without an external adapter can still sync. When the name is one edit from a built-in (two for longer names), the message suggests the built-in: `unknown target: claud (did you mean claude? no agnostic-ai-adapter-claud on PATH)`.

A known target outside this run (`sync --only codex` with `targets: [claude]`) reports `codex is not in this run's targets (claude)`.

**Fix:** check the spelling. If the target exists but is not in this run, add it to `targets` or pass it with `-t`. Built-ins: `claude`, `codex`, `gemini`, `cursor`, `copilot`, `aider`, `cline`, `windsurf`, `continue`, `amp`, `zed`, `warp`, `opencode`, `antigravity`, `junie`, `kiro`, `crush`, `trae`, `qoder`, `openhands`, `factory`, `kilo`, `jules`, `goose`, `augment`. External adapters live on PATH as `agnostic-ai-adapter-<name>`.

### AAI-302: Mutually exclusive flags

Two conflicting flags were passed together, such as `--only` with `--except`, or `--watch` with `--check`. Or a flag was passed without the one it needs, such as `--diff` without `--dry-run`.

**Fix:** the message names both flags. Drop one when they conflict; add the missing one when a flag needs another.
