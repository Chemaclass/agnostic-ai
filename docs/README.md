# agnostic-ai documentation

## For users

Start at [user/README.md](user/README.md) for the recommended reading order.

1. [Getting started](user/getting-started.md): install, scaffold, first sync.
2. [Spec format](user/spec-format.md): agents, skills, rules, hooks, MCP servers; nested scope; `x-<target>` namespace.
3. [Targets](user/targets.md): capability matrix and per-target output paths.
4. [Configuration](user/configuration.md): `agnostic.config.yaml` schema, including `gitignore` automation.
5. [CLI reference](user/cli-reference.md): every command and flag, including `sync --watch`, `sync --auto-sync`, `init --demo`, `init -i`, `import <source>`, `revert`, and `sync --backup`.

## For contributors

Start at [internal/README.md](internal/README.md).

1. [Architecture](internal/architecture.md): code layout, data flow, core types.
2. [Adding an adapter](internal/adding-adapters.md): ~50-line walkthrough.
3. [Contributing](internal/contributing.md): workflow and conventions.
4. [Release process](internal/release-process.md): how versions ship.
5. [Decision log](internal/decisions.md): why things are the way they are.

## Examples

[examples/](examples/) ships a reference config. For live spec templates run `agnostic-ai init --demo`.
