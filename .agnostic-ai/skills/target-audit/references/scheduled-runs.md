# Scheduled runs and source invariants

The `vendor-watch` workflow (`.github/workflows/vendor-watch.yml`) runs `scripts/docfetch.sh` daily with no AI and posts moved pages to one open issue labeled `vendor-watch`. `scripts/vendor-watch.sh` keys each page by URL and hash, so a page is reported once per text change. A changed page whose URL already served that exact text on an earlier run stays quiet: a stale CDN copy or an alternating render is not news. A page that returns to earlier text within the 30-day snapshot window is therefore reported only by the next full audit. Start a run from that issue: audit the targets it lists, and close it once the lock moves.

The `tool-load` workflow (`.github/workflows/tool-load.yml`) runs `scripts/tool-load.sh` weekly: it syncs a probe project for Codex, Gemini CLI, and OpenCode and asks each tool, at its latest release, what it loaded. A failed check opens one issue labeled `tool-load`. Treat it as a breaking lead for that target: reproduce with `scripts/tool-load.sh <tool>` and find the vendor change before fixing. `tests/integration/import_fidelity_test.go` is its offline counterpart for import: real repositories' native config, imported and synced, against a list of known gaps in `tests/fidelity/<name>/expected.txt`.

Daily runs are cheap: a day when nothing moved ends after step 1. A scheduler wraps this skill; unattended runs should use `--fix` or default issue filing so results survive outside ignored local files.

Every registered target needs a `## <target>` section with `docs:` and `watch:` in the source registry. `tests/integration/target_audit_sources_test.go` enforces this. Add missing vendor sources, never weaken the test.

`scripts/docfetch.sh --urls` must resolve at least one docs URL and one changelog URL for every registered target. `scripts/docfetch_test.sh` enforces this, so a source line written outside the documented URL grammar fails the shell suite.
