# Governance

## Roles

| Role | Definition |
|---|---|
| Users | Anyone running `agnostic-ai` |
| Contributors | Anyone with a merged PR |
| Maintainers | Commit and release rights. Currently: @Chemaclass |
| Project lead | Tie-breaker, final say on direction. Currently: @Chemaclass |

## Decision making

Lazy consensus. If no maintainer objects within 3 working days, a proposal is approved.

For larger changes, open an issue first.

Disputes go to a maintainer vote. Simple majority wins. The project lead breaks ties.

### Needs an issue first

- Breaking spec format changes
- New adapter
- New CLI command or flag
- Changes to the `Adapter` interface

### Does not

- Bug fixes
- Doc fixes, typos
- Refactors with no public behavior change
- Test additions

## Becoming a maintainer

Sustained, high-quality contribution over at least 3 months. Existing maintainers nominate. Approval requires majority vote.

Maintainers:

- Triage issues within 7 days
- Review PRs in their area
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md)
- Disclose conflicts of interest

Stepping down: open a PR updating this file.

## Project lead transfer

Transfers by the lead's choice or by maintainer consensus if the lead is inactive for 6 months.

## Releases

See [docs/internal/release-process.md](docs/internal/release-process.md).
