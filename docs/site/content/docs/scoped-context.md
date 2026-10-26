+++
title = "Directory-specific instructions"
description = "Keep rules close to the directories where they apply across supported tools."
weight = 40

[extra]
nav_title = "Folder instructions"
group = "Workflows"
+++

# Directory-specific instructions

Use a scoped rule for instructions that apply to one directory, such as a service's conventions. Write the rule once and sync writes each supported tool's files, which keeps that text out of the root instructions every session loads.

## Start with one directory

Run commands from the project root. For a new scratch project:

```bash
echo "claude,codex" | agnostic-ai init
agnostic-ai new rule payments-context --scope services/payments
```

In an existing project, skip `init` and check [tool compatibility](#shared-files-and-safe-updates). If `new rule --help` does not list `--scope`, [upgrade agnostic-ai](@/docs/installation.md#upgrade).

Edit `.agnostic-ai/rules/payments-context.md`:

```markdown
---
name: payments-context
scope: services/payments
---

Use integer minor units for monetary values.
```

Generate and check:

```bash
agnostic-ai sync
agnostic-ai sync --check
agnostic-ai graph --spec payments-context
```

Claude gets a rule for that directory. Codex gets `services/payments/AGENTS.md`; start Codex inside `services/payments` so it loads the nested file. See [native support](#native-support) for other tools.

To change the rule, edit the source and sync again. Preview with `agnostic-ai render .agnostic-ai/rules/payments-context.md`, or trace a generated file with `agnostic-ai why services/payments/AGENTS.md`.

## Check what applies to a file

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

The report lists each instruction for one tool (Claude Code, Codex, or Cursor), with its source spec, output path, condition, and status. It reads the output sync plans to write, so it shows what is set to apply, not what the model loaded. A shared `services/payments/AGENTS.md` replaces a `.mdc` rule when another tool needs it. See the [CLI reference](@/docs/cli-reference/inspect.md#explain-a-source-file) for the status values.

<details>
<summary>Set directory scope and understand Codex directory patterns</summary>

## Scope rules

- `scope` is a project-relative directory and everything under it, with `/` separators. Absolute paths, `..`, glob characters, symlinks that leave the project, and `scope: .` are rejected. Omit `scope` for project-wide rules.
- A folder under `rules/` that names a project directory scopes the rules in it: `.agnostic-ai/rules/services/payments/limits.md` has scope `services/payments`.
- A `scope:` in the frontmatter wins over the folder. A folder that names no project directory only groups rules, so `rules/modules/a.md` with `scope: src/a` is scoped to `src/a`.
- In installed packs, a rule folder sets a scope only when it names a directory in your project, not one inside the pack.
- `lint` warns when a rule's folder names a project directory and its `scope:` points elsewhere (LINT020).
- Keep rule names unique across directories.
- Deeper rules add local context. The tool decides whether parent instructions load and which wins.
- `alwaysApply: true` adds no files to the combined scope and patterns. Sync sets the tool's own condition flags.
- Scope does not remove instructions already loaded into a conversation.

## Codex rules with globs alone

Globs are filename patterns such as `src/api/**`. With Codex enabled, an unscoped rule with `globs: [src/app/api/**, prisma/**]` writes `src/app/api/AGENTS.md` and `prisma/AGENTS.md` instead of adding to the root instructions. Other tools that read `AGENTS.md` share those files. `alwaysApply: true` keeps an unscoped rule project-wide. Set `outputs.codex.nested-glob-rules: false` to keep all unscoped rules inline.

Every pattern must cover a whole directory tree; sync never widens a file filter to a whole directory. Patterns such as `src/api/**/*.ts` or `Dockerfile`, or a mix of root files and directory patterns, keep the whole rule inline in the root file, with a note naming the always-loaded rule. The rule also stays inline, with a note, for output overrides, unmanaged destinations, and tools that read the root file without verified nested-file support, whether configured or chosen with `--target`. `on-unsupported: error` refuses this fallback. An inlined rule with `globs` begins with an `Applies to:` line, such as ``Applies to: `resources/js/**` ``, so it does not read as a rule for the whole repository.

Starting Codex at the root does not load nested files for later edits.

## Qwen Code rules with globs alone

An unscoped rule with `globs` or `paths` writes `.qwen/rules/<name>.md` with the same patterns as a `paths:` list. Qwen Code leaves it out of the prompt until a tool reads or edits a matching file, and loads project rules only in a trusted folder. `alwaysApply: true` keeps the rule in `QWEN.md`. A legacy `outputs.qwen.rules-file` or a moved `outputs.qwen.file` keeps every rule in that file. See [Qwen Code rules](https://qwenlm.github.io/qwen-code-docs/en/users/features/rules/).

</details>

<details>
<summary>Combine directory scope with file patterns</summary>

## Narrow a rule to certain files

`scope` plus `globs` or `paths` applies to the whole scope directory **and** every matching file. Use both for a module and its tests:

```yaml
scope: services/payments
globs: "tests/payments/**"
```

Claude writes both `services/payments/**` and `tests/payments/**` in `paths`. Codex writes `AGENTS.md` in both directories.

Patterns are relative to the project, not the scope, including `**/*.go`. A pattern inside the scope adds no files. To apply a rule only to certain files, omit `scope` and keep the rule outside a folder that implies scope.

`paths` and `globs` work the same way. Unsupported combinations warn and skip; set `on-unsupported: error` to fail instead, or `silent` to suppress notices.

**Breaking change:** earlier versions applied a rule only where scope and patterns overlapped. Remove `scope` from rules that used it to narrow `globs`, and write the full project-relative filters instead. Remove a catch-all `globs: "**/*"` from a rule that should apply only to its scope. See [migration](@/docs/migration.md#scope-and-pattern-unions).

</details>
<details>
<summary>Selector limits by target</summary>

- Tools that read a file per directory accept whole-directory patterns such as `tests/payments/**`. They reject file filters such as `tests/payments/**/*.go` instead of applying them to every file in that directory. Root patterns such as `CHANGELOG.md` and `**/*` are also unsupported with scope on those tools. Tools with file filters keep those patterns.
- The tool's own keys `regex`, `applyTo`, `fileMatchPattern`, and `glob` cannot be combined with `scope`.
- Cline's empty `paths` array keeps the rule disabled. Continue's empty `globs` array is unsupported with scope.
- Windsurf, Trae, and Antigravity write the rule in the project's rules directory when a pattern reaches outside the scope.
- Imported file patterns without an explicit `scope` keep their exact behavior; their folder adds no files. A tool with no equivalent skips that rule under `on-unsupported`.
- Tools that take a list keep literal commas inside a pattern. Tools that take one comma-separated string reject those commas. Commas inside brace patterns such as `tests/{a,b}/**` work everywhere.

</details>


<details>
<summary>Check scoped-rule support and runtime limits by tool</summary>

## Native support

Mappings were checked against vendor documentation on 2026-09-09. Tests verify the generated files, not how each live product behaves.

| Target | Scoped file or condition | Vendor reference |
|---|---|---|
| Claude | `.claude/rules/<scope>/<name>.md`, `paths` | [Memory](https://code.claude.com/docs/en/memory) |
| Mistral Vibe | `<scope>/AGENTS.md` | [Project context](https://docs.mistral.ai/vibe/code/cli/configuration-reference) |
| Codex | `<scope>/AGENTS.md` | [Instructions](https://developers.openai.com/codex/guides/agents-md) |
| Gemini | `<scope>/GEMINI.md` | [Context](https://geminicli.com/docs/cli/gemini-md/) |
| Cursor | `.cursor/rules/<scope>/<name>.mdc`, conditional `globs`; shared nested `AGENTS.md` when compatible peers use it | [Rules](https://cursor.com/docs/context/rules) |
| Copilot | `.github/instructions/<name>.instructions.md`, `applyTo` | [Host support](https://docs.github.com/en/copilot/reference/custom-instructions-support) |
| Cline | `.clinerules/<scope>/<name>.md`, `paths` | [Rules](https://docs.cline.bot/customization/cline-rules) |
| Windsurf / Devin | `<scope>/.devin/rules/<name>.md`, glob trigger | [Rules](https://docs.devin.ai/cli/extensibility/rules) |
| Antigravity | `<scope>/.agents/rules/<name>.md`, glob trigger | [Rules](https://antigravity.google/docs/rules) |
| Continue | `.continue/rules/<scope>/<name>.md`, `globs` without `alwaysApply` | [Rules](https://github.com/continuedev/continue/blob/main/docs/customize/deep-dives/rules.mdx) |
| Amp | `<scope>/AGENTS.md` | [Instructions](https://ampcode.com/docs/customize/agents-md) |
| Warp | `<scope>/AGENTS.md` | [Rules](https://docs.warp.dev/agents/capabilities/rules/) |
| OpenCode | `<scope>/AGENTS.md` | [Rules](https://opencode.ai/docs/rules/) |
| Kiro | `.kiro/steering/<name>.md`, `fileMatchPattern` | [Steering](https://kiro.dev/docs/steering/) |
| Trae | `<scope>/.trae/rules/<name>.md`, conditional `globs` | [Rules](https://docs.trae.ai/ide/rules) |
| Goose | `<scope>/AGENTS.md`, or nested `.goosehints` with its legacy opt-in | [Context files](https://github.com/aaif-goose/goose/blob/main/documentation/docs/guides/context-engineering/using-goosehints.md) |
| Augment | `<scope>/AGENTS.md` | [Rules](https://docs.augmentcode.com/cli/rules) |
| Qoder | `.qoder/rules/<scope>/<name>.md`, `paths` | [CLI memory](https://docs.qoder.com/cli/memory) |
| OpenHands | `.agents/skills/<name>/SKILL.md`, `paths` | [Path rules](https://docs.openhands.dev/overview/skills/path) |
| Factory | `<scope>/AGENTS.md` | [Instructions](https://docs.factory.com/harness/agents-md) |
| Kilo | `<scope>/AGENTS.md`, without unconditional `instructions` entries | [Instructions](https://kilo.ai/docs/customize/agents-md) |

Aider, Zed, Junie, Crush, and Jules have no verified automatic directory scope, so they skip scoped rules. Root rules still work.

Runtime limits:

- Codex and OpenCode load instructions from the directory they start in and its parents. Warp loads the root and current directory, and may find files in other directories. Gemini loads context as files are accessed.
- Copilot support varies by host. OpenHands path rules work in local conversations, not ACP.
- Qoder Desktop and Kiro custom-agent loading are not yet checked at runtime.

</details>

## Shared files and safe updates

Tools that read the same generated file must agree on its text and loading conditions. Sync refuses incompatible combinations; neither a partial sync (`--only`) nor `prefer-spec` bypasses the check. Use compatible tools or separate worktrees.

<details>
<summary>Resolve shared files, output overrides, and old scoped outputs</summary>

- Kiro loads nested `AGENTS.md` files everywhere, so it conflicts with tools that write them.
- Crush reads `.cursor/rules` without applying its conditions, so it conflicts with scoped Cursor rules.
- Tools that share a nested `AGENTS.md` need the same scoped rules and the same text.

Keep headers enabled. Tools that read a file per directory reject `file` and `rules-dir` overrides. Scoped rules reject `rules-file` overrides, except Goose's `.goosehints` opt-in.

For hand-written files or conflicting aliases, follow [migration](@/docs/migration.md#keep-directory-specific-instructions). After you move or delete a scope, run a full sync to remove the old files; a partial sync keeps the files of tools it skips. Backups and revert cover scoped outputs too.

See [troubleshooting](@/docs/troubleshooting.md#scoped-rules) for common errors.

</details>
