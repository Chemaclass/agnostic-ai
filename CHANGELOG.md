# Changelog

Each release lists general changes first, then changes by tool, then site work. Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style, section order, and what belongs here instead of the issue or the docs: `.agnostic-ai/agents/changelog-curator.md`.

## [Unreleased]

### By tool

#### Codex

- `outputs.codex.config.sandbox` now reaches Codex: sync writes it as `sandbox_mode`, since Codex ignored the old `sandbox` key (#1880).

#### OpenCode

- In repo mode, OpenCode saves personal memory without asking each time: sync allows its folder under `permission.external_directory` (#1881).

#### Windsurf

- In repo mode, Devin CLI saves personal memory without asking: sync allows writes to its folder in `.devin/config.json` (#1882).

## v0.81.0 - 2026-10-07

### General

- **Breaking:** `lint` and `sync` stop on a plain MCP `env` or `headers` value not marked `!literal`; run `agnostic-ai migrate --only secrets` (#1794).
- New `memory` built-in: one memory every tool loads at session start and saves to, for the team and for you (#1844, #1845, #1850, #1851, #1852).
- `memory.personal: repo` shares one personal memory across every worktree, also when `~/.agnostic-ai` is a link (#1859, #1869, #1877).
- New `memory lint`, `index`, `list`, and `path` commands check, rebuild, list, and locate memory; `doctor` flags broken links and secrets (#1847, #1877).
- Sync keeps hooks you wrote by hand when it adds its own, on Claude Code, Codex, Cursor, Gemini CLI, Qoder, and Factory (#1858).

### By tool

#### Claude Code

- **Breaking:** sync stops on an agent name that starts with `-` or contains `:`, which Claude Code would skip; rename the agent (#1870).
- With `memory`, Claude Code's own memory saves into the shared personal memory, so other tools see it (#1846).
- `lint` counts the `AGENTS.md` that `CLAUDE.md` imports in Claude Code's word total, which it used to leave out.

#### Cursor

- `.cursor/cli.json` always has both `allow` and `deny`; the Cursor CLI refused a file with only one, so its rules never loaded.
- In repo mode, the Cursor CLI can save personal memory: sync allows writes to its folder in `.cursor/cli.json`.
- Turning off the last Cursor hook removes `.cursor/hooks.json` instead of leaving `{"version": 1}` behind.

#### Copilot

- A hook with a Windows command now runs on Windows: sync writes separate `bash` and `powershell` commands (#1856).

#### Qoder

- In repo mode, Qoder saves personal memory without asking for approval each time (#1872).

#### Kilo Code

- In repo mode, Kilo finds personal memory through the rule; sync no longer lists a file Kilo ignores (#1871).

#### Augment

- `hook run` now shows that a `SessionStart` hook's output reaches the model (#1853).

#### OpenHands

- `hook run` no longer claims a `SessionStart` hook's output reaches the model; OpenHands only logs it (#1853).

### Site

- Docs rewritten in plain words and trimmed, with a new shared memory guide.

## v0.80.0 - 2026-10-06

### General

- Opt-in `handoff` built-ins carry a task between tools, save approved rules, and add Git snapshots with resume notices (#1829, #1830, #1831).
- Built-in text can change with a release and show as `sync --check` drift. Run `sync` after upgrading (#1829).
- With no changed files, `sync` and `sync --check` run 2 to 5x faster; `sync --check --against` is about 5x faster at 20k+ files (#1819, #1821).
- `@path` lines inside a code fence stay as written, and a one-line code span no longer hides the lines after it (#1823, #1828).

### By tool

#### Kiro

- Commands sync to and import from `.kiro/prompts/` for CLI V3 slash commands, with native argument templates intact (#1822).

### Site

- The Amp page drops the services `agent` key, which Amp no longer documents (#1835, #1839).

Releases before v0.80.0 are in [docs/CHANGELOG-archive.md](docs/CHANGELOG-archive.md).
