+++
title = "Configuration"
description = "Choose tools, source folders, generated file paths, and settings for your project or machine."
weight = 130

[extra]
group = "Reference"

[extra.moved]
sync = "@/docs/sync-settings.md#sync"
synccollision-policy = "@/docs/sync-settings.md#synccollision-policy"
synctarget-overview = "@/docs/sync-settings.md#synctarget-overview"
syncresolve-imports = "@/docs/sync-settings.md#syncresolve-imports"
syncdropped-summary = "@/docs/sync-settings.md#syncdropped-summary"
syncshared-skills = "@/docs/sync-settings.md#syncshared-skills"
syncunmanaged = "@/docs/sync-settings.md#syncunmanaged"
syncoutput-manifest = "@/docs/sync-settings.md#syncoutput-manifest"
syncallow-global-names = "@/docs/sync-settings.md#syncallow-global-names"
syncglobal-name-clash = "@/docs/sync-settings.md#syncglobal-name-clash"
gitignore = "@/docs/sync-settings.md#gitignore"
claude-code-worktrees = "@/docs/sync-settings.md#claude-code-worktrees"
block-contents = "@/docs/sync-settings.md#block-contents"
memory = "@/docs/configuration-reference.md#memory"
sources = "@/docs/configuration-reference.md#sources"
outputs = "@/docs/configuration-reference.md#outputs"
models = "@/docs/configuration-reference.md#models"
import = "@/docs/configuration-reference.md#import"
importcodexshred = "@/docs/configuration-reference.md#importcodexshred"
lint = "@/docs/configuration-reference.md#lint"
doctor = "@/docs/configuration-reference.md#doctor"
doctorcheck-referencesignore = "@/docs/configuration-reference.md#doctorcheck-referencesignore"
on-unsupported = "@/docs/configuration-reference.md#on-unsupported"
coverage-notes = "@/docs/configuration-reference.md#coverage-notes"
coverage = "@/docs/configuration-reference.md#coverage"
coveragefail-on-notes = "@/docs/configuration-reference.md#coveragefail-on-notes"
coverageaccept = "@/docs/configuration-reference.md#coverageaccept"
watched-inputs = "@/docs/configuration-reference.md#watched-inputs"
path-semantics = "@/docs/configuration-reference.md#path-semantics"
entry-point-files = "@/docs/configuration-reference.md#entry-point-files"
per-target-paragraphs = "@/docs/configuration-reference.md#per-target-paragraphs"
layered-specs = "@/docs/configuration-reference.md#layered-specs"
global-configuration = "@/docs/configuration-reference.md#global-configuration"
global-shared-names = "@/docs/configuration-reference.md#global-shared-names"
global-default-model-and-effort = "@/docs/configuration-reference.md#global-default-model-and-effort"
global-mcp-servers = "@/docs/configuration-reference.md#global-mcp-servers"
+++

# Configuration

## sync {#sync .legacy-anchor}

## sync.collision-policy {#synccollision-policy .legacy-anchor}

## sync.target-overview {#synctarget-overview .legacy-anchor}

## sync.resolve-imports {#syncresolve-imports .legacy-anchor}

## sync.dropped-summary {#syncdropped-summary .legacy-anchor}

## sync.shared-skills {#syncshared-skills .legacy-anchor}

## sync.unmanaged {#syncunmanaged .legacy-anchor}

## sync.output-manifest {#syncoutput-manifest .legacy-anchor}

## sync.allow-global-names {#syncallow-global-names .legacy-anchor}

## sync.global-name-clash {#syncglobal-name-clash .legacy-anchor}

## gitignore {#gitignore .legacy-anchor}

## Claude Code worktrees {#claude-code-worktrees .legacy-anchor}

## Block contents {#block-contents .legacy-anchor}

## models {#models .legacy-anchor}

## sources {#sources .legacy-anchor}

## coverage {#coverage .legacy-anchor}

## path-semantics {#path-semantics .legacy-anchor}

## entry-point-files {#entry-point-files .legacy-anchor}

`agnostic-ai.yaml` sits at the project root. Every section is optional. This page covers the keys most people change. The rest are in the [reference](@/docs/configuration-reference.md); `sync` and `gitignore` have [their own page](@/docs/sync-settings.md).

## Minimal project config

```yaml
version: 1
targets: [claude, codex]
gitignore:
  enabled: true
```

`agnostic-ai init` writes this file for the tools you pick. A **target** is a tool that receives generated files. **Sources** are the specs you edit. **Outputs** are the files tools read.

After a change:

1. `agnostic-ai sync --dry-run` shows the planned output.
2. `agnostic-ai sync` writes it.
3. `agnostic-ai sync --check` confirms nothing drifted.

Which value wins: [precedence](#precedence) and [layered specs](@/docs/configuration-reference.md#layered-specs). For directory-specific instructions, give a rule a `scope` ([scoped context](@/docs/scoped-context.md)).

<details>
<summary>Details: scripts, old file name, editor schema</summary>

- In scripts and coding agents, name the tools instead of relying on the prompt.
- The old file name `agnostic.config.yaml` still loads with a warning. `agnostic-ai migrate` renames it.
- `init` adds a schema comment so editors check the file against your release's schema:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.92.0/docs/schemas/config.schema.json
```

</details>

## Local overrides

`agnostic-ai.local.yaml` holds changes for one machine. `init` git-ignores it. Personal specs go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

```yaml
# agnostic-ai.local.yaml (never committed)
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

| Local value | Result |
|---|---|
| A single value or a list | Replaces the shared value. |
| A group of settings | Merges key by key. |
| `{}` | Clears the shared group. Model tiers reject `{}`. |
| An empty key over a group | Leaves the group unchanged, and lint warns. |
| An empty key over a list or single value | Replaces the shared value. |

<details>
<summary>Details: empty keys</summary>

- An empty key with only commented-out children counts as an empty key over a group.
- An empty `targets:` also replaces the default targets when no shared list exists.
- An empty `requires:` turns the version check off.

</details>

## Top-level fields

An unknown key, in `agnostic-ai.local.yaml` too, fails every command that reads the config with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed), naming the file, line, and closest known key.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Any value other than `1` fails with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed). |
| [`requires`](#requires) | string | none | Releases the specs work with. |
| [`sources`](@/docs/configuration-reference.md#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | see [targets](#targets) | Tools to write files for. |
| [`builtins`](#built-ins) | list | none | Built-in specs to enable. |
| [`outputs`](@/docs/configuration-reference.md#outputs) | map | per target | Output paths. |
| [`models`](@/docs/configuration-reference.md#models) | map | none | Model tiers. |
| [`on-unsupported`](@/docs/configuration-reference.md#on-unsupported) | string | `warn` | Spec kind a tool lacks. |
| [`gitignore`](@/docs/sync-settings.md#gitignore) | map | `enabled: false` | `.gitignore` block. |
| [`sync`](@/docs/sync-settings.md#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | Command `verify` runs. |
| [`import`](@/docs/configuration-reference.md#import) | map | per source | Import behavior. |
| [`lint`](@/docs/configuration-reference.md#lint) | map | see section | Always-loaded text budgets. |
| [`doctor`](@/docs/configuration-reference.md#doctor) | map | see section | Opt-in diagnostic checks. |
| [`coverage`](@/docs/configuration-reference.md#coverage) | map | none | Coverage notes to accept. |
| [`memory`](@/docs/configuration-reference.md#memory) | map | `personal: checkout` | Personal memory location. Local config only. |

## Built-ins

`builtins` turns on specs bundled with the binary. None are on unless listed.

```yaml
requires: ">=0.92.0"
builtins: [handoff]
```

`init` writes `[handoff]`, or `[handoff, memory]` with `--memory`.

| Name | Adds | Setup |
|---|---|---|
| `handoff` | A skill to leave and resume a task | [Session handoffs](@/docs/handoff.md) |
| `handoff-hook` | Automatic Git snapshots and startup notices | [Automatic snapshots](@/docs/handoff.md#add-automatic-git-snapshots) |
| `memory` | Shared project and personal memory | [Shared memory](@/docs/memory.md) |
| `rtk` | A Claude Code hook that shortens command output | [RTK setup](@/docs/rtk-and-caveman.md#turn-them-on) |
| `caveman` | An on-demand short-reply skill | [Caveman setup](@/docs/rtk-and-caveman.md#turn-them-on) |
| `side-conversation` | A skill to start a second task in a new tab | [Side conversations](@/docs/side-conversation.md) |
| `wrap-up` | A skill to end a session with a summary to share | [Session wrap-ups](@/docs/wrap-up.md) |
| `cloud-handoff` | A skill to push the task and continue it in a cloud session | [Cloud handoffs](@/docs/cloud-handoff.md) |
| `approvals` | A Claude Code, Codex and Kiro hook that answers routine permission prompts. Home config only: a project sync skips it | [Approvals](@/docs/approvals.md) |

Set `requires` to the release you use. An upgrade that changes built-in text makes `sync --check` fail until you sync.

<details>
<summary>Details: precedence, removal, local lists</summary>

- Before you enable RTK or Caveman, [check for an existing copy](@/docs/rtk-and-caveman.md#if-you-already-use-rtk-or-caveman). Install RTK separately. Caveman comes from a fixed upstream version and needs no separate program.
- An unknown name fails as AAI-004 and lists the valid names.
- A project, pack, or personal spec with the same kind and name wins over the built-in in the project.
- A global skill still wins on some tools ([shared names](@/docs/global-configuration-reference.md#global-shared-names)).
- The same list works in the [global home config](@/docs/configuration-reference.md#global-configuration).
- A `builtins` list in a local config replaces the shared list. `builtins: []` there turns every built-in off.
- Removing a name makes the next sync delete the files and tool settings it managed. Unrelated settings and project overrides stay.
- A local `builtins` list never changes the committed ignore files ([block contents](@/docs/sync-settings.md#block-contents)).

</details>

## `requires`

`requires` names the releases your specs work with. Pick one form:

```yaml
requires: ">=0.92.0"          # minimum: 0.92.0 or any newer release
requires: "0.92.0"            # exact: only 0.92.0; "=0.92.0" means the same
requires: ">=0.92.0 <0.93.0"  # range: 0.92.0 and its patch releases
```

| Form | Use it when |
|---|---|
| Minimum | Default. `init` writes the release that created the project. Newer releases keep working. |
| Exact | You commit generated files and run `sync --check` in CI, since a newer release can write different output. |
| Range | You accept patch releases but not the next minor one. |

An installed release outside the range stops every command that reads your specs, before any write, with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires).

<details>
<summary>Details: syntax, upgrades, local override</summary>

- When npm installs the tool, pin the same exact release in `package.json`.
- A value is one or more space-separated terms. The binary must match every term.
- A term is `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`.
- Anything else fails as AAI-004 and names the file. That includes `^`, `~`, commas, and `||`.
- Before 1.0, `^0.92.0` would allow only 0.92 patch releases. Write `>=0.92.0` to accept newer ones.
- A build from source (`go run`, or a commit after a tag) is not a release. It warns once instead.
- `init --gitignore=off` writes a minimum and prints the advice to pin an exact release.
- [`upgrade --requires`](@/docs/cli-reference/upgrades.md#upgrade-options) raises the value to the installed release. A minimum stays a minimum and an exact pin stays exact. A range becomes a minimum, and the command prints the value it replaced.
- `sync --watch` stops when a pulled config puts the binary outside `requires`.
- `agnostic-ai.local.yaml` can replace the value. An empty `requires:` there turns the check off.
- The [global home config](@/docs/configuration-reference.md#global-configuration) accepts the key too.

</details>

## `targets`

`targets` lists the tools to write files for. `-t/--target` replaces the list for one run.

`init` and the first `sync` can write the list through a [picker](@/docs/cli-reference/sync.md#first-sync-target-picker). The [web interface](@/docs/cli-reference/serve.md#serve) has a switch per tool under **Settings**, **Coding tools**.

<details>
<summary>Details: defaults, typos, empty lists</summary>

- The default list is every tool except `amp`, `warp`, `jules`, `goose`, `augment`, `ellipsis`, `grok`, `codebuddy`, and `kimi`.
- A likely typo of a built-in target, such as `cursr`, fails before any write.
- Any other unknown name is skipped with a warning. It may be an external adapter missing from this machine's PATH.
- An empty list keeps generated files. `sync` warns, and `sync --check` fails while they remain.
- [`--allow-empty-targets`](@/docs/cli-reference/sync.md) removes them.

</details>

## `verify`

`verify.command` lists the program to run and its arguments. `agnostic-ai verify` runs it without a shell, so put pipes and redirects in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

The [`verify` command](@/docs/cli-reference/check.md#verify) covers the drift check, input, exit codes, and result recording (flags only).

## Which value wins {#precedence}

The last item wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags, such as `agnostic-ai sync -t claude`
