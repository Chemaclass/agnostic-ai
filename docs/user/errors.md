# Error codes

Every user-facing error has a stable code of the form `AAI-NNN`, prefixed in square brackets:

```
[AAI-003] read config: no agnostic-ai.yaml or agnostic.config.yaml in /path/to/project
```

Look up a code without leaving the terminal:

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

A spec file could not be parsed. Markdown specs use YAML frontmatter; hooks and MCPs are pure YAML. The error includes the path and (when available) line:col of the offending byte.

**Fix:** open the file at the reported position. Confirm the frontmatter delimiters (`---`) wrap the metadata and that the YAML is well-formed (correct indentation, no tabs, quoted strings where needed).

### AAI-002: Spec kind not supported by target

A spec kind (hook, mcp, command, ...) is in the bundle but the target adapter does not emit it. Default policy logs a warning; `on-unsupported: error` makes it a hard failure.

**Fix:** drop the spec, switch to a target that supports the kind, or set `on-unsupported: warn` (or `silent`) in `agnostic-ai.yaml`.

### AAI-003: Config file missing

Neither `agnostic-ai.yaml` nor the legacy `agnostic.config.yaml` exists in the project root.

**Fix:** run `agnostic-ai init` to scaffold a config, or `cd` into the directory that already contains one. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-004: Config decode failed

The config file was found but could not be parsed as YAML, or its keys do not match the schema.

**Fix:** validate against `docs/schemas/config.schema.json`. Check indentation and that list keys (e.g. `targets:`) hold a YAML sequence. Run `agnostic-ai doctor` for a full diagnosis.

### AAI-102: Targets emit to the same output path

Two or more enabled targets would write to the same path (commonly the root `AGENTS.md`, shared by codex, amp, warp, zed, cline, junie, kiro, crush, trae, jules, goose, augment, qoder, openhands, factory and kilo). Last-writer-wins would mask drift.

**Fix:** drop one colliding target from `targets:` in `agnostic-ai.yaml`, or override the path via `outputs.<target>.file`.

### AAI-202: Import source name unknown

The argument to `agnostic-ai import` matches no registered source.

**Fix:** run `agnostic-ai import --help` for the supported list. Spelling counts.

### AAI-301: Unknown sync target

A target requested via `--target`, `--only`, or the config is not a built-in adapter and no `agnostic-ai-adapter-<name>` binary is on PATH.

**Fix:** check the spelling. Built-ins: `claude`, `codex`, `gemini`, `cursor`, `copilot`, `aider`, `cline`, `windsurf`, `continue`, `amp`, `zed`, `warp`, `opencode`, `antigravity`, `junie`, `kiro`, `crush`, `trae`, `qoder`, `openhands`, `factory`, `kilo`, `jules`, `goose`, `augment`. External adapters live on PATH as `agnostic-ai-adapter-<name>`.

### AAI-302: Mutually exclusive flags

Two conflicting flags were passed together (e.g. `--only` with `--except`, or `--watch` with `--check`).

**Fix:** pick one. The error message names both flags.
