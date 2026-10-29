---
name: cut-release
description: Cut a new agnostic-ai release end to end. Use when the user wants to ship a new version (tag + GitHub Release).
---

# cut-release

Cuts a new release of agnostic-ai. The release commit carries the version,
exact changelog, and one public release briefing. GoReleaser publishes
artifacts and the GitHub Release once the tag is pushed. Pages publishes the
briefing from the same commit on `main`.

## Scope

Use this workflow when the user asks to publish a new version. A request to prepare release material stops before pushing or tagging; preserve any narrower scope from the user. On later release authorization, inspect the prepared version, changelog, briefing, and working changes, then resume at the first incomplete step. Preserve recognized release edits; do not repeat the changelog rollover. Reuse checks only while their inputs remain unchanged.

## Steps

1. Confirm `main` and a clean working tree for a new release, then `git pull --ff-only`. For resumed preparation with uncommitted release edits, fetch and inspect remote changes without discarding that work; reconcile an advanced base before validation.
2. Use the requested version, or select the next version from the changes since
   the last tag (`git log --oneline <last-tag>..HEAD`). Check diffs and migration
   notices as well as commit types; the changelog groups by scope, not kind:
   - patch: bug fixes only
   - minor: additive features
   - major: breaking changes after v1.0; while on 0.x, use a minor release
     with explicit breaking notes, as required by `spec-migration`
3. Update `CHANGELOG.md`: drop empty `### ` and `#### ` headings from `## [Unreleased]`, then move the remaining lines into a new dated `## vX.Y.Z - YYYY-MM-DD` section (no brackets). The released section must never carry a heading with no entries. Reset `## [Unreleased]` to empty.

   Curate the section first with the layout, entry rules, and length check in
   `.agnostic-ai/agents/changelog-curator.md`. Condensing keeps every `#NNN`.
   A path, flag, or case it drops must already be on the docs page or in the
   PR; if it is not, add it to the docs, not back to the bullet.

   `CHANGELOG.md` keeps the current group of ten minor versions. When you
   release v0.81.0, v0.91.0, and so on, move every dated section older than
   v0.80.0, v0.90.0, and so on to the top of `docs/CHANGELOG-archive.md`.
   Update the "Releases before" line at the end of `CHANGELOG.md` and at the
   top of the archive. `scripts/release-notes.sh` and
   `scripts/signals-shipped.sh` read both files.

   Then run `scripts/signals-shipped.sh`. It sets `shipped-date` in
   `scripts/target-audit/signals.tsv` to the release date on every signal
   whose issue the new section cites, and never moves a date already set.
   Review the changed rows in `git diff -- scripts/target-audit/signals.tsv`.
   A line that only mentions the issue, without delivering support, does not
   ship the signal, so clear only the date newly set for that row.
4. Bump `version` in `cmd/agnostic-ai/main.go`, `extra.version` in
   `docs/site/config.toml`, and both pins in the root `agnostic-ai.yaml`:
   exact `requires: "X.Y.Z"` and the `$schema` URL's `vX.Y.Z` tag. Keep
   them in the release commit. `make site-test` fails when these values
   disagree with the latest dated changelog section.
5. After finalizing the changelog, create exactly one
   `docs/site/content/updates/YYYY-MM-DD-vX.Y.Z.md` release briefing. Read and
   follow [references/release-briefing.md](references/release-briefing.md).
   Explain how to upgrade from the previous release, name the main shipped
   value with a few useful examples, and select consequential target changes. Keep
   agnostic-ai support separate from upstream availability.
6. Validate the completed release tree before committing. Follow the generation
   checks for project source specs in repository-root `docs/internal/contributing.md`,
   section "Choose checks for your change", then run `make ci-local` and
   `make site-build site-test`. Check each command's own exit code, never
   through a pipe, and refuse to proceed on any failure.

   `preflight` covers formatting, lint (including govet), and Go tests. A
   release also needs race tests, the WASM build, schema drift, spec lint, shell
   tests, binary size, and editor checks. `ci-local` runs those checks with the
   JetBrains condition below. Inspect skipped site tests; a successful exit
   with missing required coverage does not satisfy the gate.

   The JetBrains plugin runs only when `editors/jetbrains/` changed since
   `origin/main`; `FORCE_JETBRAINS=1` runs it anyway. `SKIP_JETBRAINS=1`
   exists for machines without Java or access to the Gradle distribution. If
   you use it, say so in the release report: that job was not gated locally.

   `ci-local` tests on this machine's OS alone. Step 8 checks the exact
   release commit across all three OSes. The plugin version-bump job is
   PR-only; if the plugin changed since the last release, confirm that its
   PR check passed.
7. Confirm the version file, site version, project config pins, dated
   changelog section, briefing, archive changes, and any `signals.tsv` or
   tracked generated output changes are all staged for the same commit. Review the staged diff for
   unrelated edits and record the tested tree. If release inputs changed after
   validation, rerun the affected checks before committing. Commit `chore(release): vX.Y.Z`, GPG-signed.
8. Push `main`, then dispatch `gh workflow run ci.yml --ref main`
   and wait for that run on the exact release commit. Confirm Linux,
   macOS, the Windows smoke check, and every other job that ran passed.
   The full Windows Go race suite is optional and manual only: add
   `-f full_windows=true` when needed. It takes about 32 minutes and stops after 45.
9. Record the release SHA and confirm the remote tag does not already exist.
   Tag that exact green commit with `git tag -s vX.Y.Z <release-SHA> -m "vX.Y.Z"`,
   then push the tag with `git push origin vX.Y.Z`. Never infer the tagged commit
   from a branch that may have advanced during CI.
10. Watch the `Release` workflow for the tag and the `Pages` workflow for the
    release commit on `main`, including package verification jobs. If a run
    fails, inspect its logs and published state before retrying. Keep a published
    tag fixed; stop and report any recovery that requires replacing a release.
    If the automatic Pages run is absent, dispatch `playground.yml` on `main`,
    then watch that run and record its SHA.
11. Verify the published release: download the archive for this machine and
    `checksums.txt` to a temporary directory, verify its checksum, and run the
    extracted binary with `--version`. Confirm the version, release notes, and
    published briefing match the release. Report the tag, commit, release URL,
    briefing URL, check results, and any unresolved publication failure.

## Conventions

- `scripts/release-notes.sh vX.Y.Z` selects that version's dated section from `CHANGELOG.md` or its archive. Preview its output after step 3; a missing section fails the release workflow.
- A release briefing is release material, not a follow-up docs change. It must
  be created after the changelog is finalized and included in the tagged
  release commit.
- The briefing orders breaking behavior, default changes, removals, and
  deprecations before additions. It never presents upstream news as shipped
  agnostic-ai support.
- Cover every substantial feature and migration. A fixed bullet count must
  never hide a change a reader needs to use the release.
- Tag format `vX.Y.Z` (lowercase `v`). GoReleaser matches this prefix.
- Commit message follows Conventional Commits. Never mention AI in the message.
