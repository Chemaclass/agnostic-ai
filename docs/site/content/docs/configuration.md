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

`agnostic-ai.yaml` sits at the project root, and every section is optional. The old name `agnostic.config.yaml` still loads with a warning; `agnostic-ai migrate` renames it. The `sync` and `gitignore` keys have [their own page](@/docs/sync-settings.md). The less common keys are in the [reference](@/docs/configuration-reference.md).

## Minimal project config

```yaml
version: 1
targets: [claude, codex]
gitignore:
  enabled: true
```

`agnostic-ai init` creates this file for the tools you pick. A **target** is a tool that receives generated files. **Sources** are the specs you edit; **outputs** are the files tools read.

After a change, run `agnostic-ai sync --dry-run`, then `agnostic-ai sync` and `agnostic-ai sync --check`. In scripts and coding agents, name the tools instead of relying on the prompt.

[Top-level fields](#top-level-fields) lists every key. For which value wins, see [precedence](#precedence) and [layered specs](@/docs/configuration-reference.md#layered-specs). For directory-specific instructions, give a rule a `scope` ([scoped context](@/docs/scoped-context.md)).

## Local overrides

`agnostic-ai.local.yaml` holds changes for one machine. `init` adds it to `.gitignore`. Personal specs go in [`.agnostic-ai/local/`](@/docs/local-overrides.md).

- A single value or list replaces the shared value. A group of settings merges key by key.
- An empty key leaves a shared group unchanged (lint warns, also when only commented-out children remain). Write `{}` to clear the group; model tiers reject `{}`.
- Over a list or single value, an empty key replaces the shared value. An empty `targets:` also replaces the default targets when no shared list exists. An empty `requires:` turns the version check off.

```yaml
# agnostic-ai.local.yaml (never committed)
on-unsupported: error
outputs:
  claude:
    dir: .claude-local   # overrides base; rules-file from base survives
```

## Editor validation

`init` adds this comment so editors check the file against your release's schema.

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/Chemaclass/agnostic-ai/v0.92.0/docs/schemas/config.schema.json
```

## Top-level fields

An unknown key, in `agnostic-ai.local.yaml` too, fails every command that reads the config with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed). The error names the file, line, and closest known key.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `version` | int | `1` | Schema version. Any value other than `1` fails with [AAI-004](@/docs/errors.md#aai-004-config-decode-failed). |
| [`requires`](#requires) | string | none | Releases the specs work with: a minimum, one release, or a range. |
| [`sources`](@/docs/configuration-reference.md#sources) | map | `.agnostic-ai/<kind>/` | Source directories. |
| [`targets`](#targets) | list | see [targets](#targets) | Tools to write files for. |
| [`builtins`](#built-ins) | list | none | Built-in specs to enable. `init` enables `handoff`. |
| [`outputs`](@/docs/configuration-reference.md#outputs) | map | per target | Output path overrides. |
| [`models`](@/docs/configuration-reference.md#models) | map | none | Model tiers that specs name. |
| [`on-unsupported`](@/docs/configuration-reference.md#on-unsupported) | string | `warn` | What to do with a spec kind a tool lacks. |
| [`gitignore`](@/docs/sync-settings.md#gitignore) | map | `enabled: false` | Managed `.gitignore` block. |
| [`sync`](@/docs/sync-settings.md#sync) | map | see section | Sync behavior. |
| [`verify`](#verify) | map | disabled | A project command that `verify` runs. |
| [`import`](@/docs/configuration-reference.md#import) | map | per source | Import behavior. |
| [`lint`](@/docs/configuration-reference.md#lint) | map | see section | Budgets for always-loaded text. |
| [`doctor`](@/docs/configuration-reference.md#doctor) | map | see section | Opt-in diagnostic checks. |
| [`coverage`](@/docs/configuration-reference.md#coverage) | map | none | Coverage notes to accept, and whether notes fail sync. |
| [`memory`](@/docs/configuration-reference.md#memory) | map | `personal: checkout` | Where personal memory lives. Set it in `agnostic-ai.local.yaml` only. |

## Built-ins

`builtins` turns on specs bundled with the binary. None are on unless listed; `init` writes `builtins: [handoff]` for a new project, or `[handoff, memory]` with `--memory`.

```yaml
requires: ">=0.92.0"
builtins: [handoff]
```

| Name | Adds | Setup and supported tools |
|---|---|---|
| `handoff` | A skill to leave and resume a task | [Session handoffs](@/docs/handoff.md) |
| `handoff-hook` | Automatic Git snapshots and startup notices | [Automatic snapshots](@/docs/handoff.md#add-automatic-git-snapshots) |
| `memory` | Shared project and personal memory | [Shared memory](@/docs/memory.md) |
| `rtk` | A Claude Code hook that shortens command output; install RTK separately | [RTK setup](@/docs/rtk-and-caveman.md#turn-them-on) |
| `caveman` | An on-demand response skill from a fixed upstream version; no separate program needed | [Caveman setup](@/docs/rtk-and-caveman.md#turn-them-on) |
| `side-conversation` | A skill to start a second task in a new terminal tab, linked to this session | [Side conversations](@/docs/side-conversation.md) |
| `wrap-up` | A skill to end a session with a summary to share | [Session wrap-ups](@/docs/wrap-up.md) |

<details>
<summary>Built-in precedence, removal and upgrades</summary>

Before you enable RTK or Caveman, [check for an existing copy](@/docs/rtk-and-caveman.md#if-you-already-use-rtk-or-caveman).

Unknown names fail as AAI-004 and list the valid names. A project, pack, or personal spec with the same kind and name wins over the built-in in the project. A global skill still wins on some tools; see [shared names](@/docs/global-configuration-reference.md#global-shared-names).

The same list works in the [global home config](@/docs/configuration-reference.md#global-configuration). A `builtins` list in a local config replaces the shared list, and `builtins: []` there turns them all off. When you remove a name, the next sync removes the files and tool settings it manages, and keeps unrelated settings and project overrides. A local `builtins` list never changes the committed ignore files; see [block contents](@/docs/sync-settings.md#block-contents).

When you enable built-ins, set `requires` to the release you use. An upgrade that changes built-in text makes `sync --check` fail until you sync.

</details>

## `requires`

Pick one form:

```yaml
requires: ">=0.92.0"          # minimum: 0.92.0 or any newer release
requires: "0.92.0"            # exact: only 0.92.0; "=0.92.0" means the same
requires: ">=0.92.0 <0.93.0"  # range: 0.92.0 and its patch releases
```

<details>
<summary>What each form means, upgrades and errors</summary>

- **Minimum**: the default that `init` writes, naming the release that created the project. Newer releases keep working.
- **Exact**: choose it when you commit generated files and run `sync --check` in CI, since a newer release can write different output. `init --gitignore=off` writes a minimum and prints this advice. When npm installs the tool, pin the same release in `package.json`.
- **Range**: accept patch releases but not the next minor one.

[`upgrade --requires`](@/docs/cli-reference/upgrades.md#upgrade-options) raises the value to the installed release, keeping a minimum as a minimum and an exact pin as an exact pin. A range becomes a minimum, and the command prints the value it replaced.

An installed release outside the allowed range stops every command that reads your specs before any write, with [AAI-005](@/docs/errors.md#aai-005-installed-version-outside-requires). Follow that page for fixes. `sync --watch` stops when a pulled config puts the binary outside `requires`.

A value is one or more terms separated by spaces, and the binary must match every term. A term is `>=X.Y.Z`, `<X.Y.Z`, `<=X.Y.Z`, `=X.Y.Z`, or a bare `X.Y.Z`; anything else fails as AAI-004, naming the file. That includes `^`, `~`, commas, and `||`. Before 1.0, `^0.92.0` would allow only 0.92 patch releases, so write `>=0.92.0` to accept newer releases. A build from source (`go run`, or a commit after a tag) is not a release, so it warns once instead.

`agnostic-ai.local.yaml` can replace the value. An empty `requires:` there turns the check off. The [global home config](@/docs/configuration-reference.md#global-configuration) accepts the key too.

</details>

## `targets`

Default: every tool except `amp`, `warp`, `jules`, `goose`, `augment`, `ellipsis`, `grok`, `codebuddy`, and `kimi`. `-t/--target` replaces the list for one run.

<details>
<summary>Typos, empty lists, picker and web interface</summary>

A likely typo of a built-in target, such as `cursr`, fails before any write. Any other unknown name is skipped with a warning, since it may be an external adapter missing from this machine's PATH.

An empty list keeps generated files: `sync` warns, `sync --check` fails while they remain, and [`--allow-empty-targets`](@/docs/cli-reference/sync.md) removes them.

`init` and the first `sync` can write this list through a [picker](@/docs/cli-reference/sync.md#first-sync-target-picker).

The web interface lists every tool with a switch under **Settings**, **Coding tools**; see [serve](@/docs/cli-reference/serve.md#serve).

</details>

## `verify`

`verify.command` lists the program to run and its arguments. `agnostic-ai verify` runs it without a shell, so put pipes and redirects in your script.

```yaml
verify:
  command:
    - ./scripts/verify-harness
    - --strict
```

See the [`verify` command](@/docs/cli-reference/check.md#verify) for the drift check, input, exit codes, and result recording, which uses flags only.

## Which value wins {#precedence}

Last wins:

1. Built-in defaults
2. `agnostic-ai.yaml`
3. `agnostic-ai.local.yaml`
4. CLI flags (such as `agnostic-ai sync -t claude`)

