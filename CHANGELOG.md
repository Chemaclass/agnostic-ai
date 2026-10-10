# Changelog

Changes by release, following [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### General

- Enable RTK and Caveman independently with `builtins` in YAML. Both stay off by default, and neither tool is installed automatically (#1964).
- Interactive commands offer upgrades with release notes, project migrations, and sync checks (#1952).
- Sync preserves JSON, hooks and ignore files; import avoids text loss; memory notes shrink and warnings name tools (#1889, #1923, #1943, #1934, #1935, #1937).
- Commands run faster at 500 specs, `graph --target` shows only that tool, and `doctor` lists tools in name order (#1905, #1913, #1914, #1953, #1956).
- Releases fix an HTTP/2 crash and no longer include the build machine's source paths (#1912, #1926).

### By tool

#### Claude Code

- `failClosed: true` blocks an action when its hook fails or times out, including global hooks and `hook run` (#1918).
- `import claude` preserves each hook's settings and `onFailure` values, so sync no longer adds duplicate hook groups (#1921, #1929).
- `doctor` names hook specs that cause duplicate runs; delete the named spec, then run `sync` and `import claude` (#1924, #1925).

#### Codex

- **Breaking:** Codex applies and validates `outputs.codex.config.sandbox`; set `read-only`, `workspace-write`, or `danger-full-access` (#1884).
- With `memory.personal: repo`, sync shows how to let Codex save memory when you keep a hand-written config (#1949).
- `import codex` keeps manual skill sources intact when their generated files are imported again (#1948).

#### OpenCode

- With `memory.personal: repo`, OpenCode saves personal memory without asking each time (#1884).
- Moving `AGNOSTIC_AI_HOME` removes the old personal memory path from OpenCode's instructions (#1888).

#### Kilo Code

- A command named `goal` is accepted with a warning that Kilo offers it as `/goal:command` (#1917).

#### Trae

- Empty agent tool lists disable all tools, including after import and sync (#1954).
- `import trae` reads shared `.agents/skills/` folders and assets; `.trae/skills/` takes priority for matching names (#1955).

#### Kiro

- `hook run` follows CLI V3 prompt and Stop exit codes, category tags, wildcards, and MCP selectors (#1974).

#### Windsurf / Devin CLI

- With `memory.personal: repo`, Devin CLI can save personal memory without asking each time (#1884).

### Site

- Guides use simpler words, and RTK and Caveman recommendations separate cost, speed and response style.
- RTK and Caveman docs cover setup, approvals, recovery and measured session costs; raw evidence preserves failed checks (#1963, #1959, #1960, #1961).
- Memory and project setup guides use plain words and explain when tools load shared memory (#1933, #1951).

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
