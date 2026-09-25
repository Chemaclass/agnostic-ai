# Contributing to agnostic-ai

Build the CLI, make one focused change, run the checks for what you changed, and open a pull request.

## Set up

You need Git, Go (version in [go.mod](go.mod)), and Make. Put `$(go env GOPATH)/bin` on `PATH` first.

```bash
git clone https://github.com/Chemaclass/agnostic-ai.git
cd agnostic-ai
make tools          # pinned development tools, once
make build
./agnostic-ai sync  # this repository's own tool config
```

Run `./agnostic-ai` or `go run ./cmd/agnostic-ai` so you use your checkout. Project instructions live in `.agnostic-ai/`. Most generated files are ignored; `.openhands/setup.sh` is tracked for bootstrap. Edit the specs, then sync.

## Make a change

Branch, keep the change to one outcome, and add tests for behavior you change.

| Area | Start here |
|---|---|
| Commands, config, or spec loading | [Architecture](docs/internal/architecture.md) |
| A target adapter | [Adding an adapter](docs/internal/adding-adapters.md) |
| Starter specs | [Contributing a preset](docs/internal/contributing-presets.md) |
| Docs site | [Docs site](docs/internal/contributing.md#docs-site) |
| Browser playground | [Playground development](docs/playground/README.md) |
| Editor extensions | [Editors](editors/README.md) |

## Check

```bash
make preflight
```

It runs formatting, lint (including `govet`), and the Go tests. CI adds race, shell, schema, WASM, and editor jobs. [Checks by change type](docs/internal/contributing.md#choose-checks-for-your-change) says which of those your change needs. `make hooks` installs quick formatting and commit-message checks.

## Submit

- Commit with a Conventional Commits prefix: `feat:`, `fix:`, `docs:`, `ref:`, `test:`, or `chore:`. Keep the subject under 72 characters and explain why in the body.
- For a user-visible change, add one line under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md) and update the docs it affects ([checklist](docs/internal/contributing.md#documentation-checklist)).
- In the pull request, say what changed and how you checked it, and link the issue. Push new commits to update it.

## Questions and reports

Open an [issue](https://github.com/Chemaclass/agnostic-ai/issues) for a bug or feature; a short description is enough. Ask questions in [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions). Report security problems through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new).

[Contributor documentation](docs/internal/README.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE)
