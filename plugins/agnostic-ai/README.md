# agnostic-ai plugin for Claude Code

Four skills that let you drive [agnostic-ai](https://github.com/Chemaclass/agnostic-ai) from inside Claude Code. Install the CLI, scaffold a project, adopt an existing config, keep every tool's files in sync.

You do not need the plugin to use agnostic-ai. The CLI does everything on its own. The plugin just means you stop leaving the chat to run it.

## Install

Two steps. Adding the marketplace does not install anything by itself.

```
/plugin marketplace add Chemaclass/agnostic-ai
/plugin install agnostic-ai@chemaclass
```

The first command points Claude Code at this repository. The second installs the plugin from it.

To remove it:

```
/plugin uninstall agnostic-ai@chemaclass
```

`/plugin` on its own opens an interactive manager, with tabs for browsing, installed plugins, marketplaces and load errors. Use it if you would rather click than remember the subcommands.

## What you get

| Skill | Runs when |
|---|---|
| `install` | `agnostic-ai` is missing, or `agnostic-ai --version` fails |
| `init` | the project has no `.agnostic-ai/` directory yet |
| `import` | the project already has config for one AI tool and the rest should get it |
| `sync` | you edited anything under `.agnostic-ai/`, or a generated file looks stale |

You do not type these. Claude Code reads each skill's description and picks the one that fits what you asked. Say "set up agnostic-ai here" and `init` runs. Say "my CLAUDE.md is out of date" and `sync` runs.

Each skill is a plain Markdown file under `skills/`. Read them. They are the whole implementation, and they are short.

## How it works

Three files matter.

`.claude-plugin/marketplace.json`, at the root of the repository, lists what this marketplace offers. Claude Code reads it when you run `/plugin marketplace add`.

`plugins/agnostic-ai/.claude-plugin/plugin.json`, this directory's manifest, carries the name, the version, and the description.

`plugins/agnostic-ai/skills/<name>/SKILL.md`, one per skill. The YAML frontmatter gives a `name` and a `description`. The description is what Claude Code matches against your request, so it says when to use the skill, not just what it does.

Nothing here bundles the binary. The `install` skill fetches it, picks the route that fits your machine, and verifies the download against the release checksums.

## Updating

Claude Code pins your install to the version in `plugin.json`. Its own docs put it plainly: "Users only get updates when the version field changes." So CI fails a pull request that edits `plugins/` and leaves the version alone.

Auto-update is **off by default for third-party marketplaces** like this one. It is on only for Anthropic's own. So you pick up a new version yourself:

```
/plugin marketplace update chemaclass
```

Or turn auto-update on for this marketplace in `/plugin`, under the Marketplaces tab.

## If something breaks

Run `agnostic-ai doctor` first. It reports what is installed, what the config points at, and what is out of sync.

A skill that never fires usually means the request did not match its description. Ask for the thing directly: "run agnostic-ai sync".

`tests/integration/plugin_marketplace_test.go` validates this whole tree on every build: the manifests parse, the marketplace's `source` resolves, the names agree, and every skill has loadable frontmatter. If you change something here and that test goes red, it is telling you the plugin would not install.
