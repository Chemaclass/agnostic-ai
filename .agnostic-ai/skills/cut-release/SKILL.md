---
name: cut-release
description: "Use to ship a new version (tag and GitHub Release)"
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
3. Run `make release-prepare VERSION=vX.Y.Z` (add `DATE=YYYY-MM-DD` for
   another day). `scripts/release-prepare.sh` refuses a branch other than
   `main` and edits outside the release files, then:
   - folds `changes/*.md` into `## [Unreleased]`, adding the pull request
     number from the commit that added each file to bullets that cite none;
   - moves `## [Unreleased]` into `## vX.Y.Z - DATE` without empty headings;
   - on vX.Y1.0 (v0.101.0, v0.111.0), moves the sections before vX.Y0.0 to
     `docs/CHANGELOG-archive.md` and updates both "Releases before" lines;
   - bumps `cmd/agnostic-ai/main.go`, `docs/site/config.toml` and the
     `requires` and `$schema` tag in `agnostic-ai.yaml`;
   - runs `make size-budget` and `scripts/signals-shipped.sh`;
   - writes `docs/site/content/updates/DATE-vX.Y.Z.md` with its front matter
     filled and `TODO` in each prose field.

   A rerun skips the steps already done and keeps the first date, so rerun it
   to resume after a failure.
4. Curate the dated section with the layout and entry rules in
   `.agnostic-ai/agents/changelog-curator.md`. Condensing keeps every `#NNN`.
   A path, flag, or case it drops must already be on the docs page or in the
   PR; if it is not, add it to the docs, not back to the bullet.

   Review `git diff -- scripts/target-audit/signals.tsv`. A line that only
   mentions the issue, without delivering support, does not ship the signal,
   so clear only the date newly set for that row.
5. Write the briefing prose and replace every `TODO`. Read and
   follow [references/release-briefing.md](references/release-briefing.md):
   a short title that names the user value, Highlights first, required
   upgrade actions only, at most four "What changed" groups, one target line
   per tool with the verification method stated once, and about 600 words.

   Then run `make release-check VERSION=vX.Y.Z`. It fails on a bullet over 160
   characters, more than five bullets under one heading, an empty heading, or
   a `TODO` left in the briefing.
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
7. Confirm the version file, site version, project config pins, size budget,
   dated changelog section, removed `changes/*.md` files, briefing, archive changes, and any `signals.tsv` or
   tracked generated output changes are all staged for the same commit. Review the staged diff for
   unrelated edits and record the tested tree. If release inputs changed after
   validation, rerun the affected checks before committing. Commit `chore(release): vX.Y.Z`, GPG-signed.
8. Push `main`, then dispatch `gh workflow run ci.yml --ref main`
   and wait for that run on the exact release commit. Confirm Linux,
   macOS, the Windows smoke check, and every other job that ran passed.
   The full Windows Go race suite is optional and manual only: add
   `-f full_windows=true` when needed. Each of its shards stops after ten minutes.
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
11. Run `make release-verify VERSION=vX.Y.Z`. In a temporary folder it
    downloads this machine's archive and `checksums.txt`, checks the checksum
    and the binary's `--version`, and checks that the release has notes and the
    briefing page answers 200. It exits non-zero on any mismatch. Report the
    tag, commit, release URL, briefing URL, check results, and any unresolved
    publication failure.

## Conventions

- `scripts/release-notes.sh vX.Y.Z` selects that version's dated section from `CHANGELOG.md` or its archive. Preview its output after step 3; a missing section fails the release workflow.
- A release briefing is release material, not a follow-up docs change. It is
  written after the changelog is finalized and included in the tagged
  release commit.
- The briefing orders breaking behavior, default changes, removals, and
  deprecations before additions. It never presents upstream news as shipped
  agnostic-ai support.
- Every required migration appears in the briefing's actions section. Smaller
  fixes stay in the linked release notes; the briefing is a summary, not a
  second changelog.
- Tag format `vX.Y.Z` (lowercase `v`). GoReleaser matches this prefix.
- Commit message follows Conventional Commits. Never mention AI in the message.
