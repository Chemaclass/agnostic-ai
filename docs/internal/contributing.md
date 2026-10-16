# Development workflow

[Contributor docs](README.md) · [Setup and first contribution](../../CONTRIBUTING.md)

Use this page after building the CLI. It covers checks, conventions, and generated files without repeating checkout setup.

## Development loop

Run the smallest relevant example while implementing, then run the required checks once the change is complete.

```bash
go run ./cmd/agnostic-ai validate
go run ./cmd/agnostic-ai list
go run ./cmd/agnostic-ai sync -t claude --dry-run
```

Use a temporary project when experimenting with imported or generated files. Adapter tests should pass a small bundle to `Emit`, then read the resulting files back from disk.

## Choose checks for your change

This section owns validation selection for contributions, audit fixes, and PR review. Select checks from the whole diff against the PR base, including changed tests, fixtures, generated files, and embedded inputs. Combine every applicable row below.

- Documentation, audit data, and source instructions use the focused checks for their changed inputs. Do not add a full local gate only to validate prose.
- Adapter, shared Go, or runtime behavior changes require `make ci-local`, plus applicable checks below that it does not cover. Unknown impact uses this full gate too.
- Classify test-only and fixture-only changes by the behavior they exercise. Rerun the affected checks; tests and fixtures for adapter, shared Go, or runtime behavior require the full gate. A test-only merge is not an exemption.
- Run expensive local checks serially, including across workers and worktrees. Use focused reproductions during diagnosis, then run the selected gate once the change set is complete. Check each command's own exit code, never a filtering pipe.

| Change | Checks |
|---|---|
| Adapter, shared Go, runtime behavior, or unknown impact | `make ci-local`; focused package and integration tests for the acceptance scenario |
| Concurrency | `make test-race` (included in `make ci-local`) |
| Dependencies (`go.mod`) or binary size | `make size-check` (included in `make ci-local`); raise [`scripts/binary-size-budget`](../../scripts/binary-size-budget) only when the growth is worth it, and say why in the PR |
| Shell scripts or CLI end-to-end behavior | `make test-shell` (included in `make ci-local`, requires bashunit on PATH) |
| Project setup, branch changes or package versions | `go test ./tests/integration -run TestProjectLifecycle -count=1`; Linux and macOS CI plus Windows smoke |
| Config struct or schema fields | `go run ./cmd/schemagen`; review the updated schema and include it |
| Project source specs or adapters | `go run ./cmd/agnostic-ai validate`, `go run ./cmd/agnostic-ai lint`, then `go run ./cmd/agnostic-ai sync` and `go run ./cmd/agnostic-ai sync --check`; review tracked output changes and keep ignored output ignored |
| Adapter output | Adapter tests and relevant saved expected output files (golden fixtures), including integration and round-trip tests; preview with `sync --dry-run` and follow [capability fixtures](../../.agnostic-ai/rules/capability-fixtures.md) when support changes |
| Playground or code used by WebAssembly (WASM) | `make playground-build`, then `make playground-serve` and exercise affected behavior in a browser; see [playground development](../playground/README.md) |
| Documentation or audit data | Check relative links, anchors, and changed data against its consumers; run existing checks for affected data and execute changed command examples in a temporary project |
| Docs site (`docs/site/`) | `make site-serve` to preview, then `make site-check` and `make site-test`, with the pinned Zola; count skipped tests, not only the exit code. See [Docs site](#docs-site) |
| Editor extension | Use Node.js 22+ for VS Code builds; follow its [development guide](../../editors/README.md) and CI job |

`make preflight` covers formatting, lint (including `govet`), and Go tests. It is useful during development but does not replace the full gate above. `make ci-local` adds race tests, the CLI and WASM builds, schema drift, source lint, shell tests, binary size, and editor checks. Use `SKIP_JETBRAINS=1` only when Java or Gradle is unavailable, report the skip, and require JetBrains CI when its inputs changed. Neither command replaces applicable site checks or required remote CI.

Record the commands, outcomes (including failures and skips), tested commit or tree, and any relevant working-tree, generated, embedded, or external inputs. Include toolchain versions and environment limits that affect the result, without secrets. A binary used for sync must match the relevant source and embedded inputs; rebuild it or use `go run` after those inputs change.

Manual evidence may be reused only when its relevant inputs are known unchanged and its environment still applies. A commit SHA alone does not establish that for an uncommitted tree, a stale binary, or external inputs. Otherwise rerun the affected checks. Changed tests and fixtures invalidate results that depend on them. After a rebase, merge, or review fix, inspect the resulting diff and rerun checks whose inputs changed.

All selected checks must pass before pushing. Missing tools, skipped tests, and pending or absent required CI are limits to resolve, not green checks. Before merging, wait for required checks on the current PR head and the remote OS checks below when applicable. Audit fixers stop at reviewable PRs; merging requires an explicitly authorized merge workflow. Releases still require the full local and remote gates in [cut-release](../../.agnostic-ai/skills/cut-release/SKILL.md).

The [project lifecycle tests](../../tests/integration/project_large_lifecycle_test.go) use a generated project with 500 entries, temporary clones and linked checkouts, and a local test installer. They check initial setup, installation using saved package versions, branch changes, project checks and migration plans that leave files unchanged. They do not connect to coding tools or call a model. On Windows, the launcher tries to save a failed command's result outside the project. Follow-up checks have time and output limits.

To investigate first-launch failures without the large project, run `go test ./tests/integration -run '^TestProjectLifecycle_FirstLaunchAfterFreshInstall$' -count=1 -v`. This installs into eight separate small projects and stops at the first setup error. It uses the same test installer and Windows launcher. Passing runs do not explain an earlier failure.

`make lint` names the problem when `golangci-lint` is missing or not the pinned version; rerun `make tools` after the required version changes. A linter older than your Go toolchain can report a type-checking failure in files you never touched.

Pull requests and pushes to `main` run the Go tests on Linux only. Nightly and manual runs add the full macOS suite and a focused Windows smoke check. Windows smoke covers fresh package startup, handoff hooks, paths, import, and linked checkouts without the race detector. Its whole job stops after seven minutes, including setup. Dispatch `gh workflow run ci.yml --ref <branch>` for these checks before releases and when a branch touches paths, file watching, renames, permissions, or import. The full Windows Go race suite is optional and manual only: add `-f full_windows=true` for a ten-minute job limit. Linux and macOS Go jobs also stop after ten minutes. Windows OpenCode and editor jobs keep their native tests and stop after seven minutes. Editor jobs on pull requests run when their source or dependencies change.

## Docs site

The site is [Zola](https://www.getzola.org/), and `zola --version` must match `ZOLA_VERSION` in the [Makefile](../../Makefile) exactly. `site-serve`, `site-check`, and `site-build` refuse a different version. To run the pinned build without replacing your install, download that release and put it first on `PATH`:

```bash
gh release download v0.23.6 --repo getzola/zola --pattern '*aarch64-apple-darwin.tar.gz'
tar xzf zola-v0.23.6-aarch64-apple-darwin.tar.gz
PATH="$PWD:$PATH" make site-serve   # http://127.0.0.1:1111
```

**A mismatched Zola does not fail `go test ./...`; it skips the three site build tests.** Run `make site-test` with the pinned binary before trusting a change under `docs/site/`.

The social preview image is rendered from `docs/site/og/card.html`. After editing the card, run `make site-og` (it needs Chrome or Chromium; set `CHROME` if it is not found). Platforms cache the image by URL, so a redesign ships under a new file name: pass the new path to `scripts/render-og-image.sh` and update `image_url` in `docs/site/config.toml` and the image tags in `docs/playground/index.html`.

A long reference page can be split into a topic index (a hub): a folder with an `_index.md` (`template = "docs/hub.html"`, `hub = true`) and one page per topic, such as `cli-reference/` and `spec-format/`. Link to a topic page as `@/docs/<hub>/<page>.md#anchor`. The hub redirects old `#anchor` links from before the split to the topic page that now holds them. Add new hubs to the sitemap and `llms-full.txt` lists in `scripts/`.

Never edit `docs/site/templates/` just to satisfy a newer Zola. Moving versions is its own change: bump `ZOLA_VERSION` in the Makefile and `.github/workflows/playground.yml` with the template edits, and compare `_site/` built on both versions.

## Conventions

| Topic | Convention |
|---|---|
| Format | `gofmt`; use `goimports` for import grouping |
| Dependencies | Prefer the standard library; justify new dependencies |
| Adapters | Stateless, constructed with `New()`; no imports between target packages |
| Shared output code | Put it in `internal/adapters/internal/emit/` |
| Tests | Behavior names; `t.TempDir()` and `testutil.Chdir(t, dir)` for filesystem tests; no external mocking libraries |
| Errors | Wrap with file or operation context and `%w` |
| Scope | One concern per PR; separate unrelated refactors |

## Generated project configuration

The repository's `.agnostic-ai/` specs generate root entry points and tool folders. Most output is ignored; `.openhands/setup.sh` is tracked for initial setup. Edit specs and review any tracked output change after syncing.

The repository's output ignore block is maintained by hand. Preserve hierarchical patterns such as `**/AGENTS.md` and the `!internal/adapters/*/testdata/**` exception that keeps saved expected output files (golden fixtures) tracked. Add new adapter paths to [.gitignore](../../.gitignore).

CI lints source specs. It does not run `sync --check` against a fresh checkout because generated files are not committed. After generating locally, `sync --check` can confirm local output matches the specs.

## Write documentation people can use

Start with the reader's task, one complete example, and a clear next action. Explain a term where the reader first needs it. Keep each fact on one canonical page and link to it from related guides.

Cut repeated benefits and cautions. Put necessary edge cases in a named disclosure or focused reference page, keeping limits that affect the example visible. Preserve full details in HTML and agent exports, and keep old links pointed at meaningful answers.

Verify examples in a temporary project. Describe what the generated files prove separately from what the coding tool does at run time.

## Documentation checklist

- New or changed flags, targets, or output fields: update the target's page under [targets](../site/content/docs/targets/_index.md) and [configuration](../site/content/docs/configuration.md), plus the [CLI reference](../site/content/docs/cli-reference/_index.md) for command changes.
- New or changed spec fields: update the matching page under [spec format](../site/content/docs/spec-format/_index.md).
- Config struct tag changes: regenerate [config.schema.json](../schemas/config.schema.json).
- New commands or visible behavior: update the matching capability or quickstart explanation in [README](../../README.md).
- User-visible changes: add an `[Unreleased]` entry in [CHANGELOG](../../CHANGELOG.md).

Keep tutorials focused on one working outcome. Put optional workflows in task guides and field details in references. Link to the main explanation instead of copying tables or setup instructions. Preserve existing page paths and section anchors when reorganizing docs.

## Before submitting

Review the diff for unrelated edits and generated files. Describe the final behavior and the checks you ran in the PR. Record design choices that need an explanation in the [decision log](decisions.md).

Read [build dependency checks](build-dependencies.md) for package integrity, repeating vendor load checks, and the evidence needed to review security alerts.
