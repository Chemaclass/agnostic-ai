+++
title = "Import"
description = "Bring existing tool configuration into a project's source specs."
weight = 11

[extra]
group = "Reference"
+++

# Import

## import

Translate an existing AI CLI configuration into specs in the `sources:` directories. Run it after `init`. It writes nothing else. Preview diffs hide known [credential values](@/docs/cli-reference/sync.md#credential-previews) on both sides.

```bash
agnostic-ai import claude
agnostic-ai import claude codex   # in order; AGNOSTIC_AI.md comes from the last
agnostic-ai import all
agnostic-ai import claude codex --dry-run --diff   # review content and conflicts
```

| Flag | Effect |
|---|---|
| `--dry-run` | List every file the import would write, without contents, from a temporary copy of the project. |
| `--diff` | With `--dry-run`, show each destination as `create`, `change`, or `unchanged`, its sources, and a unified diff. It names each destination where sources disagree and the one a real import keeps (the last). It still exits 0. A protected spec still needs `--overwrite`. |
| `--overwrite` | Replace existing specs with what the tool's files hold. Keys the tool never shows and comments on kept keys stay. `::target` blocks for other tools are lost. |

### Import behavior

`import all` imports each tool it detects from config files and directories. A detected tool with no importer prints `skipping <tool>`. `all` cannot combine with other sources.

- An entry file or skill folder that links outside the project is skipped with a `skipped <path>` note. When you name the tool (`import claude`), import follows an entry file link.
- Unedited sync output is skipped, so `import all` right after `sync` changes nothing.

`import all` (and `init --from all`) reads a root `AGENTS.md` last. It adds each `##` section that `.agnostic-ai/AGNOSTIC_AI.md` lacks and reports `merged <n> sections from AGENTS.md`. With no tool detected:

- A hand-written root `AGENTS.md` becomes `.agnostic-ai/AGNOSTIC_AI.md` (`seeded from AGENTS.md`).
- A generated one is left alone (`no importable AI CLI configs detected`).

`import --global [targets]` reads your user config from the tools `sync --global` writes into `$AGNOSTIC_AI_HOME`: default model, effort, and [voice dictation](@/docs/spec-format/settings.md#voice-dictation) (`settings/imported.yaml`), Claude Code permission rules (`settings/permissions-claude.yaml`), MCP servers (`mcps/<name>.yaml`), and supported [Kiro hook files](@/docs/targets/kiro.md) (`hooks/<name>.yaml`).

- It leaves out anything a home spec provides, `local/` included. Existing specs stay, except for [voice additions](@/docs/spec-format/settings.md#voice-dictation) and new rules appended to `settings/permissions-claude.yaml`.
- Permission rules come only from `permissions.allow`, `ask`, and `deny` in `~/.claude/settings.json`, never managed or project settings. Rules sync recorded there are left out. Rules with credentials or shell syntax it cannot inspect are left out with a warning.
- It skips three kinds of server with a warning that names the server and field: one another tool defined differently (the first tool wins), one it cannot convert, and one with a literal [credential](@/docs/spec-format/mcps.md#what-import-writes). A plain setting such as `NODE_ENV: production` still imports.
- A full `sync --global` writes the imported MCP servers to every tool it manages. After a one-tool import, the printed `sync --global --target <tool> --check` passes while that tool's files match. When the home `targets` list leaves the tool out, import asks you to add it first. After an import from several tools, it points at `sync --global --dry-run`.

### Keep existing specs

- Replacing a spec with content the tool never read stops the import with [AAI-203](@/docs/errors.md#aai-203-import-would-replace-an-existing-spec), which lists the cases and fixes.
- A spec the tool already reads, unchanged since the last sync or that tool's import wrote it, may be replaced. That is how a native edit comes back.
- `.agnostic-ai/AGNOSTIC_AI.md` never stops the import. In `import claude codex` a later source still replaces an earlier one.
- An identical spec keeps its content and modification time. After several sources import one file, only those whose content matches it may re-import without `--overwrite`.
- A sync that keeps edited or unmanaged outputs keeps earlier records for unchanged specs. It does not make changed specs safe to replace.
- A disabled output, a skill asset left out, or settings with no translated fields do not count as read by the tool.

### Bring tool edits back

- Re-importing tool files that still match the last sync keeps the spec exactly as written, including `::target` blocks, `workspaces`, comments, and overlays an older release wrote.
- An edit in a tool's file goes back into the spec. The spec keeps frontmatter keys the tool never shows (such as `workspaces` for Claude) and comments on kept keys, with `--overwrite` too. A spec with `::target` blocks stops with AAI-203 instead.
- A skill or agent spec keeps keys the tool has no place for, such as Cursor's `argument-hint`. Deleting a key the tool does write reaches the spec: removing `model` from a Qoder agent removes it there. Rules are rebuilt from the native file.

### Files and source order

- An absolute source path keeps its own destination. Links at the destination are kept, also in dry-run. A dangling link fails with its path. A conflict or interrupt restores writes made through symlinks and hard links.
- The search for nested config skips git-ignored directories, directories with their own `.git`, and `node_modules/`.
- Each source copies its top-level instructions file to `.agnostic-ai/AGNOSTIC_AI.md`, so the last argument wins. A fenced `AGNOSTIC_AI.md` stays when the entry point matches what sync would write from it. Otherwise import overwrites it and warns.
- If another entry point holds different hand-written content (an `AGENTS.md` next to `CLAUDE.md`), import warns that `sync` would overwrite it. Merge it into `.agnostic-ai/AGNOSTIC_AI.md` first. `import all` merges a root `AGENTS.md` itself.
- Valid sources: `claude`, `codex`, `cursor`, `aider`, `amp`, `warp`, `gemini`, `copilot`, `opencode`, `zed`, `antigravity`, `continue`, `cline`, `windsurf`, `junie`, `trae`, `kiro`, `crush`, `qoder`, `kilo`, `goose`, `factory`, `openhands`, `augment`, `ellipsis`, plus `all`. `jules` supports sync but not import.

Each [target page](@/docs/targets/_index.md) lists what `import <target>` reads. See [Claude import](@/docs/targets/claude-reference.md#import) for what `import claude` leaves in place.

### Filename prefix reclassification {#filename-prefix-reclassification}

`import cline`, `import windsurf`, `import trae`, `import qoder`, and `import continue` reclassify each file in their rules directory by name:

| Target | Rules directory |
|--------|-----------------|
| `cline` | `.clinerules/`, with `.cline/rules/` fallback |
| `windsurf` | `.devin/rules/`, with `.windsurf/rules/` fallback |
| `trae` | `.trae/rules/` |
| `qoder` | `.qoder/rules/` |
| `continue` | `.continue/rules/` |

| Source filename | Becomes |
|-----------------|---------|
| `agent-<name>.md` | `<agents>/<name>.md` |
| `skill-<name>.md` | `<skills>/<name>.md` |
| `<name>.md` | `<rules>/<name>.md` |
| `<scope>/<file>.md` | nested under the same `<scope>` in the destination |

Import strips the leading `# <heading>` block the adapter adds and adds a `name:` field in frontmatter.

### import reconcile

Plan how to bring later edits from a tool's skill folder into the source specs after an import:

```bash
agnostic-ai import reconcile \
  --base before-migration \
  --migrated migration-commit \
  --upstream other-branch \
  --map .cursor/skills=.agnostic-ai/skills
```

| Flag | Commit or path |
|------|----------------|
| `--base` | Holds the original tool skill folder. An ancestor of `--upstream` and `HEAD`. |
| `--migrated` | Holds the first complete source specs, so import's own changes are the baseline. An ancestor of `HEAD`, which supplies the current specs. |
| `--upstream` | The branch with later edits to the tool skill folder. |
| `--map <native>=<source>` | A native skill directory and `sources.skills`, separate and relative to the project directory, even when nested in a larger repository. Repeat per native directory. |
| `--json` | Print the four resolved commit IDs and an `entries` array of `action`, `source`, and `destination`. |

Each row reports `add`, `remove`, `update`, or `conflict`, followed by the tool and source paths. Assets and executable modes are included. Matching files are omitted. A conflict is one of:

- a concurrent source edit, deletion, or addition at an occupied destination
- a proposed file clashing with a surviving file above or below it (planned removals can allow a file-to-directory swap)
- a change to a destination several mapped sources own

Conflicts do not change the exit status.

- It compares file contents stored in Git without merging text.
- Only direct skill folders with `SKILL.md` and regular files are supported. Symlinks and special files stop planning.
- Every base file needs a source counterpart at `--migrated`.
- Commit changes in mapped trees first. Other uncommitted changes are fine.
- It only reports a plan for you to apply. It copies no files, restores no skills, and leaves the index alone.
