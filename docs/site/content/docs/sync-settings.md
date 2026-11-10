+++
title = "Sync and gitignore settings"
description = "The sync and gitignore keys of agnostic-ai.yaml: collisions, shared skills, unmanaged paths, and the managed .gitignore block."
weight = 132

[extra]
group = "Reference"
+++

# Sync and gitignore settings

These two keys of `agnostic-ai.yaml` control how `sync` writes generated files and what it adds to `.gitignore`. Every other key is in [Configuration](@/docs/configuration.md).

## `sync`

Per-run flags such as `--diff`, `--format`, and `--jobs` have no config key. See [`sync`](@/docs/cli-reference/sync.md#sync) and [reading a failing `--check`](@/docs/cli-reference/sync.md#reading-a-failing---check).

| Key | Default | Effect |
|-----|---------|--------|
| [`collision-policy`](#synccollision-policy) | `prompt` | What happens when two targets write the same path. |
| [`target-overview`](#synctarget-overview) | `false` | Add a section to each entry-point file that lists where generated files live. |
| [`resolve-imports`](#syncresolve-imports) | `passthrough` | What tools that cannot resolve `@path` lines get. |
| [`dropped-summary`](#syncdropped-summary) | `false` | Print a per-target summary of dropped and downgraded kinds. |
| [`shared-skills`](#syncshared-skills) | `false` | Link identical skill folders to one copy. |
| [`unmanaged`](#syncunmanaged) | empty | Paths sync never touches. |
| [`output-manifest`](#syncoutput-manifest) | `false` | Write `.agnostic-ai/outputs.lock`, the committed list of generated paths. |
| [`allow-global-names`](#syncallow-global-names) | empty | Names shared with the global home that `sync` does not warn about. |
| [`global-name-clash`](#syncglobal-name-clash) | `warn` | Whether `sync` warns about names shared with the global home. |

### `sync.collision-policy` {#synccollision-policy}

Applies when two targets write different content to one path, such as `outputs.codex.rules-file: AGENTS.md` and `outputs.amp.rules-file: AGENTS.md`. Per-target override: `outputs.<target>.collision-policy`.

| Value | Behavior |
|-------|----------|
| `prompt` | Default. Fail with an `output collision` error and a hint. |
| `prefer-spec` | Write the version of the last tool in `targets:` that writes the file. `sync --check` expects that version. Sync prints one note per winning tool naming the files it won and the tools it overrode; reorder `targets:` to change it. Use in CI when the overlap is intentional. |
| `fail` | Hard error with no hint. |

```yaml
sync:
  collision-policy: prefer-spec
```

### `sync.target-overview` {#synctarget-overview}

When `true`, each entry-point file (`CLAUDE.md`, `AGENTS.md`, `GEMINI.md`, ...) gets a section listing where that tool's generated files live, following `outputs.<target>.*` overrides.

- A shared entry point such as `AGENTS.md` lists each reader in its own section.
- Sync rewrites the section between `<!-- agnostic-ai:target-overview:start -->` and `<!-- agnostic-ai:target-overview:end -->`. `import` removes it. Aider and external adapters get no section.

### `sync.resolve-imports` {#syncresolve-imports}

Controls standalone `@path` lines in `AGNOSTIC_AI.md` for tools that cannot resolve imports. `CLAUDE.md` always keeps the line. An `@mention` inside a sentence or a fenced code block is untouched. Paths resolve from the project root.

| Value | Tools that cannot resolve it get |
|-------|---------------------------|
| `passthrough` | Default. The `@`-line as written, as a dead reference. |
| `strip` | Nothing: the line is dropped. |
| `inline` | The file's content between `<!-- agnostic-ai:import:start <path> -->` and `<!-- agnostic-ai:import:end -->`. `import` restores the `@`-line. A missing file fails the sync. |

```yaml
sync:
  resolve-imports: inline
```

### `sync.dropped-summary` {#syncdropped-summary}

When `true`, sync ends with a per-target list of kinds that were dropped (the tool has no support) or downgraded (written only behind an opt-in key, or only in the source directory). It groups capability warnings and [coverage notes](@/docs/configuration.md#coverage-notes) by tool.

### `sync.shared-skills` {#syncshared-skills}

When `true`, tools that share the Agent Skills layout (`<dir>/<name>/SKILL.md` plus assets: Claude, Cursor, Codex, Amp) keep one real copy of each skill and link the others to it with relative symlinks.

- Only folders with identical content link. Skills with per-tool overrides such as `x-cursor` keep real copies.
- Turning the option off restores real copies on the next sync.
- Without symlink support (Windows without the privilege), sync warns once and keeps real copies.

If you commit shared skill links, Git must check them out as links. Git's `core.symlinks` setting is separate from the system's permission to create links. With `core.symlinks=false`, `sync --check --against index` and `--against HEAD` name each link Git exported as a plain file. To fix it:

- Where the system can create links, run `git config --local core.symlinks true` and rerun the check.
- To keep `core.symlinks=false`, set `sync.shared-skills: false`, run `agnostic-ai sync`, and stage the config and the restored skill copies. Commit that before you check `--against HEAD`.

### `sync.unmanaged` {#syncunmanaged}

Paths you own. `sync` never writes, merges, copies, or removes them.

```yaml
sync:
  unmanaged:
    - .cursor/rules/legacy.mdc        # exact path
    - .claude/agents/hand-*.md        # glob; `*` stays inside one path segment
    - .claude/skills/legacy/          # trailing slash: everything under the directory
```

- Entries are project-relative with forward slashes. Filename patterns use Go `path.Match`; `**` acts like `*` and stays inside one folder name, so `**/x` misses the root and deeper folders and `dir/**` matches only files directly inside `dir/` (write `dir/`). Lint, doctor, and sync warn about it (LINT057). A malformed glob or an empty entry fails config load.
- `sync` prints `~ skip (unmanaged) <path>`. `sync --check`, `status`, and `doctor` never count it as drift. `revert` and `doctor --fix` never touch it.
- Removing the entry deletes nothing. The next sync rewrites the file.
- `sync.shared-skills` never links such a skill folder.
- `agnostic-ai.local.yaml` replaces the entire list.
- Hook script bodies copied from `.agnostic-ai/scripts/` are not covered.

### `sync.output-manifest` {#syncoutput-manifest}

Use it when you commit generated files. A full `sync` writes `.agnostic-ai/outputs.lock`: one line per generated path, with a checksum of what sync wrote. Commit it with the outputs; `sync --check` fails when it is missing or out of date.

```yaml
sync:
  output-manifest: true
```

- With no `.sync-state`, `doctor` and `sync --check` count a tracked file the generated-path list includes but no spec produces as a leftover. `doctor --fix` removes it.
- A file edited since sync wrote it no longer matches its checksum, so it is left for you.
- `sync --check --against` reads the generated-path list from the state it checks. It catches a deleted spec's outputs even in a shallow clone, unless that list was regenerated in the same commit.

### `sync.allow-global-names` {#syncallow-global-names}

Skill and agent names this project shares with the [global home](@/docs/global-configuration.md#global-shared-names) on purpose. `sync` prints no warning for a listed name. Other shared names still warn.

```yaml
# agnostic-ai.local.yaml
sync:
  allow-global-names: [gh-issue, gh-issues]
```

- Put the list in `agnostic-ai.local.yaml`, since the home exists only on your machine. A local list replaces the entire shared list.
- Names match the way Claude Code folds skill names: case, spacing, and invisible characters do not count.
- `doctor` still lists each allowed name under **Global names**, marked as allowed.

### `sync.global-name-clash` {#syncglobal-name-clash}

What `sync` does with every name this project shares with the [global home](@/docs/global-configuration.md#global-shared-names): `warn` prints the warning, `ignore` prints nothing.

```yaml
# agnostic-ai.local.yaml
sync:
  global-name-clash: ignore
```

- Use it when a project shadows global skills or agents as a rule. To accept a few names, use [`sync.allow-global-names`](#syncallow-global-names).
- `doctor` still lists each shared name under **Global names**, marked as ignored.
- Any other value fails config load.

## `gitignore`

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` when absent; `agnostic-ai init` writes `true` | Every `sync` rewrites a managed `.gitignore` block listing every path the configured adapters write. |
| `path` | `.gitignore` | Another file inside the project, such as a nested folder's `.gitignore` or the local-only `.git/info/exclude`. Any other path outside the project or in `.git/` fails to load. |
| `commit` | empty | Kinds of generated output to keep in Git, for every target or as `<target>:<kind>` for one. The block leaves out their paths. |
| `allow` | empty | Gitignore globs written as `!` lines at the end of the block, so a hand-written file at a generated path (such as a `testdata/AGENTS.md` fixture) is not ignored. One pattern per entry; a line break fails to load. |
| `worktree-include` | `true` | Keep the same block in `.worktreeinclude`. See [Claude Code worktrees](#claude-code-worktrees). |
| `ignore-worktree-include` | `false` | Keep `.worktreeinclude` out of Git. See [Claude Code worktrees](#claude-code-worktrees). |

`sync --gitignore` and `init --gitignore` override it per run. See the [CLI reference](@/docs/cli-reference/start.md#init).

Use `commit` for files that review bots and a fresh worktree read before any sync runs:

```yaml
gitignore:
  enabled: true
  commit: [instructions, hooks]
```

- Kinds: `instructions`, `agents`, `skills`, `commands`, `hooks`, `mcps`, `settings`, `reviews`, `environments`, and `ignores`. An unknown kind fails config loading.
- `instructions` covers entry-point files (`AGENTS.md`, `CLAUDE.md`, `GEMINI.md`) and rule outputs, root and scoped. Every other kind is named after its spec source.
- A file written by several kinds, such as `.claude/settings.json`, is committed when any of them is listed.
- `ellipsis:reviews`, `ellipsis:environments`, and `ellipsis:mcps` also count the settings spec that holds `x-ellipsis`, because `.ellipsis/code-review.yaml` and `.ellipsis/environments/<name>.yaml` need both ([Ellipsis](@/docs/targets/ellipsis.md#keep-the-files-in-git)).
- A file that only the config or an [overlay](@/docs/configuration.md#watched-inputs) produces belongs to no kind and stays ignored.

Prefix a kind with a target to commit it for that target only:

```yaml
gitignore:
  enabled: true
  commit: [cursor:reviews, cursor:environments]
```

This keeps `.cursor/environment.json`, `.cursor/worktrees.json`, and every `BUGBOT.md` in Git, and ignores `.claude/launch.json` and `.codex/environments/environment.toml`. `lint` warns (LINT017) when the target is not in `targets`.

The block sits between `# >>> agnostic-ai (managed) >>>` and `# <<< agnostic-ai (managed) <<<`. Lines outside it are kept.

### Claude Code worktrees

A new Claude Code worktree has no gitignored files. Claude Code copies the ones that [`.worktreeinclude`](https://code.claude.com/docs/en/worktrees#copy-gitignored-files-into-worktrees) lists from the main checkout.

With `claude` in `targets`, sync also keeps the block in `.worktreeinclude`, minus `.sync-state`, `.command-lock`, Claude worktree directories, and the task lock. Your local files and packs come along, so the worktree renders what the main checkout does.

- Set `gitignore.ignore-worktree-include: true` to keep the managed file out of Git by listing it in the managed ignore block. It requires `worktree-include: true` and `claude` in `targets`. If the file is already tracked, run `agnostic-ai sync --untrack` to remove it from the index and keep the file.
- Set `gitignore.worktree-include: false` to manage the file yourself. Sync then removes only its own block.

A fresh clone or `git worktree` lacks these paths until `sync` runs. See [checkout and merge hooks](@/docs/git-hooks.md#regenerate-on-checkout).

### Block contents

- Entries are root-anchored (`/AGENTS.md`), so nested same-named files are not ignored.
- Files collapse to their generated subdirectory (`/.claude/rules/`), never higher, so siblings such as `.claude/settings.json` stay visible. A per-kind dir such as `outputs.<target>.rules-dir` collapses at the dir itself.
- Output under a rule or review `scope` stays one line per file (`/services/api/AGENTS.md`).
- The block always holds `agnostic-ai.local.yaml`, `/.agnostic-ai/.command-lock`, `/.agnostic-ai/.sync-state`, `/.agnostic-ai/packs/`, and `/.agnostic-ai/local/`. `init` seeds them even with `gitignore.enabled: false`.
- A leftover file from a deleted spec stays ignored until you remove it.
- A target can add entries of its own, such as [Claude Code](@/docs/targets/claude.md)'s local settings.
- Built-ins come from `agnostic-ai.yaml` alone, so a `builtins` list in `agnostic-ai.local.yaml` leaves the block and `.worktreeinclude` unchanged. Outputs only the local list adds go in an `agnostic-ai (personal)` block in `.git/info/exclude`, and sync removes those lines when the local list stops adding them. Outputs the local list turns off stay in the block. A linked worktree shares `.git/info/exclude` with every checkout, so there sync leaves those outputs untracked and names them in a warning.
