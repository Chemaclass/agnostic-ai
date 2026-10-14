+++
title = "Ellipsis"
description = "What agnostic-ai writes for Ellipsis code review: the file path, the reviewer settings, and what it leaves out."
weight = 260

[extra]
group = "Reference"
target_id = "ellipsis"
+++

# Ellipsis (`ellipsis`)

[Ellipsis](https://www.ellipsis.dev/docs/code-review) reviews each push to a pull request from one file, `.ellipsis/code-review.yaml`, on the default branch. agnostic-ai writes that file from your [review specs](@/docs/spec-format/reviews.md) and one settings spec. Ellipsis is opt-in. See [Selecting targets](@/docs/configuration.md#targets).

## Set up this tool

1. Add `ellipsis` to the [`targets` list](@/docs/configuration.md#targets) in `agnostic-ai.yaml`.
2. Name the reviewer in a settings spec, `.agnostic-ai/settings/ellipsis.yaml`:

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

3. Run these commands from the project root:

   ```bash
   agnostic-ai sync --dry-run
   agnostic-ai sync
   agnostic-ai sync --check
   ```

4. Commit `.ellipsis/code-review.yaml` to the default branch and install the Ellipsis GitHub App on the repository. Sync does neither the install nor the commit. See [Keep the file in Git](#keep-the-file-in-git).

To keep an existing file, start with [`agnostic-ai import ellipsis`](#import).

## Output

```
.ellipsis/code-review.yaml     # one review file, one reviewer
```

Sync writes the file only when a review spec reaches Ellipsis and the reviewer is complete. No review, no file. Reviews without a usable reviewer print a sync note that names the missing key, and write nothing. With `on-unsupported: error`, sync stops instead; with `silent`, it says nothing.

## How specs map to the file

| Source | In the file |
|--------|-------------|
| Review specs with no scope | The start of the reviewer's `prompt` |
| A review spec scoped to `services/api` | A later section of the same prompt |
| `x-ellipsis.ellipsis` | `ellipsis`. `kind` is always `code_review`; `name` defaults to `code-review` |
| `x-ellipsis.pull_requests` | `pull_requests`, copied as written |
| `x-ellipsis.reviewer` | The one entry of `reviewers`, with the `prompt` filled in from the review specs |
| `x-ellipsis.output` | `output`, copied as written |
| `x-ellipsis.budget` | `budget`, copied as written |

The reviewer needs a `name`, one engine block (`claude_code` or `codex`), and that block's `model`. Ellipsis names the model for every session, so none is guessed. Other native reviewer keys, such as `paths`, `environment`, `skills`, `permissions`, `budget`, and `comments`, pass through. A later settings spec wins key by key.

Nothing is added for you. With no `output` block, Ellipsis stays silent on a pass and comments on a failure, and never approves. Automatic approval needs `output.on_pass: approve` in your settings.

The prompt comes only from review specs. A `prompt` inside `x-ellipsis.reviewer` is an error, because the file would then have two sources for it. Agent specs never become a reviewer.

### Directory-specific checks

A scoped review follows the root guidance in the same prompt, under a heading that says when it applies:

```yaml
reviewers:
  - name: project-review
    claude_code:
      prompt: |-
        Flag any handler that talks to the database directly instead of going through a repository.

        ## Checks for changes under services/api/

        Apply the checks in this section only when the pull request changes a file under `services/api/`. Skip them otherwise.

        Reject a change that removes or renames a field of a public response.
      model: claude-opus-5-5
```

Sections follow in sorted directory order. The condition is text the model reads. Ellipsis has its own reviewer-level `paths` filter, which starts or skips a whole reviewer. agnostic-ai does not use it for scopes, because that needs one reviewer per directory. Check on a real pull request that Ellipsis applies the scoped checks as written before you depend on them.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.ellipsis.review-file` | `.ellipsis/code-review.yaml` | Ellipsis reads only this path from the default branch, so move it only for a trial |

## Settings fields

Only `x-ellipsis` reaches the file. The portable `model`, `permissions`, and `protected` fields of a settings spec have no key in a review file, so sync reports each one as having no effect. A key in `x-ellipsis` that is not one of `ellipsis`, `pull_requests`, `reviewer`, `output`, or `budget` stops the file, because Ellipsis rejects unknown fields. The inner keys of `pull_requests`, `output`, `budget`, and the reviewer are copied without checking, so a typo there fails in the Ellipsis config check on the pull request.

## Keep the file in Git

Ellipsis reads the file from the default branch, so it must be committed. With `gitignore.enabled: true`, list the target's reviews:

```yaml
gitignore:
  enabled: true
  commit: [ellipsis:reviews]
```

The settings spec that holds `x-ellipsis` counts with the reviews for this target. See [`gitignore`](@/docs/configuration.md#gitignore).

## Import

`agnostic-ai import ellipsis` reads a hand-written `.ellipsis/code-review.yaml` back into specs:

| Source | Becomes |
|--------|---------|
| The reviewer's `prompt` | `<reviews>/review.md` |
| Everything else in the file | `<settings>/ellipsis.yaml`, under `x-ellipsis` |

The next sync writes the same behavior back. Comments and key order in the hand-written file are not kept. A file that carries the generated header imports nothing, since its specs are the source.

Import stops with a message and writes nothing when the file has more than one reviewer, is not a `code_review` file, has a field Ellipsis does not list, or has a reviewer without a prompt. Keep one reviewer and import again, or leave the file hand-written and list it under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged). `import all` detects the target from a `.ellipsis/` folder.

## Not covered

- More than one reviewer, and the organization file in the `.ellipsis` repository.
- Environments, MCP servers, scheduled agents, and skills. Name an environment you manage by hand in `x-ellipsis.reviewer.environment`.
- Installing the GitHub App, billing, and session logs.
- Rules, agents, skills, hooks, and MCP. Sync reports them as unsupported for this target.

## Protected paths

Not enforced. A review file cannot block edits. State the paths in a rule if the agent should know about them.

## Verify

1. Install the [Ellipsis GitHub App](https://www.ellipsis.dev/docs/code-review) on the repository.
2. Check the file: `ls .ellipsis/code-review.yaml`, then commit it to the default branch.
3. Push to a pull request that changes a file under a scoped directory and one that does not. One review from `ellipsis-dev[bot]` lands per push. Ellipsis shows a config error on the pull request if the file does not parse.
