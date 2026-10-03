+++
title = "MCP recipes"
description = "Specs for GitHub, Context7, Playwright, and the reference filesystem server, ready to copy into mcps/."
weight = 65

[extra]
group = "Reference"
+++

# MCP recipes

Four servers to start from. Copy one into `mcps/<name>.yaml`, run `agnostic-ai sync`, and every selected tool gets the entry. Field meanings are in [MCP servers](@/docs/spec-format/mcps.md).

Checked on 2026-10-03. Upstream changes names and URLs often, so compare against the linked source before you rely on one.

No recipe holds a secret. Each reads one from your shell with a [`${NAME}` reference](@/docs/spec-format/mcps.md#environment-references). Export the variable before you start the tool.

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

A GitHub token is yours, not the team's. Save the spec as `.agnostic-ai/local/mcps/github.yaml` instead. The [local layer](@/docs/local-overrides.md) stays out of Git, and `sync` still writes the server for you. Teammates who want it add their own file.

`disabled: true` does not keep a server off on Cursor, Augment, Junie, Trae, or Warp. Sync strips the key there. Leave a server you do not want out of `mcps/`, or put it in the local layer. See [`disabled` support by target](@/docs/spec-format/mcps.md#disabled-support-by-target).

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
