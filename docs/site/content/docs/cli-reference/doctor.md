+++
title = "Doctor"
description = "Diagnose project and local setup problems, then follow the suggested next step."
weight = 31

[extra]
group = "Reference"
+++

# Doctor

## doctor

```bash
agnostic-ai doctor
```

Doctor checks the project and your local setup, then prints a final **Next step**. Read it before you repair anything. The [troubleshooting guide](@/docs/troubleshooting.md) covers symptoms.

It reports four kinds of generated file:

| Kind | Meaning |
|------|---------|
| missing | Never synced. |
| stale | Out of date with the specs. |
| edited | Changed since the last sync. |
| orphaned | No longer generated, kept because sync cannot prove it wrote the file. |

It exits non-zero on any drift, [lint](@/docs/cli-reference/check.md#lint) error, or untrusted, modified, or unreadable Codex hook. Hooks you disabled on purpose do not fail. It also names any [spec migration](@/docs/cli-reference/upgrades.md#migrate) that applies.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config). An unknown name fails with AAI-301, as in `sync`. |
| `--scope <all\|project>` | `all` (default) includes local Codex hook trust and your own Claude settings files. `project` skips them. |
| `--fix` | Write missing files and overwrite stale or edited ones. Like `sync`, it first saves each hand-edited file as `<path>.bak`. See below. |
| `--backup` | With `--fix`, copy every file to `<path>.bak` before it is overwritten or removed, not only hand-edited ones. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag broken relative Markdown links in generated skills. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus the lists in the [reference](#reference). Fails on the same findings as text mode. Not with `--fix`. |

On a fresh CI runner, run `agnostic-ai doctor --scope project --check-references`.

{% <details summary="Details: what --fix does, CI scope, and the packaging check"> %}
- `--fix` also removes each nested `CLAUDE.md` whose text is already in a scoped rule.
- In a terminal, `--fix` asks before removing each kept orphan and each group of [Claude skill overrides](#claude-skill-overrides). No is the default.
- Without a terminal, `--fix` keeps them and explains why. It exits non-zero while any orphan remains.
- `--backup` also covers removing a confirmed orphan or confirmed skill overrides.
- A hand-edited file whose `<path>.bak` already holds an earlier edit is kept, and `--fix` fails until you move both into `.agnostic-ai/` and delete the `.bak`.
- An edit made through a symlinked output is handled as `sync` handles it. See [hand edits](@/docs/cli-reference/sync.md#hand-edits).
- With `--scope project`, Codex hook trust shows `SKIPPED` in text. JSON adds `hook_trust_check: {"status": "skipped", "reason": "project scope skips local runtime trust"}` and an empty `hook_trust`.
- `--scope project --fix` repairs drift without reading or approving trust. `--json` stays read-only.
- When sync removes unchanged managed keys from a shared settings file but keeps your keys and edits, doctor reports the file as out of date. JSON reports its action as `stale`.
- A hand-written file that stops sync appears in JSON as `stale` with a `reason`: the text doctor prints for it, with the keep and replace steps on their own lines.
- The packaging check handles literal paths, `*`, `?`, character classes, and `**` segments. Braces, extended globs, escaped patterns, and unsupported classes give a coverage-check warning, not an uncovered claim.
- It reads only the root ignore file. It ignores `package.json` allowlists, built-in package exclusions, nested ignore files, and Dockerfile-specific ignore files. Verify with `npm pack --dry-run`, `vsce ls`, or your Docker build context.
{% </details> %}

## What doctor prints after the file report {#report-blocks}

| Block | What it shows | Counts as drift |
|-------|---------------|-----------------|
| **Spec health** | What `agnostic-ai lint` reports. An error fails doctor; a warning does not. | No |
| **Coverage notes** | With [`coverage.accept`](@/docs/configuration-reference.md#coverageaccept) set, how many notes it accepts (`coverage_accepted` in `--json`). `sync -vv` lists them. | No |
| **Packaging ignores** | Generated paths a root `.npmignore`, `.vscodeignore`, or `.dockerignore` does not cover. | No, reported only |
| **Tracked despite ignored** | A generated path git tracks and ignores, with the `git rm --cached` fix. | No |
| **Codex hook trust** | With Codex selected: inactive handlers in the project hooks file and user `hooks.json`. Next step: `/hooks`. | Untrusted, modified, or unreadable fails; disabled does not. Never grants trust. |
| **MCP** | Whether each stdio `command:` is on PATH, and which `${NAME}` references are unset in this shell. | No |
| **Claude hook specs** | A hook spec that runs its commands twice (see below). | No |
| **Hand-written config** | An instructions file or whole MCP file, such as `.cursor/mcp.json`, holding config agnostic-ai did not write, which [stops sync](@/docs/cli-reference/sync.md). Doctor names the `import` that keeps it. | Yes, `--fix` stops on it; `--fix --backup` replaces it and keeps `<path>.bak` |
| **Nested CLAUDE.md** | A hand-written `<dir>/CLAUDE.md` that repeats a scoped rule. | Yes, `--fix` removes it |
| **Script divergence** | Same-named hook scripts with different bodies across tools. | Yes, no automatic fix |
| **Unmanaged config** | Markdown and TOML config with no generated-file marker, a `.cursor/mcp.json` that names a server no spec has, and files sync never writes (such as `.cursorrules` or `.github/prompts/*.prompt.md`) until import has written their spec, and an OpenCode or Copilot original while sync writes a copy the tool also loads. Grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/sync-settings.md#syncunmanaged) entries, left out of Unmanaged config. | Never |
| **Global names** | A project skill or agent sharing a name with one in `~/.agnostic-ai/`, and which wins per tool. | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text. | Never |
| **Claude skill overrides** | Synced skills that Claude settings turn off or hide. See [below](#claude-skill-overrides). | Never |

{% <details summary="Details: the longer blocks"> %}
- Codex hook trust is read from `CODEX_HOME/config.toml` (default `~/.codex/config.toml`). When `CODEX_HOME` does not exist yet and the project has no Codex hooks file, the check is skipped.
- MCP: a `${NAME}` is checked in `env`, `headers`, `url`, and `args`, and listed by name only. A reference with a default is skipped. Install hints come with the missing commands.
- Packaging ignores: the report names the ignore file.
- Claude hook specs: with `claude` enabled, doctor flags a spec with several commands that `.claude/settings.json` already runs in one group with different settings, as an older `import claude` wrote it. To fix it, delete the spec, run `sync`, then run `import claude`.
- Nested CLAUDE.md: the file equals the body of a rule scoped to `<dir>`, as `import claude` leaves it. Claude Code would load it beside the synced rule.
- Script divergence: scripts under `.agnostic-ai/scripts/<tool>/`. Doctor suggests the path `.agnostic-ai/scripts/<basename>`.
- Global names: a name in [`sync.allow-global-names`](@/docs/sync-settings.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/global-configuration-reference.md#global-shared-names).
{% </details> %}

## Check skill links {#check-references}

`--check-references` reads each Markdown file a selected tool writes for its skills. Any broken link fails doctor. Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

{% <details summary="Details: which links count"> %}
- A link is valid when it resolves from the document's directory or from the project root.
- It skips code, URLs, absolute paths, and `#fragment`-only links.
- In a `file#fragment` link, it checks only the file.
- [`doctor.check-references.ignore`](@/docs/configuration-reference.md#doctorcheck-referencesignore) exempts destinations that can never resolve.
{% </details> %}

### Claude skill overrides

Claude Code's `/skills` menu saves each skill you switch off in your git-ignored `.claude/settings.local.json`. The skill still syncs, but Claude Code refuses to run it. Doctor lists the synced skills that Claude settings turn off, hide, or list without a description. The highest file comes first: local, shared, user.

```text
Claude skill overrides:
  ! .claude/settings.local.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: deploy
    → review them in Claude Code with /skills, run `agnostic-ai doctor --fix` in a terminal, or delete the entries from .claude/settings.local.json
  ! .claude/settings.json turns off 1 of 3 synced skills (off), so Claude Code refuses to run them: lint
    → team policy: set skillOverrides in .agnostic-ai/overlays/claude.settings.json and run `agnostic-ai sync`
  ! ~/.claude/settings.json hides 1 of 3 synced skills from Claude (user-invocable-only): deploy
    → applies to every project on this machine; delete the entries from ~/.claude/settings.json
  ~ ~/.claude/settings.json also sets 4 entries for skills sync does not write
```

To re-enable skills, run `doctor --fix` in a terminal. It asks once per state, and a yes removes only their entries from the project's `.claude/settings.local.json`.

{% <details summary="Details: files read, precedence, parse errors, and what --fix skips"> %}
- Claude Code also reads `skillOverrides` from the shared `.claude/settings.json` and your user `~/.claude/settings.json` (under `CLAUDE_CONFIG_DIR` when set).
- Doctor reports an entry only when no higher file sets the same skill. A user file that turns `deploy` off is not reported when the local file turns it back `on`.
- Entries for skills sync does not write, such as your personal ones, are only counted.
- Skills whose generated files are under `sync.unmanaged` do not count as synced.
- The shared file is team policy. Its line points to the [overlay](@/docs/spec-format/overlays.md) sync writes it from. When the overlay sets the same value, the line is a `~` note instead.
- Below the git root, Claude Code also reads the local file at the root (or the main checkout in a worktree). Its value wins for a key both set. Doctor lists it first, labelled `(git root)` or `(main checkout)`.
- Claude Code and doctor skip the root file on Windows and when the root is your home folder. Claude Code also skips it when you do not own the root or its `.git` or `.claude` entry. Doctor does not check ownership.
- `--scope project` reads only the shared file. An empty settings file counts as no settings. Managed (enterprise) settings and `--settings` are not read.
- Claude Code reads settings as strict JSON, and so does doctor:

| Case | Doctor reports | `--json` |
|------|----------------|----------|
| The file is not strict JSON (a comment or trailing comma), so Claude Code skips it | The file and the parse error | One item with `problem` |
| A value other than `on`, `off`, `user-invocable-only`, or `name-only`, which Claude Code rejects and may skip the whole file for | The value's JSON text. The file's other entries get a caution line and overrule nothing in a lower file. | `rejected: true`, and `uncertain: true` on the other entries |
| Two keys name one skill (`Deploy` and `deploy`) with different values, and Claude Code does not say which applies | Both keys. The file still overrules lower files. | `conflict: true` on both |
| Two keys name one skill with the same value | One key | The other has `effective: false` |

- `doctor --fix` removes both keys of a duplicate. It edits only the project's `.claude/settings.local.json`.
- When a lower file sets the same skill, the question names the value that then applies (or may apply, if that value is rejected).
- It skips a file that is linked, hard-linked, read-only, or owned by another user, one that sets `skillOverrides` twice, and one under [`sync.unmanaged`](@/docs/sync-settings.md#syncunmanaged).
- It replaces the file in one step, so a failed write leaves it as it was, and it leaves alone a file that changed while doctor ran.
- `--json` lists every entry, with `effective` and `synced` as in the [reference](#reference).
{% </details> %}

## Run one check {#doctor-subcommands}

| Subcommand | Check |
|------------|-------|
| `doctor config` | Validate `agnostic-ai.yaml` with the checks every command runs when it loads the project. |
| `doctor install` | Which AI CLIs are on PATH. |
| `doctor mcp` | Find each MCP server's command and list its unset `${NAME}` references. Fails when the config does not load. |
| `doctor rtk --command '<command>'` | Preview an RTK rewrite and compare declared Claude Code approval rules. Add `--json` for JSON. |

### RTK approval diagnostics

`doctor rtk --command 'git status'` calls the installed `rtk --version` and `rtk rewrite`, following [RTK's rewrite protocol](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/hooks/rewrite_cmd.rs). It never runs the command or changes permissions.

| RTK result | Doctor reports |
|------------|----------------|
| RTK missing | `missing` |
| Unsupported or already-prefixed command | `unchanged` |
| Exit 3 with a replacement (RTK asks for approval), or exit 2 without one (a deny rule) | The result, with `processor_exit` in JSON |
| Other RTK failures, unreadable settings | The command fails |

{% <details summary="Details: rules compared, precedence, and limits"> %}
- The report compares simple Bash rules declared in planned project settings (or native project settings when none is planned), project-local settings, and user settings (`CLAUDE_CONFIG_DIR` or `~/.claude`).
- It uses [Claude Code's precedence](https://code.claude.com/docs/en/permissions): deny over ask over allow.
- Exact rules, bare `Bash`, and simple trailing command prefixes are supported.
- Shell wrappers, parameter rules, other shell syntax, and ambiguous patterns report `unknown`.
- A rewrite from a known ask to a known allow warns.
- It lists known RTK `PreToolUse` handlers by settings path and warns when there is more than one. It recognizes direct Bash handlers that use `rtk rewrite` or RTK's Claude processor. Plugins, indirect scripts, and other matchers can go unseen.
- Runtime approval always reports `unknown`. Administrator settings, session options, project trust, enabled hooks, and other handlers can change the outcome on a live host.
- Warnings do not grant approval or fail the command.
{% </details> %}

## Reference

### JSON lists {#json-lists}

`doctor --json` adds these lists to the `sync --check --json` schema.

| `--json` list | Fields |
|------|--------|
| `references` | With `--check-references`: `target`, `source` (omitted when unknown), `path`, `line`, `destination`. |
| `globs` | With `--check-globs`: one item per rule that never loads: `rule`, `globs`, and `invalid` (the pattern doctor cannot read) when that is the reason. |
| `nested_copies` | `path` and `rule` for each nested `CLAUDE.md` a rule already holds. |
| `hook_script_drift` | `script` (the basename) and `tools` for each hook script whose body differs across tools. |
| `lint` | `code`, `severity` (`error` or `warn`), `path`, `message`. |
| `hook_trust` | `path`, `event`, `group`, `handler`, `hook`, `status` (`untrusted`, `modified`, `disabled`, or `unknown`). An `unknown` entry adds `problem` with the failed check. |
| `packaging_ignore` | `path`, and either `uncovered` (generated paths) or `problem` (why coverage could not be checked). |
| `skill_overrides` | With `claude` selected, one item per `skillOverrides` entry, highest file first: `path` (absolute outside the project), `scope` (`local`, `shared`, or `user`), `skill` (the key as written), `value` (the string, or the JSON text of another value), `synced` (false for a skill sync does not write), and `effective` (false when a higher file sets the skill). An unreadable file has one item with `path`, `scope`, and `problem`. |
