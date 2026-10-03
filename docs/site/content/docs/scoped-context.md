+++
title = "Directory-specific instructions"
description = "Keep rules close to the directories where they apply across supported tools."
weight = 40

[extra]
group = "Workflows"
+++

# Directory-specific instructions

Write a service's conventions once. Sync writes each tool's native scoped instructions and keeps them out of root context.

## Start with one directory

Run commands from the project root. For a new scratch project:

```bash
echo "claude,codex,gemini,cursor" | agnostic-ai init
agnostic-ai new rule payments-context --scope services/payments
```

In an existing project, skip `init` and check [target compatibility](#shared-files-and-safe-updates). If `new rule --help` does not list `--scope`, your binary is too old. With Go, install main: `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@main`.

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

Claude gets a conditional rule. Codex and Cursor share `services/payments/AGENTS.md`. Gemini gets `services/payments/GEMINI.md`. Cursor on its own would get a native `.mdc` rule.

Edit the source and sync again. Preview with `agnostic-ai render .agnostic-ai/rules/payments-context.md`, or trace a generated file with `agnostic-ai why services/payments/AGENTS.md`.

## Check what applies to a file

Start from the file you are editing:

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

The report lists each Cursor instruction with its source spec, output path, selector, and status: `always`, `match`, `no-match`, `model-selected`, `manual`, `excluded`, `not-emitted`, or `unknown`. It reads the planned sync output. So a shared `services/payments/AGENTS.md` replaces a `.mdc` rule when a peer target needs it. It shows what is configured to apply, not what the model loaded. Only Cursor is supported. See the [CLI reference](@/docs/cli-reference/inspect.md#explain-a-source-file).

## Scope contract

- `scope` is a project-relative directory and its descendants. Use `/` separators. Absolute paths, `..`, glob-control characters, and symlink escapes are rejected. Omit `scope` for project-wide rules. `scope: .` is invalid.
- A folder under `rules/` that names a project directory scopes the rules in it: `.agnostic-ai/rules/services/payments/limits.md` has scope `services/payments`.
- A `scope:` in the frontmatter wins over the folder. A folder that names no project directory only groups rules, so `rules/modules/a.md` with `scope: src/a` is scoped to `src/a`.
- Installed packs check rule folders against the consuming project. A directory inside the pack gives a rule no scope.
- `lint` warns when a rule's folder names a project directory and its `scope:` points elsewhere (LINT020).
- Keep rule names unique across directories.
- Deeper rules add local context. The tool decides parent loading and precedence.
- `alwaysApply: true` adds no files to the scope and pattern union. Sync picks the native conditional flags.
- Scope does not remove instructions already loaded into a conversation.

## Codex rules with globs alone

With Codex enabled, an unscoped rule with `globs: [src/app/api/**, prisma/**]` writes `src/app/api/AGENTS.md` and `prisma/AGENTS.md`. That keeps the text out of root context. Compatible `AGENTS.md` readers share those files. `alwaysApply: true` keeps an unscoped rule project-wide. Set `outputs.codex.nested-glob-rules: false` to keep all unscoped rules inline.

Every selector must cover a complete subtree. `src/api/**/*.ts`, `Dockerfile`, and a mix of root files and directory patterns keep the whole rule inline, with a note naming the always-loaded rule. `on-unsupported: error` refuses that fallback. The whole rule also stays inline, with a note, for output overrides, unmanaged destinations, and root readers without verified nested discovery. Configured and one-off command-line targets go through the same reader check. Sync never widens a filename filter to a directory.

Codex loads the instruction chain for its session working directory. Start it in the rule's subtree. Starting at the root does not load every nested document for later edits.

## Narrow a rule to certain files

`scope` plus `globs` or `paths` applies to the whole scope directory **and** every matching file. Use the union for a module and its tests:

```yaml
scope: services/payments
globs: "tests/payments/**"
```

Claude writes both `services/payments/**` and `tests/payments/**` in `paths`. Codex writes `AGENTS.md` in both directories.

Patterns are project-relative, including `**/*.go`. They are not rewritten relative to the scope. A pattern inside the scope adds no files, because the scope already includes that directory. To apply a rule only to certain files, omit `scope` and keep the rule outside a source folder that implies scope.

`paths` and `globs` feed the same union. Unsupported combinations warn and skip. Set `on-unsupported: error` to fail instead, or `silent` to suppress notices.

**Breaking change:** earlier versions intersected scope and patterns. Remove `scope` from rules that used it to narrow `globs`, and write the full project-relative filters instead. Remove a catch-all `globs: "**/*"` from a rule that should apply only to its scope. See [migration](@/docs/migration.md#scope-and-pattern-unions).

{% <details summary="Selector limits by target"> %}
- Directory-document targets accept complete subdirectory patterns such as `tests/payments/**`. They reject external file filters such as `tests/payments/**/*.go` instead of applying them to every file in that directory. Root selectors such as `CHANGELOG.md` and `**/*` are also unsupported with scope on those targets. File-filter targets keep those selectors.
- Native `regex`, `applyTo`, `fileMatchPattern`, and `glob` keys cannot be combined with `scope`.
- Cline's empty `paths` array keeps the rule disabled. Continue's empty `globs` array reports unsupported with scope, because replacing it with a directory filter would change activation.
- Windsurf, Trae, and Antigravity write union rules in the project's rules directory when a selector reaches outside the scope.
- Imported native file selectors without an explicit `scope` keep their exact activation. Their source folder preserves native placement and adds no files to the selector. A target with no portable equivalent skips that native-only rule under `on-unsupported`.
- List-native targets keep literal commas inside selector list items. Targets that need comma-separated scalar selectors reject those commas. Commas inside brace patterns such as `tests/{a,b}/**` work everywhere.
{% </details> %}

## Native support

Mappings were checked against vendor documentation on 2026-09-09. Tests verify generated output, not identical behavior across live products.

| Target | Scoped destination or condition | Vendor reference |
|---|---|---|
| Claude | `.claude/rules/<scope>/<name>.md`, `paths` | [Memory](https://code.claude.com/docs/en/memory) |
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
| Factory | `<scope>/AGENTS.md` | [Instructions](https://docs.factory.ai/harness/agents-md) |
| Kilo | `<scope>/AGENTS.md`, without unconditional `instructions` entries | [Instructions](https://kilo.ai/docs/customize/agents-md) |


Aider, Zed, Junie, Crush, and Jules have no verified automatic directory scope. They skip scoped rules. Root rules still work.

Runtime limits:

- Codex and OpenCode use working-directory ancestry. Warp documents root/current-directory loading and best-effort cross-directory discovery. Gemini discovers context as files are accessed.
- Copilot support varies by host. OpenHands path injection works in local conversations, not ACP.
- Qoder Desktop parity and Kiro custom-agent resource loading are not yet checked at runtime.

## Shared files and safe updates

Sync checks all configured readers, even with `--only`:

- Kiro loads nested `AGENTS.md` globally, so it conflicts with targets emitting those files.
- Crush reads `.cursor/rules` without applying its conditions, so it conflicts with native scoped Cursor rules.
- Readers sharing nested `AGENTS.md` need identical scoped rules, target selection, and bodies.

Use compatible targets or separate worktrees. `prefer-spec` cannot bypass scope conflicts.

Keep provenance headers enabled. Directory-document targets reject `file` and `rules-dir` overrides. Scoped rules reject `rules-file` overrides, except Goose's `.goosehints` opt-in.

For hand-written files or conflicting aliases, follow [migration](@/docs/migration.md#keep-directory-specific-instructions). After you move or delete a scope, run a full sync to remove obsolete managed output. A partial sync keeps the files of omitted targets. Backups and revert cover scoped outputs too.

See [troubleshooting](@/docs/troubleshooting.md#scoped-rules) for common errors.
