+++
title = "Error codes"
description = "Every AAI-NNN diagnostic, what causes it, and the fix."
weight = 150

[extra]
group = "Reference"
+++

# Error codes


Every user-facing error has a stable code of the form `AAI-NNN`, prefixed in square brackets, with its fix on the next line:

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
  Run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one.
```

Pass `--json` for machine-readable output.

## Numbering

| Range     | Area                       |
| --------- | -------------------------- |
| `001-099` | spec / config load + parse |
| `100-199` | emit (collisions, hooks)   |
| `200-299` | import                     |
| `300-399` | sync / validate            |

Codes are stable across releases. New codes append; existing codes are never renumbered.

## Codes

### AAI-001: Spec parse failed

A spec file could not be parsed. Markdown specs use YAML frontmatter; hooks and MCPs are pure YAML. The error gives the path and, when available, the line:col of the offending byte. A review `@path` include that cannot be read reports here too.

**Fix:** open the file at the reported position. Check that the frontmatter delimiters (`---`) wrap the metadata and that the YAML is valid: correct indentation, no tabs, quoted strings where needed. For an include, create the file or use a path inside the project.

### AAI-002: Spec kind not supported by target

A spec kind (hook, mcp, command, ...) is in the bundle but the target adapter does not emit it. Default policy logs a warning; `on-unsupported: error` makes it a hard failure.

**Fix:** drop the spec, switch to a target that supports the kind, or set `on-unsupported: warn` (or `silent`) in `agnostic-ai.yaml`.

### AAI-003: Config file missing

Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

**Fix:** run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-004: Config decode failed

The config file was found but could not be parsed as YAML, or its keys do not match the schema.

**Fix:** validate against `docs/schemas/config.schema.json`. Check indentation and that list keys (e.g. `targets:`) hold a YAML sequence. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-005: Installed version older than requires

The config's `requires` key names the oldest agnostic-ai release its specs work with, and the installed binary is older. Every command that reads the specs, such as `sync`, `lint`, `validate`, `doctor`, `revert`, and `cleanup`, stops before it reads specs or writes files. The message names the file, the required version, and the installed one:

```
[AAI-005] agnostic-ai.yaml requires agnostic-ai >=0.71.0, but 0.70.0 is installed; run `agnostic-ai upgrade`
```

**Fix:** run `agnostic-ai upgrade`. If the version stays the same, `agnostic-ai upgrade --check` shows which binary runs and any older copy that shadows it on PATH.

### AAI-102: Targets emit to the same output path

Two or more enabled targets would write to the same path (commonly the root `AGENTS.md`, shared by codex, amp, warp, cline, windsurf, junie, kiro, crush, trae, jules, goose, augment, qoder, openhands, factory and kilo). Last-writer-wins would mask drift.

**Fix:** drop one colliding target from `targets:` in `agnostic-ai.yaml`, or override the path via `outputs.<target>.file`.

### AAI-103: Hand-authored ignore file cannot be safely overwritten

A target's ignore file (`.cursorignore`, `.geminiignore`, `.aiderignore`, `.devinignore`, `.windsurfignore`, `.kiroignore`, `.trae/.ignore`, `.aiignore`) carries no agnostic-ai header, so `sync` cannot prove its exclusions survive and leaves the file untouched. Missing or reordered patterns, new negations, and changed whitespace all trigger this check.

**Fix:** run `agnostic-ai import <target>` to copy the file's patterns into an ignore spec. Keep their order and whitespace, and review any negations contributed by other specs before syncing again. Extra exclusion patterns are allowed. See [ignore overwrite behavior](@/docs/spec-format/settings.md#overwrite-behaviour).

### AAI-202: Import source name unknown

The argument to `agnostic-ai import` matches no registered source.

**Fix:** run `agnostic-ai import --help` for the supported list. Spelling counts.

### AAI-301: Unknown sync target

A target requested via `--target`, `--only`, or the config is not a built-in adapter and no `agnostic-ai-adapter-<name>` binary is on PATH.

A name passed with `--target` fails the run. A config target that does not resolve only warns, so a teammate without an external adapter can still sync. When the name is one edit from a built-in (two for longer names), the message suggests it: `unknown target: claud (did you mean claude? no agnostic-ai-adapter-claud on PATH)`.

A known target outside this run (`sync --only codex` with `targets: [claude]`) reports `codex is not in this run's targets (claude)`.

**Fix:** check the spelling. Add a target that exists but is not in this run to `targets`, or pass it with `-t`. Built-ins: `claude`, `codex`, `gemini`, `cursor`, `copilot`, `aider`, `cline`, `windsurf`, `continue`, `amp`, `zed`, `warp`, `opencode`, `antigravity`, `junie`, `kiro`, `crush`, `trae`, `qoder`, `openhands`, `factory`, `kilo`, `jules`, `goose`, `augment`. External adapters live on PATH as `agnostic-ai-adapter-<name>`.

### AAI-302: Mutually exclusive flags

Two conflicting flags were passed together (e.g. `--only` with `--except`, or `--watch` with `--check`), or a flag was passed without the one it needs (`--diff` without `--dry-run`).

**Fix:** the message names both flags. Drop one when they conflict; add the missing one when a flag needs another.
