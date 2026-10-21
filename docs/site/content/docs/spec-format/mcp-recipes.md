+++
title = "MCP recipes"
description = "Specs for GitHub, Context7, Playwright, and the reference filesystem server, ready to copy into your MCP specs."
weight = 65

[extra]
group = "Reference"
+++

# MCP recipes

Copy a recipe into `.agnostic-ai/mcps/<name>.yaml` (or the directory set by `sources.mcps` in `agnostic-ai.yaml`). Replace example paths, and export each variable a `${NAME}` reference names before you start your coding tool. Run `agnostic-ai lint`, then `agnostic-ai sync`, and confirm the server connects in the tool.

These recipes were checked on 2026-10-03, so compare each with its linked source before you use it. [MCP servers](@/docs/spec-format/mcps.md) lists the fields, and [environment references](@/docs/spec-format/mcps.md#environment-references) lists the values each tool cannot read.

## GitHub (remote)

Source: [github/github-mcp-server, remote server](https://github.com/github/github-mcp-server/blob/main/docs/remote-server.md) and its [README](https://github.com/github/github-mcp-server#readme).

```yaml
name: github
description: GitHub issues, pull requests, and repositories.
type: http
url: https://api.githubcopilot.com/mcp/
headers:
  Authorization: Bearer ${GITHUB_PAT}
```

Create a personal access token on GitHub and export it as `GITHUB_PAT`.

### Keep it personal

A shared spec can use each person's own `${GITHUB_PAT}` without storing the token. If only you want this server, save the spec as `.agnostic-ai/local/mcps/github.yaml` instead, whatever `sources.mcps` says. [Local files](@/docs/local-overrides.md) stay out of version control, so only you get the server. Teammates who want it add their own file.

A personal file is still a loaded spec, so it does not turn the server off. To keep a server off, leave it out of every spec that loads, or turn it off in the tool itself. `disabled: true` does not keep a server off on every tool; see [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).

## Context7 (remote)

Source: [upstash/context7](https://github.com/upstash/context7#readme).

```yaml
name: context7
description: Current library documentation for the model.
type: http
url: https://mcp.context7.com/mcp
headers:
  Authorization: Bearer ${CONTEXT7_API_KEY}
```

A free key from the Context7 dashboard raises the rate limit.

## Playwright

Source: [microsoft/playwright-mcp](https://github.com/microsoft/playwright-mcp#readme).

```yaml
name: playwright
description: Drive a browser with Playwright.
command: npx
args:
  - "@playwright/mcp@latest"
```

## Filesystem

Source: [modelcontextprotocol/servers, filesystem](https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem).

```yaml
name: filesystem
description: Read and write files in the listed directories.
command: npx
args:
  - -y
  - "@modelcontextprotocol/server-filesystem"
  - /path/to/allowed/dir
```

Each path after the package name is a directory the server may reach. Replace the placeholder with a real one.
