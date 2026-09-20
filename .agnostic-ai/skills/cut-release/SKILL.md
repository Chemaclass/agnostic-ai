---
name: cut-release
description: Cut a new agnostic-ai release end to end. Use when the user wants to ship a new version (tag + GitHub Release).
---

# cut-release

Cuts a new release of agnostic-ai. The release commit carries the version,
exact changelog, and one public release briefing. GoReleaser publishes
artifacts and the GitHub Release once the tag is pushed. Pages publishes the
briefing from the same commit on `main`.

## When to run

The user asks to release, tag, ship, or cut a new version.

## Steps

1. Confirm working tree clean and on `main`. `git pull --ff-only`.
2. `make ci-local`. Not `make preflight`. Refuse to proceed on any failure.

   `preflight` is fmt-check, vet, lint and tests. It is the gate for a normal
   change, and it is not enough to cut a release: it skips the race detector,
   the WASM build, schema drift, `agnostic-ai lint`, the shell suite, and both
   editor extensions. `ci-local` runs every one of those, in the workflow's
   own order.

   Two gaps `ci-local` cannot close, both needing the remote run:

   - It tests on this machine's OS alone. A pull request now tests on Linux
     only (#982), so the macOS and Windows jobs may never have run against
     this tree.
   - It cannot run the plugin version-bump job, which compares a pull request
     against its merge base.

   So also confirm the CI run for the current `main` commit is green across
   all three platforms. Every push to `main` runs the full matrix, so it is
   usually already there; if it is not, dispatch one with
   `gh workflow run ci.yml --ref main` and wait for it before tagging.
3. Decide next version per semver:
   - patch: bug fixes only
   - minor: additive features
   - major: breaking changes
4. Update `CHANGELOG.md`: drop empty `### ` subsections from `## [Unreleased]`, then move the remaining lines into a new dated `## vX.Y.Z - YYYY-MM-DD` section (no brackets). The released section must never carry a `### ` heading with no entries. Reset `## [Unreleased]` to empty.

   Curate the section before moving it. Product sections come first and carry
   only changes to the tool; `### Site` comes last, holds everything whose only
   effect is on agnostic-ai.org or the docs, and is grouped into a few lines by
   theme. Condensing must not drop a claim: check that every backticked path,
   flag, and `#NNN` in the old lines still appears in the new ones.
5. Bump `version` in `cmd/agnostic-ai/main.go` and `extra.version` in
   `docs/site/config.toml`. The site footer publishes that value, and
   `make site-test` fails when either disagrees with the latest dated
   changelog section.
6. Immediately before the release commit, create exactly one
   `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` release briefing. Read and
   follow [references/release-briefing.md](references/release-briefing.md).
   Copy the release changelog section exactly, add only verified upstream CLI
   and model news, and keep those two sections visibly separate. Run
   `make site-build site-test`.
7. Confirm the version file, dated changelog section, and briefing are all
   staged for the same commit. Commit `chore(release): vX.Y.Z`, GPG-signed.
8. Tag that commit with `git tag -s vX.Y.Z -m "vX.Y.Z"`.
9. Push branch and tag: `git push && git push origin vX.Y.Z`.
10. Watch the `Release` workflow for the tag and the `Pages` workflow for the
    release commit on `main`. If either fails, fix the root cause. Do not delete
    and retag without clear reason. If the automatic Pages run is absent, use
    its documented `workflow_dispatch` path on `main`, then watch that run.

## Conventions

- Release notes pipeline reads the latest dated section from `CHANGELOG.md`. Never skip step 4.
- A release briefing is release material, not a follow-up docs change. It must
  be created after the changelog is finalized and included in the tagged
  release commit.
- The briefing orders breaking behavior, default changes, removals, and
  deprecations before additions. It never presents upstream news as shipped
  agnostic-ai support.
- Tag format `vX.Y.Z` (lowercase `v`). GoReleaser matches this prefix.
- Commit message follows Conventional Commits. Never mention AI in the message.
