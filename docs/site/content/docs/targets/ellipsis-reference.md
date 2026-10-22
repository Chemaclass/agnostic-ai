+++
title = "Ellipsis native reference"
description = "Review, environment, MCP, and import mappings for Ellipsis."
weight = 261

[extra]
nav_title = "Ellipsis reference"
group = "Reference"
+++

# Ellipsis native reference

Start with [one Ellipsis review](@/docs/targets/ellipsis.md). This reference covers optional environment setup, MCP servers, native fields, and import limits.

## How specs map to the file

| Source | In the file |
|--------|-------------|
| Review specs with no scope | The start of the reviewer's `prompt` |
| A review spec scoped to `services/api` | A later section of the same prompt |
| `x-ellipsis.ellipsis` | `ellipsis`. `kind` is always `code_review`; `name` defaults to `code-review`; `description`, `enabled` (`false` pauses the file), and `metadata` are copied as written |
| `x-ellipsis.pull_requests` | `pull_requests`, copied as written |
| `x-ellipsis.reviewer` | The one entry of `reviewers`, with the `prompt` filled in from the review specs |
| `x-ellipsis.output` | `output`, copied as written |
| `x-ellipsis.budget` | `budget`, copied as written |

The reviewer needs a `name`, one engine block (`claude_code`, `codex`, or `gemini`), and that block's `model`. Ellipsis names the model for every session, so none is guessed. Other native reviewer keys, such as `paths`, `environment`, `skills`, `permissions`, `budget`, and `comments`, pass through. A later settings spec replaces each top-level `x-ellipsis` block it sets (`ellipsis`, `pull_requests`, `reviewer`, `output`, `budget`) as a whole.

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

## Environment and MCP servers

Ellipsis starts each session from the environment the session names. A review names it with the reviewer's `environment` key, and sync sets that key to the selected environment's name. A `x-ellipsis.reviewer.environment` that names another environment stops the review file, because the specs would disagree. With no `x-ellipsis.environment`, the key passes through as written, so an environment you manage by hand still works. Set `x-ellipsis.reviewer.environment: null` to keep the reviewer in Ellipsis' basic sandbox: sync then writes no `environment` key, with or without a selection.

An environment selection writes `.ellipsis/environments/<name>.yaml`, even without portable environment or MCP specs. It needs a safe name and one repository. With no selection, environment and MCP specs produce a note instead of a file.

An invalid selection keeps an already-generated environment in place and the review continues to name it. Multiple possible generated environments stop sync until you repair the selection. Conflicting scripts or an output path outside `.ellipsis/` also prevent new output.

### Setup scripts

Ellipsis runs setup in four stages. Each script starts in `/sandbox`, the parent of the checkout, under `bash -eo pipefail`.

| Source | Stage | Runs |
|--------|-------|------|
| `install` | `hooks.build_base` | The first time, and when a file in its `inputs` changes. With no `inputs`, on every commit |
| `setup` | `hooks.after_checkout` | Once for each commit a session needs that is not prepared yet |
| `x-ellipsis.environment.hooks.before_start` | `hooks.before_start` | Every time a session starts or resumes |
| `x-ellipsis.environment.hooks.validate` | `hooks.validate` | Every time a session starts or resumes, last and never cached. A failure stops the session, and the pull request environment check runs it too |

Sync writes `install` and `setup` after `cd /sandbox/<repository>`, so they run in the repository checkout and not in its parent. Commands that read a script in the repository, such as `bash scripts/setup.bash`, work as they do locally. A script you write under `x-ellipsis.environment.hooks` goes in as written, with no `cd`. Setting both `install` and `hooks.build_base.run`, or both `setup` and `hooks.after_checkout`, stops the file; keep one source for each stage.

`hooks.build_base.inputs` lists the files whose change reruns `build_base`. Each is an exact file starting with the repository name, such as `api-repo/package-lock.json`. Globs, `..`, and files outside the repository stop the file. With `inputs` set, Ellipsis places only those files in the checkout while `build_base` runs, so list every file the install reads.

`setup-windows`, `cleanup`, `dev-commands`, and `terminals` have no Ellipsis stage or key. Sync reports each as having no effect. To run a service, start it in the background from `setup` and wait until it is ready.

### Repository, variables, and compute

These Ellipsis options have no portable spec, so they live under `x-ellipsis.environment` and are written as given:

```yaml
x-ellipsis:
  environment:
    name: api-environment
    repository:
      name: api-repo        # without the owner; checked out at /sandbox/api-repo
      owner: acme
      ref: main
    description: API checkout
    variables:
      - name: NPM_TOKEN     # no value: Ellipsis reads the stored secret of that name
      - name: NODE_ENV
        value: test
    compute:
      cpu: 2
      memory: 8GB
      timeout: 30m
    hooks:
      build_base:
        inputs: [api-repo/package.json, api-repo/package-lock.json]
      before_start: |
        cd /sandbox/api-repo
        test -d node_modules
      validate: command -v jq   # fails the session if jq is missing
    mcp_servers: [slack]    # connected integrations, listed by name
```

A variable under a credential name, such as `NPM_TOKEN`, or with a value that looks like a credential, such as `postgres://admin:pw@db/app`, may not carry a `value`: the file is committed, so the secret stays in Ellipsis. Names Ellipsis rejects (`ANTHROPIC_`, `OPENAI_`, `CODEX_`, `GEMINI_`, `GOOGLE_` and `CLAUDE_CODE_USE_` prefixes, and three exact names) stop the file.

### MCP servers

Ellipsis reads MCP servers from the environment file and ignores a repository's `.mcp.json`, so sync writes the servers there and writes no `.mcp.json` for this target. Each [MCP spec](@/docs/spec-format/mcps.md) becomes an `mcp_servers` entry, sorted by name after any integrations:

| Spec | Entry |
|------|-------|
| `type: http` with `url` and `headers` | A remote server over streamable HTTP, with `url` and `headers` |
| `stdio` with `command`, `args`, and `env` | A server that runs inside the sandbox, with `command`, `args`, and `env` |

A `${NAME}` in `env` or `headers` stays `${NAME}`, which Ellipsis fills from the stored secret `NAME` when a session starts. A plain setting needs `!literal` in the spec, as for every target. Local secret values never reach the file or a preview. A `${NAME}` in `url` or `args`, or a `${NAME:-default}` anywhere, has no Ellipsis form, so sync leaves out that server with a note, or that header or variable.

Sync leaves out a server it cannot write as it is, with a note that names the server and the reason. Under `on-unsupported: error` it stops instead:

- `sse` and `ws` transports. Ellipsis connects over streamable HTTP only.
- `oauth`, which an entry has no key for.
- `cwd` on a stdio server. Write the script path as `/sandbox/<repository>/<path>`.
- A stdio command or argument that is a path from your machine: `./`, `../`, `~`, a Windows drive, `/Users/...`, `/home/...`, `${workspaceFolder}`, or a script file name with no directory. It must start with `/sandbox/<repository>/`, and may not name another checkout or climb out of it with `..`.
- A name outside letters, digits, `-`, and `_`, or `slack` or `linear`, which Ellipsis reads as integrations.
- `disabled: true`. Ellipsis has no switch, so the server is left out, which has the same effect.

A server's `description`, `timeout`, and `roots` have no key and get a no-effect note. To give a server different values in the sandbox, set them under `x-ellipsis` on the MCP spec, which replaces the top-level key for this target only. An `env` or `headers` value under `x-ellipsis` must be a `${NAME}` reference, because the `!literal` mark works only on the top-level maps; a literal there leaves the server out:

```yaml
command: node
args: [scripts/mcp-server.js]
x-ellipsis:
  args: [/sandbox/api-repo/scripts/mcp-server.js]
```

A stdio server runs in the sandbox, so install what it needs in `install`. Sync cannot check that a command exists there. On a Codex reviewer, a server that fails to start fails the whole turn, not just that server's tools.

## Config keys

| Key | Default | Notes |
|-----|---------|-------|
| `outputs.ellipsis.review-file` | `.ellipsis/code-review.yaml` | Ellipsis reads only this path from the default branch, so move it only for a trial |
| `outputs.ellipsis.environment-file` | `.ellipsis/environments/<name>.yaml` | Must be a `.yaml` file under `.ellipsis/` |

## Settings fields

Only `x-ellipsis` reaches the file. The portable `model`, `permissions`, and `protected` fields of a settings spec have no key in a review file, so sync reports each one as having no effect. A key in `x-ellipsis` that is not one of `ellipsis`, `pull_requests`, `reviewer`, `output`, `budget`, or `environment` stops the files, because Ellipsis rejects unknown fields. The inner keys of `pull_requests`, `output`, `budget`, and the reviewer are copied without checking, so a typo there fails in the Ellipsis config check on the pull request.


See [Keep the files in Git](@/docs/targets/ellipsis.md#keep-the-files-in-git) before deploying either output.

## Import

`agnostic-ai import ellipsis` reads a hand-written `.ellipsis/code-review.yaml` back into specs:

| Source | Becomes |
|--------|---------|
| The reviewer's `prompt` | `<reviews>/review.md` |
| Everything else in the file | `<settings>/ellipsis.yaml`, under `x-ellipsis` |

A hand-written `.ellipsis/environments/*.yaml` with `ellipsis.kind: environment` imports too:

| Source | Becomes |
|--------|---------|
| A `build_base` or `after_checkout` script that starts with the line `cd /sandbox/<repository>` | `install` or `setup` in `<environments>/ellipsis.yaml`, without that line |
| Any other script, `inputs`, `before_start`, `validate` | `x-ellipsis.environment.hooks`, as written |
| The name, repository, description, metadata, variables, compute, and `slack` or `linear` entries | `x-ellipsis.environment` in `<settings>/ellipsis-environment.yaml` |
| Each server with `url` or `command` | `<mcps>/<name>.yaml` |

The environment, its servers, and its setup are scoped to `targets: [ellipsis]`, so no other tool changes. A plain `env` or `headers` value stays as written and is marked `!literal`, because Ellipsis reads every `${NAME}` as a stored secret and a missing secret fails the session. A credential becomes a `${NAME}` reference, and import prints the secret name to set in Ellipsis. A credential in a server's `url` or `args` leaves the file as written, since Ellipsis reads references in `env` and `headers` only.

When the reviewer has no `environment` and an environment file imports too, import sets `x-ellipsis.reviewer.environment: null`, so the review keeps running in the basic sandbox and does not move into the imported environment. The next sync writes the same behavior back. Comments and key order in the hand-written files are not kept. A file that carries the generated header imports nothing, since its specs are the source.

Import stops with a message and writes nothing when the review file has more than one reviewer, is not a `code_review` file, has a field Ellipsis does not list, or has a reviewer without a prompt. Keep one reviewer and import again, or leave the file hand-written and list it under [`sync.unmanaged`](@/docs/configuration.md#syncunmanaged).

An environment file it cannot express is left as written, with a note that names the reason: more than one repository, a field the schema does not list, a variable that holds a credential value, a credential in a server's `url` or `args`, or a server with both `url` and `command`. Several environment files are left too, since sync writes one. When the file is not at `.ellipsis/environments/<name>.yaml`, import says so, because sync writes there and Ellipsis would see two files with one name. `import all` detects the target from a `.ellipsis/` folder.

## Not covered

- More than one reviewer, more than one environment or repository, and the organization file in the `.ellipsis` repository.
- Cache planning beyond the `inputs` you list, scheduled agents, and skills.
- Installing the GitHub App, creating secrets, billing, and session logs.
- Rules, agents, skills, and hooks. Sync reports them as unsupported for this target.
