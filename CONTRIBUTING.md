# Contributing to agnostic-ai

Build the CLI, make one focused change, and run the checks for the area you changed.

## Set up your checkout

You need Git, Go 1.24 or newer (see [go.mod](go.mod)), and Make. On Windows, use a shell with Make or the equivalent Go commands in [Makefile](Makefile).

```bash
git clone https://github.com/Chemaclass/agnostic-ai.git
cd agnostic-ai
make build
./agnostic-ai --help
make test
```

The binary is `./agnostic-ai`. Use this path or `go run ./cmd/agnostic-ai` while developing so you run your checkout's code.

Generate this repository's tool configuration:

```bash
./agnostic-ai sync
```

Source specs live in `.agnostic-ai/`. Generated files are ignored and must be recreated in each fresh clone or worktree. Edit the source specs when changing project instructions.

## Make a change

```bash
git switch -c fix/short-description
```

Keep each change independently usable and focused on one outcome. Add tests for behavior changes. For docs-only changes, check links and run the documented examples you changed.

| Area | Start here |
|---|---|
| Commands, config, or spec loading | [Architecture](docs/internal/architecture.md) |
| A target adapter | [Adding an adapter](docs/internal/adding-adapters.md) |
| Starter specs | [Contributing a preset](docs/internal/contributing-presets.md) |
| Browser playground | [Playground development](docs/playground/README.md) |
| Editor extensions | [Editors](editors/README.md) |

## Check and submit

Install the pinned development tools once. Ensure `$(go env GOPATH)/bin` is on `PATH` first:

```bash
make tools
```

After finishing a code change:

```bash
make preflight
```

This runs formatting checks, vet, lint, and Go tests. CI also runs race tests and separate build, schema, shell, and extension jobs. See [checks by change type](docs/internal/contributing.md#choose-checks-for-your-change).

`make hooks` optionally installs the [repository hooks](lefthook.yml), including pre-push preflight.

Commit with a Conventional Commits prefix (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`). Keep the subject under 72 characters. Open a PR describing the problem, resulting behavior, and validation. Link the issue when applicable. Add a new commit when updating an open PR.

User-visible changes need an entry under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md) and matching reference updates. See the [documentation checklist](docs/internal/contributing.md#documentation-checklist).

## Questions and reports

Use [issues](https://github.com/Chemaclass/agnostic-ai/issues) for bugs and feature requests, or [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions) for questions. Report security problems through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new).

[Contributor documentation](docs/internal/README.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE)
