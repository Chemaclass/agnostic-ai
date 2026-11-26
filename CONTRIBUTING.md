# Contributing to agnostic-ai

Build the CLI, make one focused change, run the checks for what you changed, and open a pull request.

## Set up

You need Git, Go (version in [go.mod](go.mod)), and Make. Add Go's tool directory to `PATH` first: `export PATH="$(go env GOPATH)/bin:$PATH"`.

```bash
git clone https://github.com/Chemaclass/agnostic-ai.git
cd agnostic-ai
make tools          # pinned golangci-lint and lefthook, once
make build
./agnostic-ai sync  # this repository's own tool config
make doctor-dev     # each tool, the version found and the version wanted
```

`make lint` and `make ci-local` need the golangci-lint that `make tools` installs. `make tools-lefthook` installs only lefthook, for `make hooks`.

`make doctor-dev` exits non-zero when a tool is missing or has the wrong version. Run it before a long check such as `make ci-local`. On a machine without Java, run `make doctor-dev SKIP_JETBRAINS=1`.

[mise](https://mise.jdx.dev/) is optional. If you use it, `mise trust && mise install` installs the Go, Node.js, Java and Zola versions CI uses, from [mise.toml](mise.toml). Then run `make tools` as above. Run the full gate as `mise exec -- make ci-local`, or with `mise activate` in your shell. Do not rely on mise shims for it: one test changes `HOME`, and a shim then cannot find its config.

`make test-shell` and `make ci-local` need [bashunit](https://github.com/TypedDevs/bashunit) on `PATH`, at the `BASHUNIT_VERSION` in [.github/workflows/ci.yml](.github/workflows/ci.yml). Download that release's `bashunit` file into a folder on `PATH` and make it executable.

The web interface is embedded from tracked assets, so Go-only builds need no Node.js. After changing `ui/src/`, its build inputs, or dependencies, use Node.js 26 and run `make ui-build`, then commit the updated `ui/embedded/` files. `make build` checks that those assets match their sources. `make ci-local` needs Node.js 26 with npm, for the site script tests and the VS Code extension.

The JetBrains plugin under `editors/jetbrains/` needs JDK 25 on `PATH`, or `JAVA_HOME` set to it. `mise install` gets it. `make ci-local` runs the plugin tests only when that folder changed. Without Java, pass `SKIP_JETBRAINS=1` and wait for the plugin's CI job instead. See its [README](editors/jetbrains/README.md#build--run-locally).

After building and syncing, run `./agnostic-ai` or `go run ./cmd/agnostic-ai` to use your checkout. Project instructions live in `.agnostic-ai/`. Edit the specs, then sync. Most generated files are ignored; [some are committed](#generated-files).

## Make a change

Create a branch, keep the change to one outcome, and add tests for behavior you change. When working with a coding agent, give it the expected result and the checks from the guide below. Have it read `AGENTS.md` before editing and review its diff before submitting.

| Area | Start here |
|---|---|
| Commands, config, or spec loading | [Architecture](docs/internal/architecture.md) |
| Code that writes a tool's configuration (an adapter) | [Adding an adapter](docs/internal/adding-adapters.md) |
| Starter specs | [Contributing a preset](docs/internal/contributing-presets.md) |
| Docs site | [Docs site](docs/internal/contributing.md#docs-site) |
| Browser playground | [Playground development](docs/playground/README.md) |
| Editor extensions | [Editors](editors/README.md) |

## Check

Choose local checks and remote CI from [checks by change type](docs/internal/contributing.md#choose-checks-for-your-change). Record their results in the PR. `make hooks` installs quick formatting and commit-message checks. It also installs hooks that run `agnostic-ai sync --keep-edits` after a branch switch, a pull and a rebase; they need `make build` first. Agent sessions also sync when they start, in every tool with a session-start hook: Claude Code, Codex, Gemini, Qwen, CodeBuddy, Factory, Qoder, OpenHands, Goose, Augment, Cursor and GitHub Copilot.

These hooks run the `.agnostic-ai/scripts/sync-generated.sh` of the branch you check out, and an agent session runs that branch's hook scripts. Checking out someone else's branch, such as a pull request from a fork, runs their code. Read its diff first, or check it out with Git hooks off, and start no agent session there until you have read its `.agnostic-ai/` changes:

```bash
LEFTHOOK=0 git checkout <branch>
git -c core.hooksPath=/dev/null checkout <branch>   # also skips hooks lefthook did not install
```

### Generated files

The root `AGENTS.md` and `CLAUDE.md` and the setup files Claude Code cloud sessions read are generated and committed: `.claude/settings.json` and `.claude/hooks/`. Commit them together with the spec change that regenerates them. CI's Spec lint job runs `./agnostic-ai sync` on a clean checkout and fails when a committed file differs or sync writes a file Git neither tracks nor ignores.

Sync has no option to leave out personal overrides, so `.agnostic-ai/local/` and `agnostic-ai.local.yaml` put your personal content in those files. While you have either, move them aside before you sync the content you commit, then move them back and sync again:

```bash
mv .agnostic-ai/local ../aai-local && ./agnostic-ai sync
git status --short   # commit the spec change and these files
mv ../aai-local .agnostic-ai/local && ./agnostic-ai sync
```

Do the same with `agnostic-ai.local.yaml`. The CI check is the reference: it syncs a checkout with no personal files.

## Submit

- Commit with a Conventional Commits prefix: `feat:`, `fix:`, `docs:`, `ref:`, `test:`, or `chore:`. Keep the subject under 72 characters and explain why in the body.
- For a user-visible change, add a changelog entry as its own file, `changes/<slug>.md` ([format](changes/README.md)), and update the docs it affects ([checklist](docs/internal/contributing.md#documentation-checklist)).
- In the pull request, say what changed and how you checked it, and link the issue. Push new commits to update it.

## Questions and reports

Open an [issue](https://github.com/Chemaclass/agnostic-ai/issues) for a bug or feature; capture the idea or problem in a short description before planning the solution. Keep the current scope and acceptance criteria in the issue body. Maintainers choose one feature label by the user outcome from the [feature map](.github/issue-features.json), alongside existing type, priority, status and source labels. See the [triage guide](docs/internal/issue-triage.md) for boundaries and the read-only verifier. Add implementation or verification links when available, including the result before closing completed work.

Ask questions in [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions). Report security problems through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new).

[Contributor documentation](docs/internal/README.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE)
