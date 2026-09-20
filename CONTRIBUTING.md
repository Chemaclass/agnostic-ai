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

## Preview the site

The docs site is [Zola](https://www.getzola.org/), pinned to the version in
[Makefile](Makefile) (`ZOLA_VERSION`). Install that exact version:

```bash
brew install zola          # then check the version below
zola --version             # must match ZOLA_VERSION exactly
```

Homebrew tracks the newest release, so it can be ahead of the pin. To run the
pinned build without changing the system install, download it and put it first
on `PATH` for the command:

```bash
gh release download v0.22.0 --repo getzola/zola --pattern '*aarch64-apple-darwin.tar.gz'
tar xzf zola-v0.22.0-aarch64-apple-darwin.tar.gz
PATH="$PWD:$PATH" make site-serve
```

Then:

```bash
make site-serve            # live reload on http://127.0.0.1:1111
make site-check            # internal links
make site-build            # what Pages publishes, into _site/
make site-test             # site JS tests plus the Go site tests
```

`site-serve`, `site-check` and `site-build` all refuse to run on the wrong Zola
version and print what they found.

**A mismatched Zola does not fail `go test ./...`, it silently skips.**
`TestSiteDocs_BuildsBrowsablePublicGuides`, `TestSiteDocs_BuildsSiteSearchIndex`
and `TestTargetUpdates_PostAloneUpdatesSiteOutputs` call `t.Skip` when the
version does not match, so a green run proves nothing about a change under
`docs/site/`. Read the skip count, or run `make site-test` with the pinned
binary, before trusting a site change.

Never edit `docs/site/templates/` to satisfy a newer Zola. That breaks the
pinned build CI and Pages actually use.

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

`make lint` stops with a named message when the installed `golangci-lint` is missing or is not the pinned version. Rerun `make tools` after a pin bump. A linter older than your Go toolchain cannot decode its export data, and reports that as a typecheck failure in files you never touched.

`make hooks` optionally installs the [repository hooks](lefthook.yml), including pre-push preflight.

Commit with a Conventional Commits prefix (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`). Keep the subject under 72 characters. Open a PR describing the problem, resulting behavior, and validation. Link the issue when applicable. Add a new commit when updating an open PR.

User-visible changes need an entry under `[Unreleased]` in [CHANGELOG.md](CHANGELOG.md) and matching reference updates. See the [documentation checklist](docs/internal/contributing.md#documentation-checklist).

## Questions and reports

Use [issues](https://github.com/Chemaclass/agnostic-ai/issues) for bugs and feature requests, or [Discussions](https://github.com/Chemaclass/agnostic-ai/discussions) for questions. Report security problems through a [private advisory](https://github.com/Chemaclass/agnostic-ai/security/advisories/new).

[Contributor documentation](docs/internal/README.md) · [Code of Conduct](CODE_OF_CONDUCT.md) · [MIT license](LICENSE)
