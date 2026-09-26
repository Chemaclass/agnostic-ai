# Changelog archive

Releases v0.1.0 through v0.59.0. Everything from v0.60.0 on lives in [CHANGELOG.md](../CHANGELOG.md).

## v0.59.0 - 2026-09-16

### Added

- `agnostic-ai verify` fails CI on stale generated output and fingerprints the harness for a project-owned verifier (#834).
- Portable model settings round-trip through Codex, Copilot, OpenCode, Junie, Qoder, and Kilo, plus allow, deny, and ask permissions on Qoder (#806, #827).
- Goose and OpenHands emit project agents to `.agents/agents/<name>.md`, and Goose hooks emit an Open Plugins package across 12 events (#631, #629).
- Amp environment specs emit `.agents/setup` scripts and supervised `.amp/services.yaml` services (#637).
- Augment and Factory emit commands, Augment writes `.augmentignore`, and scoped skills stay scoped on Codex, Cursor, Warp, and OpenCode (#630, #808, #805).

### Changed

- Antigravity agents move to the documented `.agents/agents/<name>/agent.md` layout, and sync migrates managed legacy flat files (#717).

### Fixed

- Codex imports package-style MCP server names containing `/` without splitting them across files (#711).
- Amp, Cline, and Windsurf import every documented skill path, with bundled assets, file modes, and Windsurf `triggers` preserved (#821, #823).
- Hooks keep their native handlers: Claude `continueOnBlock`, Copilot HTTP and `sessionStart`, and Zed `create_worktree` (#804, #629, #817).
- Factory and Windsurf skip unsupported WebSocket MCP entries instead of writing invalid native configuration (#809, #816).
- Kiro keeps an `x-kiro.name` display name alongside the canonical filename, and Amp and Copilot audits track their current upstream sources (#807, #822).

### Site

- `agnostic-ai.org` is the canonical site, built with Zola 0.22.0 from shared templates, with legacy URLs and feed identifiers stable.
- User guides live at `/docs/` from the same Markdown as the repository and `llms-full.txt`, and `/agent-setup.txt` gives a coding agent a safe setup path.
- The landing page leads with a copy-ready install command, a three-step activation path, and a ten-target comparison.
- The capability matrix filters and shares comparisons by URL; the playground covers all ten spec kinds; CI checks both against adapter declarations.
- Each release ships a briefing kept apart from verified upstream news, and the updates archive filters editions by target and search term.

## v0.58.0 - 2026-09-14

### Added

- `agnostic-ai upgrade --version v0.56.1` installs one named release instead of the latest, downgrades included, for standalone binary installs (#800).
- Hook specs reach Antigravity's `.agents/hooks.json`, keyed by definition name, and `validate` checks its five event names (#629).
- `agnostic-ai update` is an alias for `upgrade`, with the same `--run` and `--check` flags.
- The `agent-context` skill works globally without project agents and guides root setup and context reviews (#793).

### Changed

- `make tools` installs the golangci-lint version CI runs, and `make lint` names a stale binary instead of failing with a decode error (#749).

### Fixed

- `agnostic-ai upgrade` and `update` install the latest release by default, checksum-verified and replaced in place; `--check` stays read-only.

## v0.57.0 - 2026-09-14

### Added

- `sync.unmanaged` lists user-owned output paths that sync, `doctor`, the ledger, the `.gitignore` block, and `revert` all leave alone (#781).
- `::target` fences work in `.agnostic-ai/AGNOSTIC_AI.md`, so a fenced paragraph reaches only the entry-point files a listed target reads (#781).
- Claude emits and imports HTTP, MCP-tool, and prompt hooks; Cursor emits native prompt hooks (#767, #768).
- Ignore specs reach Crush's `.crushignore` and Kilo's `.kilocodeignore`, and review specs reach Goose's `.agents/REVIEW.md` (#770, #773, #771).
- Continue imports JSONC MCP maps, and Zed, Warp, and Antigravity import native skill folders with bundled assets (#764, #765).

### Fixed

- Hook scripts from `.agnostic-ai/scripts/` go through the sync session, so a failed sync rolls them back and deleting the hook sweeps the script (#789).
- Deleting a skill removes its bundled files from every target, and sync stops leaving stale output or flagging every prior output as an orphan (#781, #785).
- `::target` fences survive `import`, pass `validate` with external adapters, and leave no blank line when dropped (#781, #790).
- Sync keeps what it does not own: VS Code `inputs` and `sandbox`, extra MCP options, and hand-authored ignore file order (#757, #769, #763, #774, #775, #761).
- Native output matches what each tool loads, on Gemini hooks, Zed skill names, and Kiro timeouts and actions (#762, #766, #772).

## v0.56.1 - 2026-09-12

### Fixed

- `curl -fsSL ... | bash` installs again, and a run piped into `/bin/sh` no longer exits 0 having installed nothing.

## v0.56.0 - 2026-09-12

### Upgrade notes

Three changes make a previously green repo fail. All three are deliberate.

- `lint` now errors on an MCP spec missing `command:` or `url:` (LINT008). The entry was dead on every target and nothing said so.
- `sync` now fails with `AAI-103` rather than overwriting a hand-authored ignore file. Run `agnostic-ai import <target>`, then sync again.
- Gemini agents move to `.gemini/agents/`. Set `outputs.gemini.emit-agents-as-commands: true` to keep typing `/name`.

### Added

- Hook specs reach five more targets: Copilot (`.github/hooks/agnostic-ai.json`, 14 events), Factory, Trae, Crush (`PreToolUse` only), and Augment (#629).
- Hook specs accept `args` to run in exec form, so a path with a space or `$` runs as written on Claude Code, Qoder, and Copilot (#732, #746, #755).
- Ignore specs reach Kiro (`.kiroignore`), Trae (`.trae/.ignore`), and Junie (`.aiignore`) instead of skipping with a warning (#728).
- Gemini agents emit as native subagents at `.gemini/agents/<name>.md`, invokable with `@name` and listed by `/agents` (#733).
- `lint` reports an MCP server missing the field its transport requires, which targets wrote as a dead entry or dropped (LINT008, #747).

### Changed

- Gemini agents stop emitting `.gemini/commands/<name>.toml` and sweep old copies; set `outputs.gemini.emit-agents-as-commands: true` to keep `/name` (#733).
- Qoder hook events sort in the vendor's own order, so `.qoder/settings.json` changes bytes once for a project mixing documented and custom event names (#744).

### Fixed

- `sync` fails with `AAI-103` instead of overwriting a hand-authored ignore file on seven targets; run `import <target>` to read it into an ignore spec (#754).
- `sync` no longer deletes every user-authored key in a JSONC config such as `kilo.jsonc`, credentials and permission settings included (#725).
- `import gemini` reads `.gemini/commands/*.toml` again, after #748 made one subagent in a project hide every command (#750).
- Continue MCP servers match its schema: `streamable-http` for `type: http`, `headers` under `requestOptions`, and a coverage note for `ws` (#726, #730, #739).
- Warp skips an MCP entry with nothing to launch, Crush accepts each documented event spelling, and Codex round-trips `startup_timeout_ms` (#731, #735, #753).
- `sync -t amp` stops writing Agent specs to the directory Amp's own migration tells users to delete, and sweeps what a previous sync left there (#727).
- `validate` accepts Qoder's documented hook events and Cursor's matchers, and a Kilo command named `goal` raises a coverage note (#734, #736, #744).
- Docs fix stale vendor claims and warn that `claude` plus `copilot` can run a hook twice through `.claude/settings.json` (#737, #745, #755, #756).

## v0.55.0 - 2026-09-10

### Added

- Windsurf hooks emit to `.devin/hooks.v1.json` across eight events, `type: prompt` included, and `import windsurf` reads them back (#629).
- Qoder hooks emit to `.qoder/settings.json` across 23 events, matching Claude Code's tool-name matcher vocabulary (#629).
- Kilo Code and Qoder commands emit natively, with the `description` frontmatter each vendor requires; both previously skipped with a warning (#630).
- Trae rule frontmatter merges `x-trae` custom keys such as `scene: git_message`, which never reached a rule file before (#635).

### Fixed

- Kiro skills emit natively to `.kiro/skills/<name>/SKILL.md` with bundled assets, and a stale flattened copy in steering is swept (#642).
- `import codex` reads inline hooks in the vendor's documented nested shape, which previously imported as zero hooks (#669).
- Docs list every target that emits hooks and commands natively, after both enumerations drifted behind the adapters (#629, #630).
- Amp, Junie, Trae, Kiro, Warp, and Cursor docs drop stale vendor citations and add vendor-confirmed fields (#647).

## v0.54.0 - 2026-09-10

### Added

- `new rule --scope` adds directory-specific rules with native scoped output for 19 targets, kept out of root instructions (#704).
- The site and playground move onto a single amber accent on ink, with every foreground/background pair checked against WCAG AA.

### Changed

- Docs start with a first-rule tutorial and dedicated installation, migration, and troubleshooting guides, plus a runnable scoped-context walkthrough.

### Fixed

- Codex: an MCP server name with a package-style character no longer writes a TOML header that breaks every server in the file (#706).
- Windsurf: `outputs.windsurf.workflows-dir` warns instead of writing files nothing reads, since Devin removed the only agent that ever read a Workflow (#707).
- Junie docs name `allowPromptArgument` as vendor-documented, and the Kiro MCP comment drops an implied confirmation it never had (#708).

## v0.53.0 - 2026-09-09

### Added

- OpenHands hooks emit to `.openhands/hooks.json` across six events; a matcher copied from a Claude spec raises a coverage note (#629).
- Codex and Copilot MCP entries keep documented keys they dropped before, including Codex `oauth` and VS Code `envFile` and `sandboxEnabled` (#692).
- Codex hooks gain `type: mcp_tool`, which calls a connected server's tool instead of running a shell command (#693).
- The site is findable: canonical URL, Open Graph and Twitter cards, structured data, `llms.txt`, `sitemap.xml`, and `robots.txt`.

### Fixed

- `sync --jobs` no longer fails intermittently with `invalid argument` on macOS when two targets share a directory (#701).
- Warp: sync warns when a hand-authored `WARP.md` sits beside the generated `AGENTS.md`, since Warp reads `WARP.md` first and ignores synced rules (#691).
- Cursor skills promote the documented `icon` and `color` to first-class keys, which previously reached the file only through `x-cursor` (#694).
- The generated entry-point body offers `.geminiignore` instead of the `.aiexclude` this tool stopped writing in #625 (#651).

## v0.52.1 - 2026-09-07

### Changed

- The npm publish gates on the release archives, not on every package-manager push, and skips a version already on the registry, so a re-run is safe.

## v0.52.0 - 2026-09-07

### Added

- `sync --global` installs user-level instructions, rules, hooks, and skills from `$AGNOSTIC_AI_HOME` at the documented path of 22 of 25 targets (#680).
- Codex MCP servers emit per-tool sub-tables from a `tools` map, covering `output_token_limit` and the per-tool approval override (#678).
- `outputs.claude.settings.bashOutputMaxChars` and `.taskOutputMaxChars` raise how much output Claude Code keeps inline, up to 128K characters (#679).

### Changed

- Ordinary project sync no longer loads `~/.agnostic-ai/` as a low-precedence spec layer; it is now the explicit global source for `sync --global` (#680).
- Crush docs record that `crush.json` is the vendor's deprecated legacy format, merged with `crushrc`; emission is unchanged (#674).

### Fixed

- `sync --global` removes a hooks file left with nothing in it instead of writing `{"hooks": {}}`; a file holding other keys is kept (#680).
- `sync --jobs` no longer fails intermittently on a directory two adapters share.
- Kiro and Antigravity docs record a conflict between two vendor pages and drop a stale "stays unconfirmed" note (#675).

## v0.51.0 - 2026-09-04

### Added

- MCP servers stop dropping vendor-documented fields on ten targets, such as Claude Code `timeout` and Kiro `oauth` (#634, #641, #661).
- Augment MCP servers merge into `.augment/settings.json` under `mcpServers`, keeping the file's other settings (#633).
- Copilot MCP servers also emit to `.github/mcp.json`, the file Copilot CLI reads; override with `outputs.copilot.cli-mcp-file` (#646).
- Agents reach the subagent loader on Windsurf, Trae, and Antigravity with `model` and `tools`; stale `agent-<name>.md` rules are swept (#638).
- Windsurf maps an agent's `tools` onto Devin's own names under `allowed-tools`; on Antigravity, set `x-antigravity.tools` (#638).
- Windsurf writes the shared root `AGENTS.md` that Devin CLI reads, so unscoped rules reach a windsurf-only repo (#645).
- Factory and Goose emit skills to the shared `.agents/skills/` tree; neither had a skill surface before (#632).
- OpenHands emits a scoped rule as a path-triggered `.agents/skills/<name>/SKILL.md`, so it loads only for matching files, not always from `AGENTS.md` (#643).
- OpenHands emits an environment spec's `install` field as `.openhands/setup.sh`, the vendor's repository bootstrap script (#662).
- Codex hooks accept `async: true` to run in the background, and `import codex` reads it back (#636).

### Changed

- Qoder MCP servers move from `.mcp.json` to `.qoder/settings.json`. In a Qoder-only project, delete the leftover `.mcp.json`, which still outranks it (#641).
- Warp MCP servers stop emitting `description`, `disabled`, and `roots`, which Warp ignores; use `x-warp` to keep one (#641).
- Codex's sweep of its old `.agents/agents/` tree only removes `.toml`, so it no longer deletes Antigravity subagents in the same directory (#638).

### Fixed

- Scoped Cline and Continue rules stop loading on every request, using Cline's `paths` and Continue's `globs`; `x-continue.regex` reaches the file too (#639).
- `validate` accepts the `PreModelSwitch`, `PostModelSwitch`, `Interrupt`, and `AgentSpawn` hook events, which already emitted correctly but failed CI (#660).
- Warp's skills doc names `WARP_SKILL_DIRS` for Cloud agents, not the `SKILLS_DIRS` it claimed before, which did nothing (#663).
- Kilo Code's docs note that `.kilo/kilo.jsonc` outranks the root `kilo.jsonc` this adapter writes when both exist (#644).

## v0.50.0 - 2026-08-28

### Added

- `outputs.copilot.root-mcp-file` opts in to writing Copilot MCP servers to a root `.mcp.json`, the file Copilot CLI and VS Code's Agent Host read (#610, #622).
- Spec bodies expand path variables such as `{{$SKILLS_DIR}}` and `{{$MCP_FILE}}` to each target's own location, honoring `outputs.<target>` overrides (#616).
- `lint` warns on frontmatter keys that near-miss one agnostic-ai owns, such as `allowed_tools`, which parse but leave the agent unrestricted (#617).

### Changed

- `validate` exits 1 when it reports any issue, and `doctor --check-globs` exits 1 when a rule's globs match no files, so CI can gate on both (#617).

### Fixed

- Zed rules reach Zed again: sync writes its entry point to `.rules`, so enabling copilot beside zed no longer hides every rule (#624).
- OpenCode's entry point moves from `.opencode/AGENTS.md` to the root `AGENTS.md`, the file OpenCode reads; a managed file at the old path is swept (#623).
- Windsurf scoped rules land at `<scope>/.devin/rules/<name>.md`, where Devin finds them, and `alwaysApply: false` rules carry a matching `trigger` (#628).
- Gemini ignore specs emit as `.geminiignore`, the file Gemini CLI reads, and a managed `.aiexclude` is removed (#625).
- Kiro hook files write `"version": "v1"` as the schema documents instead of the number `1`, which a validating parser rejects (#626).
- Factory droids translate `tools` onto Droid CLI's own IDs, so a droid no longer fails to load on Claude-style names (#627).

## v0.49.0 - 2026-08-13

### Added

- Junie subagents emit at `.junie/agents/<name>.md`, so `tools`, `model`, and `mcpServers` reach Junie. Override with `outputs.junie.agents-dir` (#604).
- Junie slash commands emit at `.junie/commands/<name>.md`. Command specs targeting junie previously had no emission path at all (#605).
- Windsurf MCP servers emit to `.devin/mcp_config.json`, the project file Devin Local reads, instead of skipping every MCP spec (#587).
- Trae rules and agents carry `description`, `globs`, and `alwaysApply` frontmatter, so a glob-scoped or manual rule works on Trae (#607).
- With the `outputs.goose.rules-file` opt-in, a Goose rule scoped to a subdirectory writes a nested `<scope>/.goosehints` (#608).
- Antigravity and OpenCode MCP servers pass through extra `x-antigravity` and `x-opencode` fields, such as their `oauth` settings (#588).
- `import antigravity` reads `.agents/mcp_config.json` back into MCP specs, so a synced setup round-trips (#589).
- OpenHands streamable-HTTP MCP servers emit `timeout`; an `sse_servers` entry that sets it gets a coverage note (#588).
- Qoder agents emit `color`, matching Augment and Kilo Code (#588).

### Fixed

- Crush emits a `type: sse` MCP entry as `"type": "sse"` instead of `"http"`, so an SSE-only server connects (#586).
- Warp MCP stdio entries emit `working_directory` from a spec's `cwd` instead of dropping it, and no longer carry the undocumented `type` field (#592, #606).
- The `sync --watch` tests no longer flake on Windows CI: they wait for the watcher to arm before editing a file (#585).
- Capability tables drop duplicate `trae` and `qoder` rows, a test rejects duplicates, and the MCP and `disabled` target lists are corrected (#597).
- `docs/user/spec-format.md` adds a `color` table by target: Augment, Kilo Code, and Qoder share the key but not its values (#609).
- Targets docs cover `x-zed.disable-model-invocation`, mark windsurf's local MCP file won't-fix, and merge two Skills bullets (#557, #558, #590, #609).
- Amp skills scope their own MCP servers through `x-amp.mcpServers`, which the passthrough already emitted; this is now documented (#591).
- Warp's workflow doc link, its skill-scan directory count, Junie's lookup-order note, and five dead `sources.md` citations are corrected (#590).

### Changed

- The `target-audit` skill records four lessons from the 2026-08-09 run, such as addressing agents by spawn ID.

## v0.48.1 - 2026-08-09

### Changed

- `golang.org/x/sync` moves to 0.19.0, not Dependabot's 0.22.0 (#514), which would raise the minimum Go for `go install` to 1.25.

### Fixed

- The install docs, landing page, and contributing guide say Go 1.24+, matching `go.mod`.

## v0.48.0 - 2026-08-09

### Added

- `lint` fails on a spec whose frontmatter opens with `---` and never closes (LINT006), which every target otherwise wrote out as a broken file.
- `scripts/e2e_test.sh` drives the built binary through scaffold, sync, import, and revert on a throwaway project, and runs in CI via `make test-shell`.

### Fixed

- `lint` reports two specs of the same kind with the same `name` (LINT003), which the loader silently collapsed into one (#582).
- `lint` no longer flags hooks that share an event and matcher, since they all run; LINT002 is retired and CI now runs `lint` (#171, #582).
- The LSP reports the same lint findings as `lint`.
- The first `sync` in a fresh project no longer gitignores `.agnostic-ai/AGNOSTIC_AI.md`, so the first commit includes the shared instruction body (#580).
- The install docs mark `npx`, `winget`, and `scoop` as unpublished, and the landing's Windows tab points at the PowerShell install script.

## v0.47.1 - 2026-08-09

### Fixed

- A `tools` list on a Cursor agent raises a coverage note instead of vanishing, and `docs/user/spec-format.md` gains a per-target `tools` table.

## v0.47.0 - 2026-08-08

### Added

- Kilo Code agents emit `color` and `mode`; `disable`, `hidden`, `steps`, `temperature`, and `top_p` stay reachable through `x-kilo` (#562).
- Qoder skills emit to `.qoder/skills/<name>/SKILL.md` instead of skipping with a warning, and `import qoder` reads them back (#558).
- Kiro agents translate a spec's `tools` list onto Kiro's `read`, `write`, `shell`, and `web` tags; other names raise a coverage note (#559).
- `sync -t warp` writes skills to `.agents/skills/<name>/SKILL.md`, the shared tree Warp's docs recommend, instead of a coverage note (#557).

### Changed

- The `target-audit` skill and auditor agent record two weeks of lessons, such as trusting `gh pr checks` over the stale `statusCheckRollup`.

### Fixed

- The capability-map parity test also catches a target that drops a kind but stays in the map.
- The release workflow now skips optional npm publishing when `NPM_TOKEN` is absent instead of attempting to publish with `setup-node`'s placeholder credential.
- `import zed` reads every MCP server shape the `.zed/settings.json` it writes can hold, including string `command` and remote `url` entries (#546).
- `sync -t junie` inlines rule and agent bodies into `.junie/AGENTS.md`, the file Junie reads, instead of `.junie/rules/`; `import junie` reads both (#552).
- OpenCode MCP servers write `"enabled": false` for a `disabled: true` spec, so OpenCode no longer runs a disabled server (#555).
- `sync -t amp` no longer writes Command specs to `.agents/commands/`, which Amp never reads; they skip with a warning (#553).
- Antigravity MCP servers emit `headers` on remote entries, `cwd` on stdio entries, and `disabled` on both (#556).
- `sync -t openhands` writes `{ url, api_key }` for an sse or shttp MCP entry with `api_key`; a `headers`-only credential raises a coverage note (#554).
- `sync -t factory` skips an agent spec with an empty body with a coverage note instead of writing a file Droid rejects (#561).
- `sync -t trae` emits MCP servers to `.trae/mcp.json`, the project registry Trae documents, instead of skipping every MCP spec (#560).
- Vendor doc citations and stale target claims are refreshed from the 2026-08-08 audit, including Codex links moving to `learn.chatgpt.com` (#563).

## v0.46.0 - 2026-08-07

### Added

- `scripts/install.sh` (macOS, Linux) and `scripts/install.ps1` (Windows) install the latest release without a package manager and verify its checksum.
- Windows package managers: `winget install Chemaclass.agnostic-ai` and `scoop install agnostic-ai` from the `Chemaclass/scoop-bucket` bucket.
- npm wrapper: `npx agnostic-ai <command>` with no install, or `npm install -g agnostic-ai`; it downloads and verifies the platform binary.
- Claude Code plugin with four skills: `/plugin marketplace add Chemaclass/agnostic-ai`, then `/plugin install agnostic-ai@chemaclass`.
- `upgrade` recognizes Scoop, winget, and npm installs and prints their update command instead of falling back to a manual download.

### Fixed

- `sync` no longer fails at random with `mkdir <dir>: invalid argument` when two targets create the first children of a shared directory (#526).

## v0.45.0 - 2026-08-02

### Added

- MCP servers reach `factory` (`.factory/mcp.json`), `qoder` (`.mcp.json`, shared with Claude Code), and `openhands` (`config.toml`), now 17 of 25 targets.
- The `target-audit` skill audits every adapter against vendor docs and files issues; `--fix` spawns `adapter-fixer` agents that open PRs but never merge.
- MCP servers accept `type: ws`, emitted with the remote shape (`url`, `headers`, `type`) instead of a malformed entry.
- Seven new targets (#479): `qoder`, `openhands`, `factory`, and `kilo` join the default set, while `jules`, `goose`, and `augment` stay opt-in.
- Antigravity MCP servers land in `.agents/mcp_config.json`, with `serverUrl` for remote entries as Antigravity requires.
- Augment gets native rules, agents, and skills under `.augment/rules/`, `.augment/agents/`, and `.agents/skills/`; restrict tools with `x-augment.tools`.
- Trae skills emit as folders that keep bundled assets, commands emit at `.trae/commands/<name>.md`, and `import trae` reads both.
- Kiro agents emit at `.kiro/agents/<name>.md`, where Kiro's agent picker reads them; set tools with `x-kiro.tools`, since a generic `tools` list raises a note.
- Kiro hooks land in `.kiro/hooks/<name>.json`, one file per hook; `disabled: true` writes `"enabled": false`.
- Kilo Code skills emit into the shared `.agents/skills/<name>/SKILL.md` tree, which Kilo loads by default, instead of a second copy under `.kilo/skills/`.
- Qoder agents emit at `.qoder/agents/<name>.md` with `model`, `tools`, `skills`, and `mcpServers`, and `import qoder` reads them back (#529).
- Windsurf ignore specs merge into `.devinignore`, gitignore syntax under a provenance header, matching the shape Aider, Cursor, and Gemini already use (#530).
- Crush MCP http and sse entries accept `oauth` settings (#531), and `x-crush.user-invocable: true` adds a skill to the command palette (#540).
- Codex and Gemini MCP stdio servers accept `cwd`, Codex http servers accept `auth` (#532), and Codex hooks accept `additionalContextLimit` (#533).
- Warp workflows and Zed Tasks accept `x-warp` and `x-zed` passthrough for their documented fields, and `import` reads them back (#538, #539).
- Kilo Code rules also emit at `.kilo/rules/<name>.md`, listed in `kilo.jsonc` `instructions`, which Kilo ranks above AGENTS.md (#535).

### Fixed

- Claude hooks accept `DirectoryAdded`, and Claude settings support `attribution`, `statusLine.refreshInterval`, and `statusLine.hideVimModeIndicator`.
- Codex MCP servers emit and import `env_vars` and `env_http_headers`, and `import codex` keeps hook `commandWindows` and prefers `.codex/agents` (#532, #547).
- Codex agents no longer emit a Claude-style `tools` array that Codex rejects; it raises a coverage note, and `x-codex.tools` emits native `[tools]`.
- Concurrent sync no longer fails at random with `mkdir .agents/...` errors when Codex legacy cleanup races other targets.
- Kilo Code MCP servers write the `mcp` key Kilo reads, with `type`, one `command` array, `environment`, and `"enabled": false` for a disabled spec.
- Kilo Code agents drop the `name` and `tools` frontmatter Kilo ignores; a `tools` allowlist raises a coverage note.
- `docs/user/targets.md` corrects Warp's MCP `type` field, Codex's `SessionEnd` event and Starlark policies, Gemini's hook list, and Amp's commands path.
- Ten stale vendor doc URLs in the target-audit sources are refreshed, including Codex's move to `learn.chatgpt.com` and Kilo's to `kilo.ai`.
- Codex exec-policy `decision` takes `allow`, `forbidden`, or `prompt`; the old `ask` wrote a rule Codex ignored.
- `outputs.claude.settings.enabledPlugins` is a map of `plugin-id@marketplace-id` to boolean, matching Claude Code, so a plugin actually enables.
- MCP `disabled: true` maps to Codex's `enabled = false`; Claude Code, Cursor, and Copilot get a coverage note, since they cannot pre-disable a project server.
- Cline and Windsurf skills emit as a folder per skill that each tool loads, and `import cline` and `import windsurf` read both the new and old forms.
- Cline rules and agents default to `.cline/rules` and `.cline/agents`; keep `.clinerules` with `outputs.cline.rules-dir` (#534).
- Antigravity rules and skills default to `.agents/rules` and `.agents/skills`, where skills dedupe with other targets; stale `.agent/` copies are swept.
- `validate` and `sync --watch` no longer call specs dead weight or skip `antigravity`, `factory`, `qoder`, `openhands`, and `augment`.
- Junie skills emit as `.junie/skills/<name>/SKILL.md` with their assets, and `import junie` reads both the new and old flat forms.
- Junie also writes `.junie/AGENTS.md`, which it reads before the root `AGENTS.md`.
- Cursor rules with `alwaysApply: false` and no `globs` stay relevance-selected or manual instead of attaching to every file (#536).
- `validate` recognizes all 11 documented Gemini hook events instead of 4 (#537).
## v0.44.0 - 2026-07-24

### Added

- `new --dry-run` previews a spec scaffold without writing it, and `init` points at `agnostic-ai completion <shell>` for tab completion (#495).
- `import kiro` and `import crush` read their rules, skills, and MCP back, so all 18 emitted targets now import (#494).
- `--profile <file>` writes a CPU profile, and `sync --verbose` shows per-target wall time, so a slow sync points at one adapter (#491).
- `sync --check --diff` prints a diff per drifted file, `--format=github` emits PR annotations, and a failing check prints the fix command (#488).
- `sync --jobs <n>` emits targets in parallel, one worker per CPU by default; `1` forces serial (#487).

### Changed

- `sync --watch` re-syncs only the targets that emit the changed spec's kind; config edits, deletes, and renames still re-sync everything (#490).

## v0.43.0 - 2026-07-20

### Changed

- Gemini skills emit natively as `.gemini/skills/<name>/SKILL.md` folders with bundled assets; `emit-skills-as-commands` now only adds the command form (#439).
- Zed reads the shared `AGENTS.md` and gets native skills under `.agents/skills/`; the merged `.rules` file is opt-in via `outputs.zed.rules-file` (#439).
- OpenCode agents emit natively as subagents under `.opencode/agents/` and skills under `.opencode/skills/`, no longer flattened to commands (#439).
- Copilot agents emit natively as `.github/agents/<name>.agent.md` and skills under `.github/skills/`, replacing flattened `.instructions.md` copies (#439).
- Cline joins the shared root `AGENTS.md` entry-point, which current Cline reads as cross-tool project instructions (#439).
- All SKILL.md emitters share one renderer, so shared `.agents/skills` trees stay byte-identical; Antigravity SKILL.md gains `x-antigravity` passthrough (#439).
- Windsurf rules move to `.devin/rules/`, where Devin Desktop looks; keep the old path with `outputs.windsurf.rules-dir: .windsurf/rules` (#473).

- Codex skills move from `.codex/skills`, which Codex never read, to `.agents/skills`, the directory it scans; Amp writes the same tree.
- Codex reads prompts only from `~/.codex/prompts`, so commands no longer emit to `.codex/prompts`; set `outputs.codex.commands-dir` to keep them.
- Cursor emits natively: skills under `.cursor/skills/`, subagents under `.cursor/agents/`, and commands to `.cursor/commands/` by default (#430, #439).
- Cursor Bugbot files move to where Bugbot reads them: `.cursor/BUGBOT.md` at the root and `<scope>/.cursor/BUGBOT.md` per scope.
- Claude rules drop the Cursor-only `alwaysApply` frontmatter on emit; Claude's rule schema defines only `paths` and an unscoped rule is always-on already.
- Targets writing byte-identical content to one path dedupe instead of erroring; only divergent content collides.
- Docs stop calling `.claude/rules/` inert: current Claude Code loads it, so `outputs.claude.rules-mode: import` only matters on older versions.

### Added

- Four new default targets, all reading the root `AGENTS.md`: `junie`, `kiro`, `crush`, and `trae`; `import junie` and `import trae` read their rules (#474).
- Claude rules take `globs`: it emits as `paths:` in `.claude/rules/*.md`, and `paths` maps back to Cursor `globs`, so one spec scopes both tools.
- Hook specs pass through current per-tool fields, such as Claude's `async` and `if`, Codex's `commandWindows`, and Cursor's `failClosed`.
- `validate` knows every documented Claude Code hook event and Codex's `SubagentStart`, `SubagentStop`, and `PermissionRequest`.
- `import cursor` captures native `.cursor/agents/*.md`, `.cursor/skills/<name>/` folders (full tree), and `.cursor/commands/*.md` alongside rules.
- `sync.shared-skills` opt-in symlinks byte-identical skill folders across targets to one copy, preferring `.agents/skills/<name>` (#437).
- Import reads the new native trees: `.gemini/skills/`, `.opencode/agents/` and `.opencode/skills/`, `.github/agents/*.agent.md`, and `.github/skills/` (#439).

## v0.42.0 - 2026-07-03

### Changed

- The managed `.gitignore` header warns that listed paths are not committed, so a fresh clone or worktree needs `agnostic-ai sync`.

### Added

- "Why not symlinks" doc explaining how agnostic-ai compares to symlinks, manual copies, and shared-file `@`-includes, plus when a simpler option suffices.
- The managed `.gitignore` block ignores Claude Code's `/.claude/agent-memory/` and `/.claude/settings.local.json` when `claude` is enabled. (#469)

## v0.41.0 - 2026-06-19

### Added

- `sync.dropped-summary` opt-in prints, per target, what it could not fully emit and the key that would carry it. (#441)

### Fixed

- `doctor --fix` keeps your own keys in merged files such as `opencode.json`, and `doctor` and `sync --check` stop reporting false drift there. (#215, #465)
- An `x-<target>` block adding several new frontmatter keys emits them in stable order, so `sync --check` stops flagging false drift.
- A spec `name:` with path separators or `..` is rejected, and `sync` refuses any path outside the project root, closing a path traversal.
- Gemini TOML honors an `x-gemini.description` override instead of always using the top-level description.

## v0.40.0 - 2026-06-17

### Added

- `settings` kind (`.agnostic-ai/settings/*.yaml`): single-source `permissions` and default `model`. Claude maps them into `.claude/settings.json`. (#432)
- `review` kind (`.agnostic-ai/reviews/*.md`): Cursor emits scope-located `BUGBOT.md` files. (#433)
- `environment` kind (`.agnostic-ai/environments/*.yaml`): Cursor emits `.cursor/environment.json`. (#434)
- `ignore` kind (`.agnostic-ai/ignore/*.md`): one list emits Cursor `.cursorignore`, Gemini `.aiexclude`, Aider `.aiderignore`. (#435)
- `command` kind emits natively to Gemini (`.gemini/commands/*.toml`), OpenCode, and Amp, and to Cursor when `commands-dir` is set. (#436)
- Cursor hooks: `hook` kind emits `.cursor/hooks.json`. Override via `outputs.cursor.hooks-file`. (#438)
- `doctor`: "Unmanaged config" block lists agentic files on disk not generated from `.agnostic-ai/`, each with the `import` to adopt it. (#440)
- `validate`: warns for each declared `sources.<kind>` whose directory is missing. (#444)

### Fixed

- Cursor `.mdc` emit preserves your frontmatter, so adopting agnostic-ai on existing Cursor rules diffs clean. (#443)
- `validate` / `lint` stop flagging `command` specs as orphaned when only Gemini, OpenCode, Amp, or Cursor are enabled. (#436)
- `import` strips the provenance header when seeding `.agnostic-ai/AGNOSTIC_AI.md`, so import→sync stays byte-stable. (#429)
- `import cursor` drops the catch-all `globs` and empty `description`, so always-apply rules round-trip stable. (#429)
- `sync` warns when a skill bundles files Cursor cannot represent (flattened to one `.mdc`). Previously dropped silently. (#430)
- Extra markdown inside a folder skill (e.g. `skills/<name>/examples.md`) is a bundled asset, not a phantom skill. (#431)

## v0.39.0 - 2026-06-16

### Added

- `outputs.claude.rules-mode: import` adds `@`-imports for `.claude/rules/*.md` to the `CLAUDE.md` pointer, so Claude Code loads them. (#424)
- `sync.resolve-imports` (`passthrough`, `strip`, `inline`) sets how `@path` import lines reach targets that cannot resolve them. (#425)

### Changed

- `scripts/release.sh` drops empty `### ` subsections from `[Unreleased]` before promoting it, so a release never ships scaffolding headings with no entries.

## v0.38.0 - 2026-06-16

### Fixed

- `import` walks rule directories recursively for cursor, claude, and copilot, preserving nested subdirectories instead of dropping them. (#411)
- Nested rule, agent, and skill specs emit under the tool's rules directory (`.cursor/rules/backend/auth.mdc`), not a stray `<scope>/` tree at the root. (#412)
- `import` quotes `description` frontmatter containing `: `, quotes, or surrounding whitespace, so imported specs pass `validate`. (#413)
- Managed `.gitignore` ignores generated subdirectories such as `/.claude/rules/` instead of all of `/.claude/`, so hand-written siblings stay tracked. (#414)
- `import` warns when another target's entry-point holds unique content the next `sync` would overwrite. (#415)

## v0.37.0 - 2026-06-14

### Added

- `sync` prints a `note:` line when a target supports a kind but emits it only behind an opt-in key, so skipped content is visible. (#404)

### Fixed

- `init` now offers Antigravity in the target prompt and enables it with `--all`. It was absent before.
- `lint --help` now states the real exit code (`1`, not `2`).
- `explain` and `why` now credit rules inlined into entry-point files (`AGENTS.md`, `GEMINI.md`, ...) instead of omitting them. (#405)
- `copilot` always-on rules now emit as `applyTo:"**"` instruction files instead of being dropped. (#403)

## v0.36.0 - 2026-06-12

### Changed

- `.gitignore`: the local-override config, sync-state file, and packs dir move into the managed block, deduplicated and root-anchored. (#401)

### Fixed

- `codex`, `amp`, `warp`, `gemini`, `aider`, and `opencode` now inline rule bodies into their entry-point file instead of dropping them. (#399)

## v0.35.0 - 2026-06-09

### Added

- `sync.target-overview` opt-in appends to each entry-point file a list of where that tool's generated files live; `import` strips it again. (#397)

## v0.34.0 - 2026-06-06

### Changed

- The `sources:` block is optional; an omitted kind defaults to `.agnostic-ai/<kind>`, the tree `init` scaffolds.

### Fixed

- Docs state that `sync` enables 12 targets by default, with Amp and Warp opt-in, instead of implying all 14.

## v0.33.0 - 2026-06-05

### Added

- `gitignore.allow` re-allows patterns at the end of the managed `.gitignore` block, so a tracked file such as a fixture `AGENTS.md` stays tracked. Closes #388.

### Changed

- `init` writes `gitignore.enabled: true` by default, so generated outputs stay out of git; pass `init --gitignore=false` to commit them.

### Fixed

- `cleanup` removes only the `.bak` backups `sync --backup` wrote, not every `*.bak` in the project. Closes #390.
- `revert` restores entry-point files such as `CLAUDE.md` and `AGENTS.md` from `.bak`, and `revert --force` deletes the generated ones. Closes #389.
- Flat-file skills no longer copy sibling skills' bodies into each emitted skill folder on claude, codex, amp, and antigravity. Closes #387.

## v0.32.1 - 2026-06-04

### Changed

- Docs: README capability matrix shows per-kind support depth (native / bundled / opt-in / none) instead of a 3-state glyph.

### Fixed

- Docs: README, targets.md, configuration.md, and spec-format.md match adapter behavior, including hook paths, `AGENTS.md` dedupe, and every `outputs.*` key.

## v0.32.0 - 2026-06-03

### Added

- Antigravity emits skills natively as `.agent/skills/<name>/SKILL.md`. Configurable via `outputs.antigravity.skills-dir`. Closes #371.
- Amp emits skills natively as `.agents/skills/<name>/SKILL.md`. Configurable via `outputs.amp.skills-dir`. (#377)

### Changed

- Amp skills move from `.agents/commands/` to native `.agents/skills/<name>/SKILL.md`; `outputs.amp.emit-skills-as-commands` no longer affects Amp. (#377)
- Docs: Codex hooks emit to `.codex/hooks.json`, not `config.toml`. (#377)

### Fixed

- Antigravity skills no longer warn as unsupported. (#371)
- Zed MCP uses the current `context_servers` schema: flat `command`/`args`/`env` for stdio, native `url`/`headers` for remote. Closes #373.
- Continue MCP files use the required `mcpServers:` block wrapper; flat single-server files did not load. Closes #374.
- Remote (HTTP/SSE) MCP servers carry an explicit `type` in `.mcp.json`, `.cursor/mcp.json`, and `.warp/.mcp.json`. Closes #375.
- Gemini SSE MCP servers use `url`, not `httpUrl`. Closes #376.

## v0.31.0 - 2026-06-03

### Added

- `x-<target>` keys reach every target with frontmatter or TOML to hold them, stay scoped to that target, and emit in sorted order. Closes #367 (#368, #369).

### Changed

- Docs: `README.md` and every `docs/` page say the same facts in fewer words; `CHANGELOG.md` gains an entry-style line.

## v0.30.0 - 2026-06-02

### Changed

- The managed `.gitignore` block ignores each generated directory with one rule such as `/.claude/` instead of listing every file in it.

## v0.29.0 - 2026-06-02

### Added

- Docs: recommend gitignoring generated outputs via `gitignore.enabled`.

### Changed

- Docs: `configuration.md` fixes the default target set (12 of 14; omits `amp`, `warp`) and links `targets.md` instead of duplicating the entry-point list.

### Fixed

- Restored overlay helper files such as `.claude/README.md` land in the managed `.gitignore` block and the output ledger instead of showing up untracked.
- `sync --only` and `--except` no longer delete the files of targets they skip; cleanup waits for the next full sync.
- Managed `.gitignore` entries are root-anchored (`/AGENTS.md`), so a generated path no longer ignores a same-named file nested elsewhere. (#362)
- `import claude` propagates a nested `.claude/CLAUDE.md`'s instructions to every target, not just the claude overlay. (#361)
- Playground: optional source reads treat a missing `js/wasm` filesystem as absent instead of failing with `ENOSYS`.
- Playground renders each target's root entry-point file (CLAUDE.md, AGENTS.md, ...) alongside per-spec artifacts.

## v0.28.0 - 2026-06-01

### Added

- Per-target models: `model:` accepts a map such as `{claude: opus, codex: gpt-5.5}`, with an optional `default`; a bare string applies to every target.

## v0.27.0 - 2026-05-29

### Added

- `import antigravity` and `import continue` (rules + MCP yaml) close the sync -> import -> sync round-trip for both targets.
- `.agnostic-ai/AGNOSTIC_AI.md` is now the canonical entry-point body, tracked in git.
- `sync` ledger at `.agnostic-ai/.sync-state` sweeps orphan generated files on the next run.

### Changed

- Every generated file carries the provenance header with a "do not edit" hint.

### Fixed

- Every adapter passes a full audit: golden output, capability parity, provenance headers, and byte-equal sync -> import -> sync where supported.
- Cursor `.mdc` frontmatter double-quotes `globs:` so `**/*` parses as a YAML string instead of an anchor reference.
- Import unwraps `## Rules`, `## Agents`, and `## Skills` wrappers and provenance comments, so legacy concatenated rules files round-trip byte-equal.
- `import codex` reads `outputs.codex.rules-file` and more MCP fields; sync removes an empty `.codex/config.toml` and legacy `.agents/` trees.
- Shared MCP JSON importer synthesizes `type: http` from url-bearing entries so claude/amp/opencode pick the http transport branch on emit.
- Per-adapter `testdata/kitsink/` trees ship in git even when the underlying patterns are gitignored.

## v0.26.1 - 2026-05-27

### Changed

- `agnostic-ai upgrade` and the install docs print `brew upgrade --cask Chemaclass/tap/agnostic-ai`, which avoids ambiguity with the formula namespace.

### Fixed

- `upgrade --run` stops before a doomed `brew upgrade --cask` when `<brew>/bin/agnostic-ai` is a stray file, and prints a `rm + brew install --cask` hint.

## v0.26.0 - 2026-05-25

### Added

- `import.codex.shred: false` keeps each `AGENTS.md` as a single rule spec instead of splitting by `##` heading. Closes #248.
- Hook frontmatter accepts `target: <name>` or `targets: [a, b]` to scope a hook to specific CLIs. Closes #249.
- `agnostic-ai doctor` flags divergent hook script bodies across tools and suggests consolidating to `.agnostic-ai/scripts/<basename>`. Closes #251.
- `outputs.codex.exec-policies` / `outputs.codex.exec-policies-file` render Codex CLI's `prefix_rule(...)` DSL into `.codex/rules/default.rules`. Closes #254.
- Codex hooks emit into `.codex/hooks.json` with matcher-aware dedupe, `timeout`, and `statusMessage`; override with `outputs.codex.hooks-file`. Closes #255.
- `target:`, `targets:`, and their `-exclude` forms scope every spec kind, not only hooks. Closes #292.
- Per-target body fences: wrap prose in `::target codex` / `::end` markers to emit that block only to the named target. Closes #293.
- `import claude` / `import codex` auto-set `target: <tool>` on agents/skills present in only one tool's tree. Closes #299.
- `import codex` auto-fences divergent agent and skill bodies: shared prose stays plain, each tool's own section goes in a `::target` block. Closes #300.

### Changed

- `import <tool>` auto-sets `target: <tool>` on imported hooks so codex-specific scripts no longer leak into claude `settings.json`. Closes #257.
- Codex `agents-dir` default changed from `.agents/agents` to `.codex/agents`. Override via `outputs.codex.agents-dir`. Closes #252.
- Codex `skills-dir` defaults to `.codex/skills`, and `shared-subagents` defaults to `true` whether or not claude is enabled. Closes #253.

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
- Routing keys (`target`, `targets`, and their `-exclude` forms) no longer leak into emitted frontmatter. Closes #303.
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

- Capability warnings group by kind across targets in one line, such as `! 5 hooks unsupported by cursor, copilot, aider, ...`, with one suppression hint.
- `sync -v` prints per-target `created / updated / unchanged` counts, and the footer counts only files that changed.
- Status symbols are colorized on tty (`✓` green, `!` yellow, `✗` red). Honors `NO_COLOR=1`. Pipes and redirects stay plain.
- The `sync --watch` banner shows the path count and backend, and each re-sync starts with a timestamped `[HH:MM:SS] change · <path>` line.
- Unchanged capability warnings shrink to a one-line reminder on repeat runs. Delete `.agnostic-ai/.sync-state` to show the full block again.

## v0.23.0 - 2026-05-17

### Added

- `agnostic-ai upgrade` prints the upgrade command for your install method; `--run` runs it and `--check` flags `PATH`-shadowed copies.
- Integration tests: `sync --check` and `doctor --fix` are zero-drift no-ops after `import claude` / `import codex` / both. Closes #232.
- README: `brew upgrade` line + releases page link.
- Docs cover the v0.22 import changes: `import codex` reads `.codex/prompts/` and `.codex/config.toml`, and `import claude` reads `.mcp.json`.
- Import summary prints `→ <overlay> seeded from <native>` for `import claude` and `import codex` when overlay file written. Closes #231.
- `sync --watch` re-emits when you edit `.agnostic-ai/overlays/`, such as `claude.settings.json` or `codex.config.toml`. Closes #234.
- Integration tests cover six round-trip edge cases, such as MCP `env` and `headers`, folded YAML scalars, and skill assets keeping exec bits. Closes #233.

## v0.22.0 - 2026-05-17

### Added

- Integration tests chain `claude → import → sync codex → import → sync claude` and the inverse, checking that every shared kind survives.
- `import codex` reads `.codex/prompts/*.md` into commands, so Codex slash prompts are no longer dropped and overwritten on the next sync.
- `import codex` saves other `.codex/config.toml` keys, such as `model`, to `.agnostic-ai/overlays/codex.config.toml`, and sync writes them back.
- `import claude` reads `.mcp.json`, one MCP spec per server, so Claude Code MCP servers reach other targets instead of being dropped.

### Changed

- Codex agent TOML keeps nested tables under `x-codex`, such as an agent-scoped `[mcp_servers.<name>]`, instead of dropping them on the next sync.
- `.claude/settings.json` writes hooks in lifecycle order with `{matcher, hooks}` and `{type, command}` key order, even on a first sync.

### Fixed

- Frontmatter keeps each scalar's source quoting: a plain `argument-hint: <ver>` stays plain and a double-quoted value stays double-quoted.

## v0.21.0 - 2026-05-16

### Changed

- Releases publish a Homebrew cask (`Casks/agnostic-ai.rb`) that clears macOS quarantine; `Formula/agnostic-ai.rb` stops getting updates. Closes #225.

### Removed

- `autoSync`, `sync --auto-sync`, the first-run auto-sync prompt, and the `auto-sync` rule. Existing `autoSync:` keys are ignored.

## v0.20.0 - 2026-05-16

### Fixed

- Frontmatter emit no longer force-quotes plain `description:` scalars, so long descriptions stay on one line. Closes #226.

## v0.19.0 - 2026-05-16

### Added

- `revert --force`: delete adapter-emitted files without a `.bak` (restores pre-#217 behavior). Closes #217.

### Changed

- `cleanup` defaults to .bak removal; `--backups` kept as alias. Closes #219.
- `revert` preserves files without a `.bak` (helpers next to `SKILL.md`, propagated templates). Pass `--force` to delete. Closes #217.
- `outputs.codex.shared-subagents` defaults to `false` when `claude` is enabled (avoids duplicating `.claude/skills/`), `true` when codex is alone. Closes #216.

### Fixed

- `doctor --fix` keeps `enabledPlugins` and `statusLine`, a clean sync reports no drift, and import keeps overlay key order. Closes #215.
- Frontmatter scalars containing `<`/`>` keep their quotes; long descriptions no longer wrap at 80 cols. Closes #218.

## v0.18.0 - 2026-05-16

### Added

- `doctor --check-globs`: opt-in flag rules whose `globs:` matches no path in the working tree. Closes #208.
- `cleanup --backups`: removes `*.bak` left by `sync --backup`. Closes #197.
- `outputs.codex.shared-subagents` (default `true`): set `false` in claude+codex setups to drop the duplicate `.agents/skills/` tree. Closes #194.
- `spec.Entry.MetaKeys` exposes source frontmatter key order; external adapters receive it as `meta_keys` (additive, no protocol bump).

### Changed

- Frontmatter emit preserves source key order, uses 2-space sequence indent, prefers double quotes. Closes #190, #191, #193.
- `.claude/settings.json` keeps overlay key order and writes hook keys and events in documented order; codex and opencode JSON match. Closes #192.
- Capability warnings collapse to one line per target and kind, with a count and an `on-unsupported: silent` hint. Closes #204.
- `import --dry-run` lists paths + count instead of dumping file bodies. Matches `sync --plan`. Closes #205.
- `doctor` drift splits into "missing" vs "stale (edited locally since last sync)". Closes #207.

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

## v0.16.0 - 2026-05-15

### Changed

- `sync` distributes `AGNOSTIC_AI.md` to `CLAUDE.md`, `AGENTS.md`, and other entry points, seeds it when absent, and keeps content written by `import <target>`.
- `AGNOSTIC_AI.md` is no longer auto-added to `.gitignore`; commit it as a source file.
- `outputs` key in `agnostic-ai.yaml` is fully optional; omitted when empty in the JSON envelope sent to external adapters.

## v0.15.1 - 2026-05-15

### Changed

- `scripts/release.sh` refuses to cut a release with no new commits or an empty `[Unreleased]` section.

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
- `agnostic-ai lint`: LINT001 to LINT005 semantic checks, `--strict` (#171, #177).
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
- `claude`: `outputs.claude.settings.*` sets keys such as `model` and `outputStyle` over the captured overlay; spec hooks still win for `hooks` (#177).

### Changed

- **BREAKING**: every entry point gets one pointer body, plus `.agnostic-ai/AGNOSTIC_AI.md`. Set `outputs.<target>.rules-file` for the old concat (#153).
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
- `.claude/settings.json` keeps user keys: `import claude` saves them to `.agnostic-ai/overlays/claude.settings.json`, and sync adds spec hooks on top.
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
- Opt-in native surfaces: Copilot chat modes, Cursor commands, Cline, Windsurf, and Warp workflows, Continue assistants, Zed tasks (#104, #105, #106, #107, #108, #109, #110).

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
- Hooks and MCP reach Codex (#78), Gemini (#79), Continue (#80), Amp (#81), Zed (#82), Warp (#83), and OpenCode (#84) config files.
- New `outputs` fields: `instructions-dir`, `commands-dir`, `mcp-dir`, `emit-skills-as-commands`.
- `sync --json`, `sync --check --json`, `revert --json`, `doctor --json` (schema v1).
- `agnostic-ai status`: project name, layers, spec counts, targets, last sync, drift count. `--json`.
- First-sync target picker (#92): interactive multi-select when config still lists every target. Selection persisted.

## v0.5.0 - 2026-05-07

### Added

- `sync --only <targets>` / `sync --except <targets>` filters (mutually exclusive). `revert` gains same flags.
- `agnostic-ai validate --fix` rewrites autofixable specs, such as backfilling a missing `name:`; plain `validate` marks fixable issues with `*`.
- Plugin protocol v1 for external adapters (`agnostic-ai-adapter-<target>` on PATH; JSON over stdin/stdout). Docs at `docs/internal/plugin-protocol.md`.
- `adapters.Resolve(name)`: lookup site with built-in → external fallback.
- `agnostic-ai packs add|remove|update|list`: shareable spec packs from Git URLs. Pinned in `agnostic.packs.lock`. Load as a layer before project specs.
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
