+++
title = "Ellipsis"
description = "Generate one repository review for Ellipsis, then verify it on a pull request."
weight = 260

[extra]
group = "Reference"
target_id = "ellipsis"

[extra.moved]
how-specs-map-to-the-file = "@/docs/targets/ellipsis-reference.md"
directory-specific-checks = "@/docs/targets/ellipsis-reference.md"
environment-and-mcp-servers = "@/docs/targets/ellipsis-reference.md"
setup-scripts = "@/docs/targets/ellipsis-reference.md"
repository-variables-and-compute = "@/docs/targets/ellipsis-reference.md"
mcp-servers = "@/docs/targets/ellipsis-reference.md"
config-keys = "@/docs/targets/ellipsis-reference.md"
settings-fields = "@/docs/targets/ellipsis-reference.md"
import = "@/docs/targets/ellipsis-reference.md"
not-covered = "@/docs/targets/ellipsis-reference.md"
set-up-this-tool = "@/docs/targets/ellipsis.md#first-review"
+++

# Ellipsis (`ellipsis`)

## Set up this tool {#set-up-this-tool .legacy-anchor}

## How specs map to the file {#how-specs-map-to-the-file .legacy-anchor}

## Directory specific checks {#directory-specific-checks .legacy-anchor}

## Environment and mcp servers {#environment-and-mcp-servers .legacy-anchor}

## Setup scripts {#setup-scripts .legacy-anchor}

## Repository variables and compute {#repository-variables-and-compute .legacy-anchor}

## Mcp servers {#mcp-servers .legacy-anchor}

## Config keys {#config-keys .legacy-anchor}

## Settings fields {#settings-fields .legacy-anchor}

## Import {#import .legacy-anchor}

## Not covered {#not-covered .legacy-anchor}


[Ellipsis](https://www.ellipsis.dev/docs/code-review) runs repository reviews from `.ellipsis/code-review.yaml` on the default branch. Sync builds that file from review specs and a target-specific settings spec. Installing the GitHub App, publishing files, creating secrets, and running sessions remain separate steps.

## First review

Select `ellipsis` in `agnostic-ai.yaml`. Add a [review spec](@/docs/spec-format/reviews.md) with one useful check for this repository, then name its reviewer in `.agnostic-ai/settings/ellipsis.yaml`:

```yaml
x-ellipsis:
  pull_requests:
    draft: false
  reviewer:
    name: project-review
    claude_code:
      model: claude-opus-5-5
  budget:
    session: 3
```

The reviewer needs a name, one engine (`claude_code` or `codex`), and an explicit model supported by Ellipsis. The review prompt comes from the review spec, not the agent specs or a settings `prompt` field.

Preview with `agnostic-ai sync --dry-run`, then run `agnostic-ai sync` and `agnostic-ai sync --check`. A review spec and complete reviewer produce `.ellipsis/code-review.yaml`. Missing requirements produce a note with the key to fix.

Existing native files? Check [import coverage](@/docs/targets/ellipsis-reference.md#import) before syncing. Sync replaces a hand-written file at its output path.

## Output

| Source | Default output |
|---|---|
| Review specs and reviewer settings | `.ellipsis/code-review.yaml` |
| Selected environment, setup, and MCP specs | `.ellipsis/environments/<name>.yaml` |

The review-only example needs no environment file. Rules, agents, skills, and hooks have no Ellipsis output. Directory-specific review checks become conditional text in the prompt, rather than separate reviewers or enforced path filters.

## Keep the files in Git

Ellipsis reads the default branch, so commit the generated review file. If managed Git ignores are enabled, allow its outputs:

```yaml
gitignore:
  enabled: true
  commit: [ellipsis:reviews]
```

This also keeps an environment named by that review trackable. Independent environments use `ellipsis:environments` or `ellipsis:mcps`. See [generated-file ownership](@/docs/configuration.md#gitignore).

## Protected paths

A review file cannot block edits. Put path-specific checks in a review spec and verify that they trigger on relevant pull requests. Portable `permissions` and `protected` settings have no Ellipsis effect.

## Verify

Install the [Ellipsis GitHub App](https://www.ellipsis.dev/docs/code-review) and publish `.ellipsis/code-review.yaml` to the default branch. Open a pull request that violates the chosen check, then one that satisfies it. Confirm one review per push and no config errors. Without an explicit output override, Ellipsis comments on failure and stays silent on a pass; it does not approve automatically.

These real review trials have not been run for this adapter. Local tests cover written files and previews only.

Next, [add an environment or MCP server](@/docs/targets/ellipsis-reference.md#environment-and-mcp-servers) if the review needs setup commands or external tools. Store referenced secrets in Ellipsis before running a session; sync does not create them.
