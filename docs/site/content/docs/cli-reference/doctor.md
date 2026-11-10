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

Read the findings and the final **Next step** before a repair or a focused check. The [troubleshooting guide](@/docs/troubleshooting.md) covers symptoms.

Doctor reports four kinds of generated file:

- **missing**: never synced
- **stale**: out of date with the specs
- **edited**: changed since the last sync
- **orphaned**: no longer generated, kept because sync cannot prove it wrote the file

It exits non-zero on any drift, [lint](@/docs/cli-reference/check.md#lint) error, or untrusted, modified, or unreadable Codex hook. Hooks you disabled on purpose do not fail. It also names any [spec migration](@/docs/cli-reference/upgrades.md#migrate) that applies.

| Flag | Description |
|------|-------------|
| `-t, --target <list>` | Comma-separated targets (default: all in config). An unknown name fails with AAI-301, as in `sync`. |
| `--scope <all\|project>` | `all` (default) includes local Codex hook trust and your own Claude settings files. `project` skips them and keeps all project checks. |
| `--fix` | Write missing files and overwrite stale or edited generated files. Remove each nested `CLAUDE.md` whose text is already in a scoped rule. In a terminal, ask before removing each kept orphan and each group of [Claude skill overrides](#claude-skill-overrides), with No as the default. Without a terminal, keep them and explain why. Exit non-zero while any orphan remains. |
| `--backup` | With `--fix`, copy each file to `<path>.bak` before overwriting it, removing a confirmed orphan, or removing confirmed skill overrides. |
| `--check-globs` | Flag rules whose `globs:` match no files. Off by default. |
| `--check-references` | Flag relative Markdown links in generated skills whose file is missing. Off by default. |
| `--json` | Drift report as JSON, same schema as `sync --check --json`, plus the lists below. It fails on the same findings as text mode. It cannot be combined with `--fix`. |

On a fresh CI runner, run `agnostic-ai doctor --scope project --check-references`. Codex hook trust then shows `SKIPPED` in text, and JSON adds `hook_trust_check: {"status": "skipped", "reason": "project scope skips local runtime trust"}` with an empty `hook_trust`. `--scope project --fix` repairs drift without reading or approving trust. `--json` stays read-only.

`--check-references` reads each Markdown file a selected tool writes for its skills.

- A link is valid when it resolves from the document's directory or from the project root.
- It skips code, URLs, absolute paths, and `#fragment`-only links.
- In a `file#fragment` link, it checks only the file.
- [`doctor.check-references.ignore`](@/docs/configuration.md#doctorcheck-referencesignore) exempts destinations that can never resolve.
- Any broken link fails doctor.

Findings group by source spec and link:

```
Skill references:
  ✗ .agnostic-ai/skills/deploy/SKILL.md:8 links to missing references/setup.md
      targets: claude, codex
```

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

When sync removes unchanged managed keys from a shared settings file but keeps your keys and edits, doctor reports the file as out of date. JSON reports its action as `stale`.

After the file report, doctor prints:

| Block | What it shows | Counts as drift |
|-------|---------------|-----------|
| **Spec health** | The findings `agnostic-ai lint` reports. An error fails doctor. A warning does not. | No |
| **Coverage notes** | With [`coverage.accept`](@/docs/configuration.md#coverageaccept) set, how many notes it accepts (`coverage_accepted` in `--json`). `sync -vv` lists them. | No |
| **Packaging ignores** | Generated paths that an existing root `.npmignore`, `.vscodeignore`, or `.dockerignore` does not cover, with the ignore file. | No, reported only |
| **Tracked despite ignored** | A generated path that git tracks and ignores, with the `git rm --cached` fix. | No |
| **Codex hook trust** | With Codex selected: inactive handlers in the project hooks file and user `hooks.json`, with `/hooks` as the next step. Reads trust from `CODEX_HOME/config.toml` (default `~/.codex/config.toml`). Untrusted, modified, or unreadable status fails. Disabled does not. | Never grants trust |
| **MCP** | Whether each stdio `command:` is on PATH, with install hints. Then each `${NAME}` a server reads in `env`, `headers`, `url`, or `args` that is unset in this shell, by name only. A reference with a default is skipped. | No |
| **Claude hook specs** | With `claude` enabled: a hook spec with several commands that `.claude/settings.json` already runs in one group with different settings, as an older `import claude` wrote it. Each command then runs twice. To fix it, delete the spec, run `sync`, then run `import claude`. | No |
| **Nested CLAUDE.md** | With `claude` enabled: a hand-written `<dir>/CLAUDE.md` whose text equals the body of a rule scoped to `<dir>`, as `import claude` leaves it. Claude Code would load it beside the synced rule. | Yes, `--fix` removes it |
| **Script divergence** | Scripts under `.agnostic-ai/scripts/<tool>/` with the same name but different bodies across tools, with the suggested path `.agnostic-ai/scripts/<basename>`. | Yes, no automatic fix |
| **Unmanaged config** | Markdown and TOML config files with no generated-file marker, grouped by the `import` source that adopts each. | No |
| **User-owned** | [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged) entries, left out of Unmanaged config. | Never |
| **Global names** | A project skill or agent that shares its name with one in `~/.agnostic-ai/`, and which wins per tool. A name in [`sync.allow-global-names`](@/docs/configuration.md#syncallow-global-names) shows as allowed. See [shared names](@/docs/global-configuration.md#global-shared-names). | Never |
| **Instructions** | A hint when `AGNOSTIC_AI.md` still holds the long default text. | Never |
| **Claude skill overrides** | With `claude` selected: synced skills that `skillOverrides` in a Claude settings file turns off, hides from Claude, or lists without a description. See [below](#claude-skill-overrides). | Never |

<details>
<summary>What the packaging check covers</summary>

The check handles literal paths, `*`, `?`, character classes, and `**` segments. Braces, extended globs, escaped patterns, and unsupported classes give a coverage-check warning, not an uncovered claim.

It reads only the root ignore file. It ignores `package.json` allowlists, built-in package exclusions, nested ignore files, and Dockerfile-specific ignore files. Verify with `npm pack --dry-run`, `vsce ls`, or your Docker build context.

</details>

### Claude skill overrides

Claude Code's `/skills` menu saves each skill you switch off in your git-ignored `.claude/settings.local.json`. The skill still syncs, but Claude Code refuses to run it. Claude Code also reads `skillOverrides` from the shared `.claude/settings.json` and your user `~/.claude/settings.json` (under `CLAUDE_CONFIG_DIR` when set). Doctor lists the synced skills each file turns off, hides, or lists without a description, highest file first: local, shared, user.

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

- Doctor reports an entry only when no higher file sets the same skill. A user file that turns `deploy` off is not reported when the local file turns it back `on`.
- Entries for skills sync does not write, such as your personal ones, are only counted.
- Skills whose generated files are under `sync.unmanaged` do not count as synced.
- `--json` lists every entry, with `effective` and `synced` as above.

The shared file is team policy. Its line points to the [overlay](@/docs/spec-format/overlays.md) sync writes it from. When the overlay sets the same value, the line is a `~` note instead.

Below the git root, Claude Code also reads the local file at the root (or the main checkout in a worktree).

- Its value wins for a key both set.
- Doctor lists it first, labelled `(git root)` or `(main checkout)`.
- Claude Code and doctor skip the root file on Windows and when the root is your home folder.
- Claude Code also skips it when you do not own the root or its `.git` or `.claude` entry. Doctor does not check ownership.

`--scope project` reads only the shared file. An empty settings file counts as no settings. Managed (enterprise) settings and `--settings` are not read.

Claude Code reads settings as strict JSON, and so does doctor:

| Case | Doctor reports | `--json` |
|------|----------------|----------|
| The file is not strict JSON (a comment or trailing comma), so Claude Code skips it | The file and the parse error | One item with `problem` |
| A value other than `on`, `off`, `user-invocable-only`, or `name-only`, which Claude Code rejects and may skip the whole file for | The value's JSON text. The file's other entries get a caution line and overrule nothing in a lower file. | `rejected: true`, and `uncertain: true` on the other entries |
| Two keys name one skill (`Deploy` and `deploy`) with different values, and Claude Code does not say which applies | Both keys. The file still overrules lower files. | `conflict: true` on both |
| Two keys name one skill with the same value | One key | The other has `effective: false` |

`doctor --fix` in a terminal asks once per state whether to re-enable those skills.

- A yes removes only their entries (both keys of a duplicate) from the project's `.claude/settings.local.json`, the only file it edits.
- When a lower file sets the same skill, the question names the value that then applies (or may apply, if that value is rejected).
- It skips a file that is linked, hard-linked, read-only, or owned by another user, one that sets `skillOverrides` twice, and one under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged).
- It replaces the file in one step, so a failed write leaves it as it was.
- It leaves alone a file that changed while doctor ran.

Subcommands run one check:

| Subcommand | Check |
|------------|-------|
| `doctor config` | Validate `agnostic-ai.yaml` with the checks every command runs when it loads the project. |
| `doctor install` | Which AI CLIs are on PATH. |
| `doctor mcp` | Find each MCP server's command and list its unset `${NAME}` references. Fails when the config does not load. |
| `doctor rtk --command '<command>'` | Preview an RTK rewrite and compare declared Claude Code approval rules. Add `--json` for JSON. |

### RTK approval diagnostics

`doctor rtk --command 'git status'` calls the installed `rtk --version` and `rtk rewrite`, following [RTK's rewrite protocol](https://github.com/rtk-ai/rtk/blob/v0.51.0/src/hooks/rewrite_cmd.rs). It never runs the command or changes permissions.

- Missing RTK reports `missing`.
- An unsupported or already-prefixed command reports `unchanged`.
- RTK's exit 3 with a replacement (it requests approval) and exit 2 without one (it found a deny rule) are reported as results, with `processor_exit` in JSON.
- Other RTK failures and unreadable settings fail the command.

The report compares simple Bash rules declared in:

- planned project settings (or native project settings when none is planned)
- project-local settings
- user settings (`CLAUDE_CONFIG_DIR` or `~/.claude`)

It uses [Claude Code's precedence](https://code.claude.com/docs/en/permissions): deny over ask over allow.

- Exact rules, bare `Bash`, and simple trailing command prefixes are supported.
- Shell wrappers, parameter rules, other shell syntax, and ambiguous patterns report `unknown`.
- A rewrite from a known ask to a known allow warns.

It lists known RTK `PreToolUse` handlers by settings path and warns when there is more than one. It recognizes direct Bash handlers that use `rtk rewrite` or RTK's Claude processor. Plugins, indirect scripts, and other matchers can go unseen.

Runtime approval always reports `unknown`. Administrator settings, session options, project trust, enabled hooks, and other handlers can change the outcome on a live host. Warnings do not grant approval or fail the command.
