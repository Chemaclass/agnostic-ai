# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Entry style: one line per change. Lead with what changed, not how. State the user-facing effect. Link the issue (`#NNN`) when one exists. No em dashes, no filler.

## [Unreleased]

### Added

### Changed

### Fixed

### Removed

## v0.34.0 - 2026-06-06

### Changed

- The `sources:` block is now optional. Any omitted kind defaults to `.agnostic-ai/<kind>`, matching the tree `init` scaffolds. Previously the default was a bare `agents/` dir. Custom source paths are unaffected.

### Fixed

- Docs now state that `sync` enables 12 targets by default and that Amp and Warp are opt-in (they share the root `AGENTS.md` entry-point with Codex). README and `docs/user/targets.md` previously implied all 14 targets sync out of the box.

## v0.33.0 - 2026-06-05

### Added

- `gitignore.allow` config: a list of re-allow patterns emitted as `!`-prefixed lines at the end of the managed `.gitignore` block. Keeps a tracked file (e.g. a `testdata/AGENTS.md` fixture) from being ignored by a broader rule, without hand-editing the block. Closes #388.

### Changed

- `init` now writes `gitignore.enabled: true` by default, so a fresh project ignores its generated outputs (`.claude/`, `.codex/`, `CLAUDE.md`, `AGENTS.md`, ...) and keeps `.agnostic-ai/` as the only committed copy. The TTY prompt defaults to yes and non-interactive runs take the default; pass `init --gitignore=false` to commit outputs instead. Existing configs without a `gitignore` key are unaffected.

### Fixed

- `cleanup` now removes only the `.bak` backups `sync --backup` wrote (scoped to emitted target files and entry-point files), instead of every `*.bak` under the project. Unrelated backups (vim, manual saves, other tools) are no longer destroyed. Closes #390.
- `revert` now restores the entry-point files (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, `CONVENTIONS.md`, `AGNOSTIC_AI.md`) from their `.bak`, matching adapter-emitted files. A `sync --backup` then `revert` round-trip previously left the user's original content orphaned in `.bak`; `revert --force` now also deletes the generated entry-point files. Closes #389.
- Flat-file skills (`.agnostic-ai/skills/<name>.md`) no longer leak their sibling skills' bodies into each emitted skill folder. Sibling-asset propagation now applies only to folder-based skills (`<name>/SKILL.md`), which own their directory. Affects the claude, codex, amp, and antigravity targets. Closes #387.

### Removed

## v0.32.1 - 2026-06-04

### Changed

- Docs: README capability matrix shows per-kind support depth (native / bundled / opt-in / none) instead of a 3-state glyph.

### Fixed

- Docs: realigned README, targets.md, configuration.md, and spec-format.md with actual adapter behavior. Notably: Gemini hook events emit verbatim, Codex hooks live in `.codex/hooks.json`, `amp`/`warp` dedupe `AGENTS.md` with Codex (no collision), Zed hooks emit via `outputs.zed.tasks-file`, and the documented `outputs.*` keys are complete.

## v0.32.0 - 2026-06-03

### Added

- Antigravity emits skills natively as `.agent/skills/<name>/SKILL.md`. Configurable via `outputs.antigravity.skills-dir`. Closes #371.
- Amp emits skills natively as `.agents/skills/<name>/SKILL.md`. Configurable via `outputs.amp.skills-dir`. (#377)

### Changed

- Amp skills moved from `.agents/commands/skill-<name>.md` to native `.agents/skills/<name>/SKILL.md`; `outputs.amp.emit-skills-as-commands` no longer affects Amp. (#377)
- Docs: Codex hooks emit to `.codex/hooks.json`, not `config.toml`. (#377)

### Fixed

- Antigravity skills no longer warn as unsupported. (#371)
- Zed MCP uses the current `context_servers` schema: flat `command`/`args`/`env` for stdio, native `url`/`headers` for remote. Closes #373.
- Continue MCP files use the required `mcpServers:` block wrapper; flat single-server files did not load. Closes #374.
- Remote (HTTP/SSE) MCP servers carry an explicit `type` in `.mcp.json`, `.cursor/mcp.json`, and `.warp/.mcp.json`. Closes #375.
- Gemini SSE MCP servers use `url`, not `httpUrl`. Closes #376.

## v0.31.0 - 2026-06-03

### Added

- Custom keys under `x-<target>` now emit to every target with an output surface: Claude/Codex `SKILL.md`, Copilot `.instructions.md`, OpenCode and Amp command frontmatter, and Gemini command TOML (scalars and string arrays). Keys stay target-scoped, emit in sorted order, and never leak across adapters. Closes #367 (#368, #369).

### Changed

- Docs: simplified `README.md` and every `docs/user/` and `docs/internal/` page. Same facts, fewer words, plain English. Added a one-line entry-style convention to `CHANGELOG.md`.

## v0.30.0 - 2026-06-02

### Changed

- The managed `.gitignore` block collapses generated output directories to a single rule (`/.claude/`) instead of listing every file beneath them. Root-level outputs (`/AGENTS.md`) and entries under a source directory (`/.agnostic-ai/.sync-state`) stay precise so committed specs are never ignored.

## v0.29.0 - 2026-06-02

### Added

- Docs: recommend gitignoring generated outputs via `gitignore.enabled`.

### Changed

- Docs: `configuration.md` fixes the default target set (12 of 14; omits `amp`, `warp`) and links `targets.md` instead of duplicating the entry-point list.

### Fixed

- Restored overlay helper files (e.g. `.claude/README.md`) now flow through the recorded write path, so they land in the managed `.gitignore` block and the output ledger instead of being written silently. They were emitted via raw `os.WriteFile`, leaking as untracked and escaping orphan tracking.
- `sync --only`/`--except` no longer deletes un-synced targets' files. The orphan sweep compared the full prior ledger against the partial run's subset; partial runs now carry the prior ledger forward and defer cleanup to the next full sync.
- Managed `.gitignore` entries are root-anchored (`/AGENTS.md`), so a generated path no longer ignores a same-named file nested elsewhere. (#362)
- `import claude` propagates a nested `.claude/CLAUDE.md`'s instructions to every target, not just the claude overlay. (#361)
- Playground: optional source reads treat a missing `js/wasm` filesystem as absent instead of failing with `ENOSYS`.
- Playground renders each target's root entry-point file (CLAUDE.md, AGENTS.md, ...) alongside per-spec artifacts.

## v0.28.0 - 2026-06-01

### Added

- Per-target models: `model:` frontmatter accepts a map keyed by target (e.g. `model: {claude: opus, codex: gpt-5.5}`). A bare string applies to every target. Optional `default` sets a fallback; omit it and unlisted targets use their own native default.

## v0.27.0 - 2026-05-29

### Added
- `import antigravity` and `import continue` (rules + MCP yaml) close the sync -> import -> sync round-trip for both targets.
- `.agnostic-ai/AGNOSTIC_AI.md` is now the canonical entry-point body, tracked in git.
- `sync` ledger at `.agnostic-ai/.sync-state` sweeps orphan generated files on the next run.

### Changed
- Provenance header carries a "do not edit" hint and is now emitted on every generated file (aider conf, amp commands, copilot instructions + chatmodes, gemini TOMLs, opencode commands, warp workflows).

### Fixed
- Per-target 10/10 audit landed for every adapter (aider, amp, antigravity, claude, cline, codex, continue, copilot, cursor, gemini, opencode, warp, windsurf, zed): kit-sink golden, capability parity, provenance coverage, and byte-equal sync -> import -> sync where the adapter supports it.
- Cursor `.mdc` frontmatter double-quotes `globs:` so `**/*` parses as a YAML string instead of an anchor reference.
- Shared `sliceMainFileByH2` importer unwraps `## Rules` / `## Agents` / `## Skills` H2 wrappers and strips provenance preamble + `<!-- source: ... -->` comments so legacy concatenated rules-file layouts (aider, amp, warp, zed, opencode, copilot) reach a byte-equal fixed point on round-trip.
- `import codex` parses `outputs.codex.rules-file`, captures MCP `description` / `disabled` / `roots` fields, and `.codex/config.toml` is cleaned up when sync renders nothing for it; codex sweeps legacy `.agents/agents/` + `.agents/skills/` trees on sync.
- Shared MCP JSON importer synthesizes `type: http` from url-bearing entries so claude/amp/opencode pick the http transport branch on emit.
- Per-adapter `testdata/kitsink/` trees ship in git even when the underlying patterns are gitignored.

## v0.26.1 - 2026-05-27

### Changed
- `agnostic-ai upgrade` (and the install/upgrade docs in `README.md` + `docs/user/getting-started.md`) now print `brew upgrade --cask Chemaclass/tap/agnostic-ai`. The explicit `--cask` flag avoids ambiguity with the formula namespace and matches how the tap publishes the binary.

### Fixed
- `agnostic-ai upgrade --run` aborts pre-flight with a `rm + brew install --cask` hint when `<brew>/bin/agnostic-ai` is a regular file (or symlink outside the brew prefix), instead of letting `brew upgrade --cask` fail and revert into a half-broken install.

## v0.26.0 - 2026-05-25

### Added
- `import.codex.shred: false` keeps each `AGENTS.md` as a single rule spec instead of splitting by `##` heading. Closes #248.
- Hook frontmatter accepts `target: <name>` or `targets: [a, b]` to scope a hook to specific CLIs. Closes #249.
- `agnostic-ai doctor` flags divergent hook script bodies across tools and suggests consolidating to `.agnostic-ai/scripts/<basename>`. Closes #251.
- `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` render Codex CLI's `prefix_rule(...)` DSL into `.codex/rules/default.rules`. Closes #254.
- Codex hooks emit into `.codex/hooks.json` with matcher-aware dedupe and `timeout` + `statusMessage` support. Override via `outputs.codex.hooks-file`. Closes #255.
- `target:` / `targets:` / `target-exclude:` / `targets-exclude:` frontmatter scoping applies to every spec kind (agents, skills, rules, commands, mcps), not just hooks. Closes #292.
- Per-target body fences: wrap prose in `::target codex` / `::end` markers to emit that block only to the named target. Closes #293.
- `import claude` / `import codex` auto-set `target: <tool>` on agents/skills present in only one tool's tree. Closes #299.
- `import codex` auto-fences divergent agent/skill bodies: shared prose stays un-fenced, each tool's unique section gets wrapped in `::target` blocks. Closes #300.

### Changed
- `import <tool>` auto-sets `target: <tool>` on imported hooks so codex-specific scripts no longer leak into claude `settings.json`. Closes #257.
- Codex `agents-dir` default changed from `.agents/agents` to `.codex/agents`. Override via `outputs.codex.agents-dir`. Closes #252.
- Codex `skills-dir` default changed from `.agents/skills` to `.codex/skills`. `shared-subagents` now defaults to `true` regardless of whether claude is enabled. Closes #253.

### Fixed
- Hook path rewriting now covers shell-expansion forms like `"$(git rev-parse --show-toplevel)/.codex/hooks/x.sh"`. Closes #250.
- `import codex` captures `.codex/rules/default.rules` into an overlay so exec-policies survive import → sync round-trips. Closes #265.
- `import claude/codex` captures helper files (CLAUDE.md, README.md, scripts) into overlays with file mode preserved. Closes #267.
- `.claude/settings.json` round-trip preserves source indent and hook event order. Closes #268.
- `outputs.<target>.provenance-header: false` suppresses the `Generated by agnostic-ai` comment. Closes #269, #276.
- `import codex` captures inline `justification` and `match` kwargs from `prefix_rule(...)` calls so they survive round-trips. Closes #277.
- `import codex` lifts a file-level comment block above the first `prefix_rule(...)` into a sidecar overlay so it re-renders on sync. Closes #283.
- `import codex` preserves claude-conventional frontmatter key order when merging agent specs. Closes #282, #284.
- Codex exec-policy emitter writes `justification` and `match` as inline kwargs for byte-stable import → sync round-trips. Closes #280, #281.
- `import codex` after `import claude` no longer overwrites claude-specific skill frontmatter (`argument-hint`, `allowed-tools`, etc.). Closes #286, #287.
- Claude skill emit skips `agents/openai.yaml` (codex-only metadata) from the source skill folder. Closes #288, #289.
- `.codex/hooks.json` emits events in lifecycle order (`PreToolUse` before `PostToolUse`). Closes #290, #291.
- Codex agent TOML emits keys in codex-docs convention order for byte-stable round-trips. Closes #294, #295.
- Routing keys (`target`, `targets`, `target-exclude`, `targets-exclude`) stripped from emitted frontmatter so they do not leak into generated files. Closes #303.
- Auto-fenced spec bodies no longer contain spurious blank lines between sections. Closes #306.
- `import codex` quotes plain-scalar SKILL.md frontmatter values containing `#` so strict YAML no longer truncates descriptions at the first hash. Closes #317.

## v0.25.0 - 2026-05-23

### Added
- `init --gitignore` flag, plus TTY prompt to enable the managed gitignore block.
- `.agnostic-ai/scripts/<tool>/<basename>` stashes hook script bodies; sync rebuilds `.<target>/hooks/` on every run so `.<tool>/` can stay gitignored.
- Codex import reads `.codex/hooks.json` alongside `config.toml`; previously skipped.
- Sync warns before overwriting hand-authored entry-point files (no agnostic-ai header).

### Changed
- `sync` footer counts only files that actually changed.
- Hook command paths rewrite `.<sibling>/hooks/` → `.<target>/hooks/` per target.
- Codex agent filenames canonicalise to dash-case; underscore form preserved via `x-codex.name`. Claude + codex variants merge onto one spec.
- Codex overlay capture preserves multi-line TOML literals and key order (text-level strip instead of decode/encode).

### Fixed
- Codex emit: multi-command hook arrays now emit one `[[hooks.<event>]]` block per command (was dropping `command` entirely).
- Claude settings.json: `timeout` and `statusMessage` round-trip.
- Claude import: spurious "AGNOSTIC_AI.md seeded" log only prints when the mirror actually wrote.
- Codex AGENTS.md shred skips rules that already exist from a prior import.

## v0.24.0 - 2026-05-19

### Added
- `sync` prints success footer: `✓ synced N targets · M files · Xms`. Suppressed by `--quiet` and `--json`.

### Changed
- Capability warnings group by kind across all targets in one line: `! 5 hooks unsupported by cursor, copilot, aider, ...` replaces eight near-identical per-target lines. Suppression hint prints once per flush.
- `sync -v` prints per-target `created / updated / unchanged` counts and a footer that counts only files that actually changed (skips unchanged content via detailed recording).
- Status symbols are colorized on tty (`✓` green, `!` yellow, `✗` red). Honors `NO_COLOR=1`. Pipes and redirects stay plain.
- `sync --watch` banner shows path count and backend: `→ watching 9 paths (fsnotify) · Ctrl+C to exit`. Each re-sync is preceded by a timestamped event line (`[HH:MM:SS] change · <path>`).
- Capability warnings sticky-suppress when unchanged: the SHA-256 of the buffered `(target, kind, count)` set is stored in `.agnostic-ai/.sync-state` and compared on the next run. Repeat runs print a single-line reminder instead of the full block. Delete the state file to re-show.

## v0.23.0 - 2026-05-17

### Added

- `agnostic-ai upgrade` command: prints upgrade command per install method (Homebrew, `go install`, binary). `--run` execs it. `--check` flags `PATH`-shadowed copies.
- Integration tests: `sync --check` and `doctor --fix` are zero-drift no-ops after `import claude` / `import codex` / both. Closes #232.
- README: `brew upgrade` line + releases page link.
- Docs (`targets.md`, `configuration.md`, `cli-reference.md`): v0.22 import-side changes (`import codex` reads `.codex/prompts/*.md` + captures `.codex/config.toml` overlay, `import claude` reads `.mcp.json`) with per-target overlay precedence.
- Import summary prints `→ <overlay> seeded from <native>` for `import claude` and `import codex` when overlay file written. Closes #231.
- `sync --watch` watches `.agnostic-ai/overlays/`: hand-edits to `claude.settings.json` / `codex.config.toml` trigger re-emit within 50 ms debounce. Documented in `cli-reference.md` + `configuration.md`. Closes #234.
- Integration tests for six round-trip edge cases: re-run `import claude` overwrites overlay (no double-stomp); empty `.codex/config.toml` writes no overlay; overlay+first-class collision on `codex.config.*` resolves overlay-wins; MCP server with `env` + http MCP with `headers` survive claude→codex→claude; folded (`>`) / literal (`|`) frontmatter scalars keep style after import+sync; skill with nested assets (exec script, `agents/openai.yaml`, fixtures subdir) round-trips claude→codex→claude with exec bit intact. Closes #233.

### Changed

### Fixed

### Removed

## v0.22.0 - 2026-05-17

### Added
- Chained round-trip integration tests covering `claude → import → sync codex → wipe specs → import → sync claude` (and the inverse codex-first chain). Each kind that both adapters support (agents, skills, rules, hooks, MCPs, commands) must survive the full chain semantically. The codex chain additionally asserts that the captured overlay carries `model`, `[profiles.*]`, and other non-managed `.codex/config.toml` keys through both syncs.
- `import codex` now reads `.codex/prompts/*.md` and writes them byte-for-byte into the commands source dir. Previously the directory was skipped, so any user-authored Codex slash prompts were silently dropped during import and overwritten on the next `sync --target codex`.
- `import codex` captures every `.codex/config.toml` key outside `hooks` and `mcp_servers` into `.agnostic-ai/overlays/codex.config.toml`. The codex emitter layers the overlay before the spec-derived sections on each sync, so `model`, `sandbox`, `approval_policy`, `notify`, `[history]`, `[profiles.*]`, `[model_providers.*]`, and any future Codex keys survive a wipe of `.codex/` between import and sync. Mirrors the existing claude settings overlay.
- `import claude` now reads `.mcp.json` and writes one yaml per `mcpServers.<name>` entry into the mcps source dir. Previously the file was skipped, so MCP servers configured in a Claude Code project were silently dropped during import and never round-tripped to other adapters.

### Changed
- Codex agent TOML now emits agent-scoped `[mcp_servers.<name>]` (and any other nested-table) keys carried under the spec's `x-codex` passthrough. Previously `writeXCodexExtras` only handled scalars, arrays, and inline string tables, so a `[mcp_servers.fs]` block inside an imported agent.toml would be silently lost on the next sync. Nested-table values emit last in the agent file so the document stays TOML-valid.
- Claude `.claude/settings.json` always emits the hooks block via ordered JSON now, even on the first sync of a fresh project. Inner objects keep `{matcher, hooks}` and `{type, command}` in lifecycle order instead of the alpha-sorted `{command, type}` / `{hooks, matcher}` that the legacy `MergeJSONFile` path produced. Existing user-edited keys in `settings.json` continue to survive until the next `import claude` captures them into the overlay.

### Fixed
- Frontmatter scalar styles now round-trip: a hand-authored plain `argument-hint: <ver>` stays plain on re-emit instead of being force-quoted to `"<ver>"`, and a hand-authored double-quoted scalar stays double-quoted. The spec loader captures per-key value styles into a new `Entry.MetaStyles` map and the emitter (`FrontmatterStyled` / `DocumentStyled`) replays them. The legacy angle-bracket auto-promotion in `preferDoubleQuotes` is dropped; explicit source-style preservation makes it unnecessary.

### Removed

## v0.21.0 - 2026-05-16

### Changed
- Goreleaser config migrated from deprecated `brews:` to `homebrew_casks:`. Releases now publish to the tap as casks (`Casks/agnostic-ai.rb`) with a post-install hook removing the macOS quarantine attribute. The legacy `Formula/agnostic-ai.rb` stops receiving updates on the next release. Closes #225.

### Removed
- `autoSync` config field, `sync --auto-sync=yes|no` flag, first-run auto-sync prompt, and the generated `auto-sync` rule spec. The feature added a separate prompt, flag, and persisted config field for marginal value; agents already have the `docs-sync` rule and `run-sync-check` skill to decide when to re-sync. Existing `autoSync:` keys in user configs are silently ignored.

## v0.20.0 - 2026-05-16

### Fixed
- Frontmatter emit no longer force-quotes plain `description:` scalars. yaml.v3 does not auto-wrap plain scalars, so long descriptions round-trip on one line without added quotes. Closes #226.

## v0.19.0 - 2026-05-16

### Added
- `revert --force`: delete adapter-emitted files without a `.bak` (restores pre-#217 behavior). Closes #217.

### Changed
- `cleanup` defaults to .bak removal; `--backups` kept as alias. Closes #219.
- `revert` preserves files without a `.bak` (helpers next to `SKILL.md`, propagated templates). Pass `--force` to delete. Closes #217.
- `outputs.codex.shared-subagents` defaults to `false` when `claude` is enabled (avoids duplicating `.claude/skills/`), `true` when codex is alone. Closes #216.

### Fixed
- `doctor` reads the settings overlay in capture mode, matching real sync output. `--fix` no longer strips `enabledPlugins` / `statusLine` and no longer reports false drift after a clean sync. Import overlay also keeps source key order. Closes #215.
- Frontmatter scalars containing `<`/`>` keep their quotes; long descriptions no longer wrap at 80 cols. Closes #218.

### Removed

## v0.18.0 - 2026-05-16

### Added
- `doctor --check-globs`: opt-in flag rules whose `globs:` matches no path in the working tree. Closes #208.
- `cleanup --backups`: removes `*.bak` left by `sync --backup`. Closes #197.
- `outputs.codex.shared-subagents` (default `true`): set `false` in claude+codex setups to drop the duplicate `.agents/skills/` tree. Closes #194.
- `spec.Entry.MetaKeys` exposes source frontmatter key order; external adapters receive it as `meta_keys` (additive, no protocol bump).

### Changed
- Frontmatter emit preserves source key order, uses 2-space sequence indent, prefers double quotes. Closes #190, #191, #193.
- `.claude/settings.json` keeps overlay key order, emits `{type, command}` / `{matcher, hooks}` in documented order, events in lifecycle sequence (`PreToolUse` before `PostToolUse`). `MergeJSONFile` (codex / opencode) inherits the same. Closes #192.
- Capability warnings (`on-unsupported: warn`) collapse to one line per (target, kind) with a count + `on-unsupported: silent` hint. `sync --watch` resets between runs. Closes #204.
- `import --dry-run` lists paths + count instead of dumping file bodies. Matches `sync --plan`. Closes #205.
- `doctor` drift splits into "missing" vs "stale — edited locally since last sync". Closes #207.

### Fixed
- emit: normalize trailing newlines to exactly one `\n`. Closes #195.
- `doctor` no longer false-positives drift on `.claude/settings.json` after sync (OrderedJSON round-trip is now byte-stable). Closes #200.

### Docs
- README: rewrite "byte-identical" round-trip claim to match reality (content-preserving; marker + canonical formatting applied). Closes #201.
- README: target table separates native from convention paths. Closes #202.
- README + getting-started: recommended adoption workflow (split `import` and `sync` into two commits). Closes #196.
- AGNOSTIC_AI.md template: new "Target-specific overlays" section. Closes #203.
- spec-format hooks: render-to-target schema mapping (Claude / Codex / Gemini). Closes #206.
- spec-format skills: nested helper files round-trip verbatim. Closes #211.
- getting-started: import scope boundary + `.agnostic-ai/.sync-state` reference. Closes #209, #210.

### Tests
- claude import: provenance-marker round-trip stays marker-free across sync ↔ import cycles. Closes #198.

## v0.17.0 - 2026-05-15

### Added
- `agnostic-ai lsp`: LSP server on stdio; pushes lint diagnostics on open/change/save. Closes #168.
- `packs add` / `init`: auto-add `.agnostic-ai/packs/` to `.gitignore`. Closes #170.
- `doctor` / `sync --check`: now detect drift in `AGNOSTIC_AI.md` and target entry-point files (`CLAUDE.md`, `AGENTS.md`, etc.).

### Changed

### Fixed

### Removed

## v0.16.0 - 2026-05-15

### Changed
- `sync`: uses `AGNOSTIC_AI.md` as the entry-point body source and distributes its content to `CLAUDE.md`, `AGENTS.md`, etc. Seeds the template when absent. Preserves content written by `import <target>`.
- `AGNOSTIC_AI.md` is no longer auto-added to `.gitignore`; commit it as a source file.
- `outputs` key in `agnostic-ai.yaml` is fully optional; omitted when empty in the JSON envelope sent to external adapters.

## v0.15.1 - 2026-05-15

### Changed
- `scripts/release.sh`: refuse to cut a release when no commits exist past the last tag or when `CHANGELOG [Unreleased]` has no entries. Prevents shipping an identical binary or empty release notes.

## v0.14.2 - 2026-05-15

### Added
- `import`: accept multiple sources (`agnostic-ai import claude codex`). `AGNOSTIC_AI.md` mirrors last source (last-wins). `all` stays exclusive.

## v0.14.1 - 2026-05-15

### Added
- `init`: scaffold `commands/` source folder and `sources.commands` entry.

## v0.14.0 - 2026-05-15

### Added
- `sync --plan`: per-target diff summary without writing (#161).
- `sync`: atomic transaction; partial writes roll back on failure (#162).
- `sync.collision-policy`: `prompt`, `prefer-spec`, `fail` (#167).
- `import all`: detect and import every installed CLI (#160).
- `init --from <cli>`, `init --dry-run`, `import --dry-run` (#158, #160).
- `agnostic-ai lint`: LINT001–LINT005 semantic checks, `--strict` (#171, #177).
- `agnostic-ai doctor`: `mcp` / `install` / `config` subcommands, `--json` (#159).
- `agnostic-ai install-hook`: pre-commit `sync --check`, `--shared` for `.githooks/` (#169).
- MCP spec: `description`, `disabled`, `roots` (#177).
- Claude import: `.claude/commands/*.md` round-trips frontmatter (#177).
- Codex `outputs.codex.config`: first-class `notify`, `profiles.<name>`, `model-providers.<id>` (#177).
- Codex `[mcp_servers.<name>]`: `description`, `disabled`, `roots` (#177).
- Codex agent: top-level `tools:` first-classed in agent TOML (#177).
- Golden snapshot tests per adapter (#166).

## v0.13.0 - 2026-05-15

### Added
- `AAI-NNN` error codes; `agnostic-ai explain <code>` (#163).
- `agnostic-ai why <file>` traces a file back to adapter/specs/config/timestamp, `--format json` (#164).
- `agnostic-ai graph` spec → target → file. Formats: text, mermaid, dot, json. Filters: `--target`, `--spec`, `--kind` (#172).
- `claude`: first-class `outputs.claude.settings.*` block (`model`, `outputStyle`, `apiKeyHelper`, etc.). Layers over captured overlay; spec hooks still win for `hooks` key (#177).

### Changed
- **BREAKING**: `sync` writes a uniform pointer body to every target's entry-point plus `.agnostic-ai/AGNOSTIC_AI.md`. Opt back into legacy concat via `outputs.<target>.rules-file` (#153).
- `import` next-steps suggest `sync` + hint other detected CLIs.
- `init` next-steps adapt to context.
- `init` adds `.agnostic-ai/.sync-state` to `.gitignore` (#151).
- Provenance header shortened to `Generated by agnostic-ai`.

## v0.12.0 - 2026-05-14

### Added
- `antigravity` adapter (Google Antigravity IDE). Supports `rule` + `agent`. Emits `.agent/rules/*.md` and `.agent/AGENTS.md`.

### Changed
- `import` writes `AGNOSTIC_AI.md` to `.agnostic-ai/` instead of project root. Delete old root file after upgrading.
- `init` two-step next-steps: `import <target>` then `sync`.
- Adapter outputs now start with `Generated by agnostic-ai. Do not edit by hand.` header. Importers strip on round-trip. Closes #140.

### Fixed
- `spec`: derive skill name from parent directory when frontmatter absent.
- `claude`: no leading blank line when frontmatter empty (#137).
- `claude`: per-file rules round-trip frontmatter, drop synthetic `# <name>` heading (#138).

## v0.11.0 - 2026-05-14

### Added
- New `command` spec kind. `commands/*.md` emits per-target slash commands.
- `claude`: `.claude/commands/<name>.md`. Override via `outputs.claude.commands-dir`.
- `codex`: `.codex/prompts/<name>.md`. Override via `outputs.codex.commands-dir`.
- `sources.commands` config key (default `commands`).
- `emit.CopyTree` for mirroring file trees (honors capture/dry-run/recording/backup).

### Changed
- Spec loader skips kinds whose `sources.<kind>` is empty rather than walking the layer root.
- `init` pre-ticks targets whose marker exists (`.claude/`, `.codex/`, etc.).

### claude
- `import claude` mirrors full skill dirs (helper scripts, fixtures, subdirs) byte-for-byte.
- `sync` propagates skill sibling files into `.claude/skills/<name>/`. Mode bits preserved.

### codex
- `import codex` mirrors full `.agents/skills/<name>/` (including `agents/openai.yaml` + assets).
- `sync` propagates skill sibling files. `x-codex`-derived `agents/openai.yaml` wins over source copy at same path.

## v0.10.0 - 2026-05-14

### claude
- Rules emit per-file under `.claude/rules/<name>.md`. `CLAUDE.md` untouched. Set `outputs.claude.rules-file: CLAUDE.md` for legacy concat.
- `.claude/settings.json` preserves user keys across sync. `import claude` captures into `.agnostic-ai/overlays/claude.settings.json`; sync layers spec hooks on top.
- Hooks merge by `event` + `matcher`; `command:` accepts string or list.

### codex
- Agents emit at `.agents/agents/<name>.toml` (was `.codex/agents/`). Override via `outputs.codex.agents-dir`.
- `import codex` round-trips agents/skills/hooks/MCPs. Unknown TOML keys land under `x-codex`.

### gemini
- `command:` accepts list; each entry emits as separate `{matcher, command}` pair.
- Fix: emit no longer drops `command` when spec uses a list.

### all
- Hook spec filenames derive from content hash (`<event>[-<matcher>]-<hash8>.yaml`). Re-imports converge.
- JSON outputs no longer HTML-escape `&`, `<`, `>`.

### Dependencies
- `github.com/BurntSushi/toml` v1.6.0 (codex TOML parsing).

## v0.9.0 - 2026-05-14

### Added
- Playground UX: target chips, per-target file selector, theme toggle, mobile layout.
- Source URL in `--help` / `--version`.
- `import` for **aider, amp, warp, gemini, copilot, opencode, zed** (full kind coverage where supported).
- Every importer mirrors target's main file to `AGNOSTIC_AI.md`. Last import wins.
- `import claude` prefers `.claude/rules/*.md` over slicing `CLAUDE.md`.
- `import copilot` lifts leading italic into `description:`; routes `agent-*` / `skill-*` files to matching source dir.

### Changed
- `init` prompts for targets by default on TTY. Pipe a list, or `--all` / `-a` to skip. `-i` / `--interactive` removed.
- `DefaultTargets()` no longer includes `amp` and `warp` (collide with codex on root `AGENTS.md`).

### Fixed
- `import claude` keeps preamble before first `##`; ignores `##` inside fenced blocks.
- `agnostic-ai.yaml`: only `version` required.
- `sync` / `sync --check` fail fast with `output collision` when two targets write the same path.

## v0.8.0 - 2026-05-13

### Added
- `agnostic-ai.local.yaml` per-machine overrides; deep-merged over base. Auto-gitignored by `init` (#128).

### Changed
- Renamed `agnostic.config.yaml` → `agnostic-ai.yaml`. Legacy still loads with deprecation warning (#128).

## v0.7.0 - 2026-05-13

### Added
- `new <kind> <name>` scaffolds a single spec (#31).
- `render <spec> [--target <t>...]` prints emission to stdout (#31).
- `explain <spec>` lists every output a spec contributes to, `--json` (#40).
- `init --preset <go|ts-react|python>` (#47).
- `sync --watch-poll` forces polling backend (#36).
- VS Code extension `editors/vscode/` (#44); JetBrains plugin `editors/jetbrains/` (#101). Schema validation, render commands, drift status indicator.
- Pre-commit recipes (pre-commit/lefthook/husky) at `docs/user/git-hooks.md` (#39).
- WASM playground at `docs/playground/` (#43).
- `validate` rejects unknown hook `event:` with supported list (#112); warns on hook/MCP with no consumer (#114).
- `doctor` resolves stdio MCP `command:` on PATH; install hints for missing npx/uvx/python/docker (#113).
- Per-target opt-in native surfaces (existing rule emission preserved): Copilot Custom Chat Modes (#105), Cursor Custom Commands (#104), Cline Workflows (#106), Windsurf Workflows (#107), Continue Assistants (#108), Zed Tasks (#109), Warp Workflows (#110).

### Changed
- `sync --watch`: fsnotify + 50 ms debounce (sub-100 ms re-sync, zero idle CPU). Polling kept as fallback (#36).
- Code of Conduct contact: `conduct@chemaclass.dev` → `agnostic-ai@chemaclass.es`.

## v0.6.0 - 2026-05-12

### Changed
- **BREAKING**: Amp default `AGENT.md` → `AGENTS.md` (per Sourcegraph spec). Legacy auto-renamed to `AGENT.md.bak` (#67).
- **BREAKING**: Warp default `WARP.md` → `AGENTS.md` (per Warp Rules / AGENTS.md standard). Legacy auto-renamed to `WARP.md.bak` (#68).
- Go toolchain bumped to 1.24 (#76).

### Added
- Native multi-file emission for ◐ adapters:
  - **Copilot** (#64): `.github/instructions/<name>.instructions.md` per scoped rule.
  - **Gemini** (#65): hierarchical `GEMINI.md` + `.gemini/commands/<name>.toml`.
  - **OpenCode** (#66): `.opencode/commands/<name>.md` per agent.
  - **Amp** (#67): hierarchical `AGENTS.md` + `.agents/commands/<name>.md`.
  - **Warp** (#68): hierarchical `AGENTS.md` (agents inlined).
- Hooks + MCP propagation: Codex `.codex/config.toml` (#78), Gemini `.gemini/settings.json` (#79), Continue `.continue/mcpServers/<name>.yaml` (#80), Amp `.amp/settings.json` (#81), Zed `.zed/settings.json` (#82), Warp `.warp/.mcp.json` (#83), OpenCode `opencode.json` (#84).
- Shared emit helpers: `MigrateLegacyFile`, `MergeJSONFile`, `WriteReference`, `GroupRulesByScope`, TOML writers. New `config.Output` fields: `instructions-dir`, `commands-dir`, `mcp-dir`, `emit-skills-as-commands`.
- `sync --json`, `sync --check --json`, `revert --json`, `doctor --json` (schema v1).
- `agnostic-ai status`: project name, layers, spec counts, targets, last sync, drift count. `--json`.
- First-sync target picker (#92): interactive multi-select when config still lists every target. Selection persisted.

## v0.5.0 - 2026-05-07

### Added
- `sync --only <targets>` / `sync --except <targets>` filters (mutually exclusive). `revert` gains same flags.
- `agnostic-ai validate --fix`: rewrite specs for autofixable issues (backfills missing `name:` from filename / parent dir). Plain `validate` flags fixable issues with `*`.
- Plugin protocol v1 for external adapters (`agnostic-ai-adapter-<target>` on PATH; JSON over stdin/stdout). Docs at `docs/internal/plugin-protocol.md`.
- `adapters.Resolve(name)`: lookup site with built-in → external fallback.
- `agnostic-ai packs add|remove|update|list`: shareable spec packs from Git URLs. Pinned in `agnostic.packs.lock`. Load as a layer between user-global and project.
- `docs/user/ci.md`: dedicated CI page + `chemaclass/agnostic-ai-action@v1`.
- `completion bash|zsh|fish|powershell`.
- JSON schemas at `docs/schemas/config.schema.json` (generated) and `spec.schema.json` (hand-authored).
- `init` injects `yaml-language-server` schema hint into generated config.

### Changed
- GitHub Release body built from `CHANGELOG.md` via `scripts/release-notes.sh`. No post-hoc `gh release edit`.
- `promote_changelog` writes `## vX.Y.Z - YYYY-MM-DD` (no brackets). `## [Unreleased]` keeps brackets.
- CI lint upgraded to `golangci-lint v2.6`.

### Fixed
- Spec frontmatter parse errors report `path:line:col`. Malformed YAML no longer silently treated as body.
- `extract_changelog_section` accepts both `## [vX.Y.Z]` and `## vX.Y.Z`.

## v0.4.0 - 2026-05-05

### Added
- `sync --watch`: re-emit on changes (200 ms poll; Ctrl+C exits). Incompatible with `--check`.
- `sync --auto-sync=yes|no`: first-sync TTY prompt; writes auto-sync rule spec; persists answer to config.
- `init --demo`: seed one example spec per source folder.
- `init -i` / `--interactive`: multi-select target picker.
- `import <source>`: translate existing CLI config into specs. Sources: `claude`, `codex`, `cursor`, `cline`, `windsurf`, `continue`.
- Codex subagents: `.codex/agents/<name>.toml`. `x-codex` frontmatter passes through.
- Codex skills: `.agents/skills/<name>/SKILL.md`. `x-codex.interface`/`policy`/`dependencies` add `agents/openai.yaml`.
- `--help` examples on every command.

### Changed
- `init` scaffolds under `.agnostic-ai/` by default. `init .` for legacy root layout.
- `sync` skips empty stub files.
- `list` / `validate` print hint when no specs loaded.
- Codex `AGENTS.md` lists agents as pointers, not inlined bodies.

### Removed
- `init --from <source>` flag. Use `init` then `import <source>`.

## v0.3.0 - 2026-05-04

### Added
- MCP servers as first-class spec kind (`mcps/*.yaml`). Propagates to Claude (`.mcp.json`), Cursor, Copilot (VS Code shape). Other targets warn.
- `sync --backup`: copy each existing file to `<path>.bak`. Opt-in. Skipped on no-op writes.
- `agnostic-ai revert`: restore `.bak` if present, otherwise remove. `--dry-run` to preview.
- Auto-managed `.gitignore` block. `gitignore.enabled: true` (or `sync --gitignore on`); lines outside block preserved.
- Provenance markers: each `###` section in merged docs opens with `<!-- source: <relpath> -->`.
- Nested rule scoping. Subdir specs (`rules/backend/auth.md`) carry implicit scope; per-dir adapters route accordingly.
- New adapters: `amp` (`AGENT.md`), `zed` (`.rules`), `warp` (`WARP.md`), `opencode` (`.opencode/AGENTS.md`).
- `init --from cursor`: import `.cursor/rules/*.mdc` (frontmatter passes through).
- `init --from codex`: walk for `AGENTS.md` at any depth; translate each `## section` into a rule with inferred `globs:`.

## v0.2.0 - 2026-05-04

### Added
- Skill emission: rules-dir adapters write per skill file; merged-doc adapters list under `## Skills`.
- Generated-by header on merged-document outputs.
- `internal/testutil` with `Chdir` / `TempCwd`.
- `.github/dependabot.yml` weekly updates.
- `SECURITY.md`.
- Roadmap doc.

### Changed
- README simplified; details moved into `docs/`.
- README aligned with AGENTS.md open standard.
- `internal/cli/import_claude.go` split into per-concern files. No behavior change.
- CI writes coverage profile; uploads to Codecov.
- `make build` adds `-trimpath -ldflags="-s -w"`.
- Makefile gains `lint`, `fmt`, `vet`, `cover`.

### Fixed
- CI Windows runner pinned to `bash`.

## v0.1.0 - 2026-05-04

### Added
- Initial release: adapters for Claude Code, Codex, Gemini CLI, Cursor, GitHub Copilot, Aider, Cline, Windsurf, Continue.
- Commands: `init`, `sync`, `validate`, `list`.
- `init --from claude`: import existing `CLAUDE.md` + `.claude/` config.
- `sync --check` and `doctor` for drift detection.
- `x-<target>` frontmatter namespace.
- Docs, examples, integration tests, dogfood specs.
- OSS scaffolding: CONTRIBUTING, COC, GOVERNANCE, issue/PR templates, CI, GoReleaser, golangci-lint, Dockerfile, lefthook, Taskfile, dependabot/renovate.

[Unreleased]: https://github.com/Chemaclass/agnostic-ai/compare/v0.18.0...HEAD
[v0.18.0]: https://github.com/Chemaclass/agnostic-ai/compare/v0.17.0...v0.18.0
