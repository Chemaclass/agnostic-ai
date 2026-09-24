+++
title = "Directory-specific instructions"
description = "Keep rules close to the directories where they apply across supported tools."
weight = 40

[extra]
group = "Workflows"
+++

# Directory-specific instructions


Write a service's conventions once. Sync generates each tool's native scoped instructions, without copying them into root context.

## Start with one directory

Run commands from the project root. For a new scratch project:

```bash
echo "claude,codex,gemini,cursor" | agnostic-ai init
agnostic-ai new rule payments-context --scope services/payments
```

For an existing project, skip `init` and check [target compatibility](#shared-files-and-safe-updates). If `new rule --help` does not list `--scope`, your binary predates the feature. Go users can install main with `go install github.com/chemaclass/agnostic-ai/cmd/agnostic-ai@main`.

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

Claude gets a conditional rule. Codex and Cursor share `services/payments/AGENTS.md`. Gemini gets `services/payments/GEMINI.md`. Cursor alone would get a native `.mdc` rule instead.

Edit the source and sync again. Use `agnostic-ai render .agnostic-ai/rules/payments-context.md` to preview, or `agnostic-ai why services/payments/AGENTS.md` to trace the generated file.

## Check what applies to a file

Start from the file you are editing:

```bash
agnostic-ai explain --file services/payments/handler.go --target cursor
```

The report lists each Cursor instruction with its source spec, output path, selector, and status: `always`, `match`, `no-match`, `model-selected`, `manual`, `excluded`, `not-emitted`, or `unknown`. It reads the planned sync output, so a shared `services/payments/AGENTS.md` shows up in place of a `.mdc` rule when a peer target needs it. The report shows configured applicability. It does not record what the model loaded. Only Cursor is supported. See the [CLI reference](@/docs/cli-reference.md#explain-a-source-file).

## Scope contract

- `scope` covers a project-relative directory and its descendants. Use `/` separators. Absolute paths, `..`, glob-control characters, and symlink escapes are rejected. Omit scope for project-wide rules; `scope: .` is invalid.
- Source subdirectories take precedence: `.agnostic-ai/rules/services/payments/limits.md` has scope `services/payments`. Prefer flat sources with explicit scope for easier navigation.
- Keep rule names unique across directories.
- Deeper rules add local context. Parent loading and precedence belong to the tool.
- `alwaysApply: true` cannot widen scope. Sync chooses the native conditional flags.
- Scope does not remove instructions already loaded into a conversation.

## Narrow a rule to certain files

On file-filter targets, use project-relative patterns inside the scope:

```yaml
scope: services/payments
globs: "services/payments/**/*.go"
```

`**/*` reduces to the whole scope. `**/*.go` is not rewritten relative to the scope and is unsupported. Directory-document targets such as Codex cannot express narrower file filters.

Prefer one `paths` or `globs` selector per rule. If both are present, their constrained patterns must agree or one must cover the scope. Multiple patterns work for scoped Claude, Cline, Continue, Qoder, and OpenHands rules. Native `regex`, `applyTo`, `fileMatchPattern`, and `glob` keys cannot be combined with `scope`. Cline's empty `paths` array keeps the rule disabled. Continue's empty `globs` array reports unsupported with scope, since replacing it with a directory filter would change activation.

Unsupported combinations warn and skip. Set `on-unsupported: error` to fail instead, or `silent` to suppress notices.

## Native support

Mappings checked against vendor documentation on 2026-09-09. Tests verify generated output, not identical behavior across live products.

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


Aider, Zed, Junie, Crush, and Jules have no verified automatic directory scope. These five targets skip scoped rules; root rules still work.

Runtime limits:

- Codex and OpenCode use working-directory ancestry. Warp documents root/current-directory loading and best-effort cross-directory discovery. Gemini discovers context as files are accessed.
- Copilot support varies by host. OpenHands path injection supports local conversations, not ACP.
- Qoder Desktop parity and Kiro custom-agent resource loading still need product-specific runtime checks.

## Shared files and safe updates

Sync checks all configured readers, even with `--only`:

- Kiro loads nested `AGENTS.md` globally, so it conflicts with targets emitting those files.
- Crush reads `.cursor/rules` without applying its conditions, so it conflicts with native scoped Cursor rules.
- Readers sharing nested `AGENTS.md` need identical scoped rules, target selection, and bodies.

Use compatible targets or separate worktrees. `prefer-spec` cannot bypass scope conflicts.

Keep provenance headers enabled. Directory-document targets reject `file` and `rules-dir` overrides. Scoped rules reject `rules-file` overrides except Goose's `.goosehints` opt-in.

For existing hand-authored files or conflicting aliases, follow [migration](@/docs/migration.md#keep-directory-specific-instructions). After moving or deleting a scope, run a full sync to remove obsolete managed output; partial sync preserves omitted targets' files. Backups and revert work for scoped outputs too.

See [troubleshooting](@/docs/troubleshooting.md#scoped-rules) for common errors.
