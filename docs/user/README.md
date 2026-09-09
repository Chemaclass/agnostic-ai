# User documentation

[All docs](../README.md) · [Contributor setup](../../CONTRIBUTING.md)

## Start here

[Install](installation.md), then follow [Getting started](getting-started.md) to sync one rule to two tools. For an existing project, use [Migration](migration.md).

The working loop is **edit `.agnostic-ai/` → run `agnostic-ai sync` → use your tool**. Commit the specs and project config. Decide whether to commit generated outputs using the [Git strategy](getting-started.md#commit-or-ignore-generated-outputs).

## Workflows

| Task | Guide |
|---|---|
| Keep generated outputs consistent in CI | [CI](ci.md) |
| Sync or check at commit and checkout time | [Git hooks](git-hooks.md) |
| Reuse specs across projects | [Packs](packs.md) |
| Share your own instructions across projects | [Global configuration](configuration.md#global-configuration) |
| See where a spec goes | [Graph](graph.md) |
| Trace an output to its source | [Why](why.md) |
| Resolve a failure or missing output | [Troubleshooting](troubleshooting.md) |
| Compare with symlinks or manual copies | [Alternatives](alternatives-why-not-symlinks.md) |

## Reference

| Look up | Page |
|---|---|
| Spec kinds, frontmatter, scope, and extensions | [Spec format](spec-format.md) |
| Tool support and native output paths | [Targets](targets.md) |
| Project config, overrides, and defaults | [Configuration](configuration.md) |
| Commands, flags, and exit codes | [CLI reference](cli-reference.md) |
| An `AAI-NNN` diagnostic | [Error codes](errors.md) |
| A complete config or starter specs | [Examples](../examples/README.md) |
