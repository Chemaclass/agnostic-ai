# Contributing to agnostic-ai

Fast path. Deeper docs: [docs/internal/contributing.md](docs/internal/contributing.md).

## Quick start

```bash
git clone https://github.com/Chemaclass/agnostic-ai
cd agnostic-ai
make build && make test
```

Open a PR with a [Conventional Commits](https://www.conventionalcommits.org/) subject.

## Where to start

| Goal | Link |
|---|---|
| Report a bug | [Bug issue](.github/ISSUE_TEMPLATE/bug_report.yml) |
| Request a feature | [Feature issue](.github/ISSUE_TEMPLATE/feature_request.yml) |
| Add a new AI CLI target | [Adapter issue](.github/ISSUE_TEMPLATE/new_adapter.yml), [adding-adapters.md](docs/internal/adding-adapters.md) |
| Architecture | [docs/internal/architecture.md](docs/internal/architecture.md) |
| Coding conventions, dev loop, tests | [docs/internal/contributing.md](docs/internal/contributing.md) |
| Decision log | [docs/internal/decisions.md](docs/internal/decisions.md) |

## PR rules

- Conventional Commits (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`).
- One concern per PR.
- Tests required for behavior changes.
- No drive-by refactors.
- Update `CHANGELOG.md` under `[Unreleased]` for user-visible changes.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).

## Licensing

Contributions are licensed under [MIT](LICENSE). No CLA.

## Questions and security

- General: [GitHub Discussions](https://github.com/Chemaclass/agnostic-ai/discussions)
- Security: report privately via [GitHub Security Advisories](https://github.com/Chemaclass/agnostic-ai/security/advisories/new). Do not file public issues for security bugs.

Triage target: 7 days. PRs reviewed in order. Ping after two weeks of silence.
