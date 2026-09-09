# Directory-specific instructions

[User docs](README.md)

Keep service conventions close to the code they govern, without maintaining a separate source file for each coding tool.

## Start with one directory

Select the tools your team uses in `agnostic-ai.yaml`, then create a rule:

```bash
agnostic-ai new rule payments-context --scope services/payments
```

Edit `.agnostic-ai/rules/payments-context.md`:

```markdown
---
name: payments-context
scope: services/payments
---

Use integer minor units for monetary values.
Run make test-payments after changing payment behavior.
```

Preview and sync:

```bash
agnostic-ai render .agnostic-ai/rules/payments-context.md
agnostic-ai sync
agnostic-ai sync --check
```

For Claude, this creates a native rule with `paths: [services/payments/**]`. For Codex, it creates `services/payments/AGENTS.md`. Gemini receives `services/payments/GEMINI.md`. The root instructions do not contain the payments rule.

Use `graph --spec payments-context` to see its destinations, or `why services/payments/AGENTS.md` to find its source.

## Scope contract

- `scope` names a project-relative directory and its descendants. Absolute paths, parent traversal, and paths escaping through symlinks are rejected.
- A source under `rules/services/payments/` has that layout-derived scope. It takes precedence over frontmatter. Flat sources with explicit scope are easier to maintain in monorepos.
- Keep rule names unique across the project, including different scopes.
- A deeper rule adds instructions for its own subtree. It does not copy parent rules into its file. The tool controls ancestor loading and precedence.
- `alwaysApply: true` cannot widen a scoped rule to the entire project. Sync writes the native conditional flags required by each target.
- `paths` and `globs` remain project-relative. A catch-all such as `**/*` reduces to the scope. A narrower selector such as `services/payments/**/*.go` stays narrow on a compatible target. Directory-document targets cannot represent narrower file filters. Unrepresentable intersections are skipped with a warning, or rejected with `on-unsupported: error`.
- Scope controls when instructions are discovered or applied. It does not remove instructions already loaded into a conversation.

## Native support

The following mappings were checked against vendor documentation on 2026-09-09. Output tests verify serialization and routing. They do not establish identical runtime behavior across products or versions.

| Target | Scoped destination or condition | Vendor reference |
|---|---|---|
| Claude | `.claude/rules/<scope>/<name>.md`, `paths` | [Memory](https://code.claude.com/docs/en/memory) |
| Codex | `<scope>/AGENTS.md` | [Instructions](https://developers.openai.com/codex/guides/agents-md) |
| Gemini | `<scope>/GEMINI.md` | [Context](https://geminicli.com/docs/cli/gemini-md/) |
| Cursor | `.cursor/rules/<scope>/<name>.mdc`, conditional `globs`; shared nested `AGENTS.md` when compatible peers use it | [Rules](https://cursor.com/docs/context/rules) |
| Copilot | `.github/instructions/<name>.instructions.md`, `applyTo` | [Host support](https://docs.github.com/en/copilot/reference/custom-instructions-support) |
| Cline | `.cline/rules/<scope>/<name>.md`, `paths` | [Rules](https://docs.cline.bot/customization/cline-rules) |
| Windsurf / Devin | `<scope>/.devin/rules/<name>.md`, glob trigger | [Rules](https://docs.devin.ai/cli/extensibility/rules) |
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

Aider, Zed, Junie, Crush, and Jules have no verified automatic directory scope in this implementation. Antigravity documents a Glob mode, but its serialized activation format remains unverified. These six targets skip scoped rules instead of placing their bodies in global context. Root rules continue to work. Set `on-unsupported: error` in CI to require complete coverage, or `silent` to suppress skip notices.

Runtime limits matter: Codex and OpenCode use working-directory ancestry at startup; Warp documents root/current-directory loading and best-effort cross-directory discovery. Gemini discovers context as files are accessed. OpenHands path injection applies to local conversations, not ACP conversations. Copilot support varies by host. Cline's `.cline/rules` versus `.clinerules`, Qoder Desktop parity, and Kiro custom-agent resource loading still need product-specific runtime checks. Start the tool in the relevant subtree when its loader requires that.

## Shared files and safe updates

Sync checks all configured readers, including during `--only` syncs. It rejects known combinations that cannot preserve scope:

- Kiro eagerly includes nested `AGENTS.md` files globally. It cannot share a worktree with targets emitting scoped `AGENTS.md` under this contract.
- Crush reads `.cursor/rules` recursively without applying Cursor's activation conditions. It cannot share native scoped Cursor rules.
- Targets reading the same nested `AGENTS.md` must receive identical rules. Target exclusions, different scopes, bodies, or file filters cannot be resolved by overwriting one target's output with another's.

Use compatible targets in one worktree, or separate worktrees for incompatible tools. `sync.collision-policy: prefer-spec` does not bypass these checks. Cursor uses the shared document when possible to avoid loading both that document and a native rule copy.

Hand-authored destination files and conflicting instruction aliases must be imported or moved before sync. Scoped output requires provenance headers. Native directory-document routing rejects output overrides that would move instructions away from their discovery path. Existing output options remain available for ordinary rules.

Generated scoped files participate in sync checks, the output ledger, backups, and revert. Full sync removes obsolete managed paths after a scope moves or a rule is deleted. Partial sync preserves files owned by omitted targets, so run a full sync after changing the scope layout. Existing Codex and Gemini importers retain directory provenance as `scope`; Claude's rule importer retains its source subdirectories. This feature adds no new importer, per-directory config, or inheritance language.
