# Build dependency checks

For a dependency alert, find the workflow that installs or runs the affected package below. Check its selected version and verification steps; release builds, editor packaging, vendor checks, and local experiments use different inputs.

| Workflow | Inputs and verification | Alert review |
| --- | --- | --- |
| VS Code packaging | Exact @vscode/vsce 4.0.0 and the committed npm lock, built with Node 22. Its dependencies no longer include the braces package; package checks still scan for secrets. | Run a fresh dependency scan after merging and include development dependencies. |
| JetBrains bootstrap | The committed Gradle 9 wrapper matches its official checksum. CI validates wrapper changes before execution. The distribution has its official SHA-256 pin. | Review the intentional committed binary alert against this evidence. |
| Local npm smoke | Validate seven locally packed package names, versions and exact platform pins. Install parent and host tarballs with an empty offline cache and scripts disabled. Compare installed binary bytes with the local build. | Review the variable local-path warning after the real package smoke passes. |
| Released npm install | Resolve the latest GitHub release, validate its exact version, wait for that version in npm, and pass it as a quoted shell argument. Compare the full reported CLI version with that release. | Preserve discovery of new releases between runs. Review the variable-version warning against the recorded version. |
| Release npm toolchain | Both release jobs download npm 11.5.1 from its exact archive URL and verify the committed SHA-512 before installation. Install the verified local archive offline with scripts disabled. The publication token is available only to the Publish step. | Verify the real empty-cache installation and refresh the dependency scan before closing its alerts. |
| Vendor load checks | Resolve current vendor releases to exact versions before installing. Create the lock in a temporary npm project with a fixed package name and version and private: true. Save the selected packages and their registry checksums. | Preserve the scheduled discovery check; review its intentional moving inputs separately from release builds. |
| Caveman experiment | Install the committed CLI 2.1.0 lock with `npm ci --ignore-scripts`, then retain the entrypoint digest and signed engine checks. | Review the fixed lock and experiment scope. Debian package installation still uses current repository packages. |

These checks do not record dismissed alerts. For each alert, record a confirmed fix or a reviewed exception backed by a fresh scan.

## Check npm dependencies

The [npm audit workflow](../../.github/workflows/npm-audit.yml) scans every tracked `package-lock.json` on pull requests, pushes to main, and daily. It includes development and optional dependencies, fails on findings outside the exact temporary exception below, and reads the lockfiles without installing packages. Dependabot alerts and security updates are enabled for the repository.

Repeat a scan locally with Node 22:

```sh
npm audit --prefix editors/vscode --package-lock-only --ignore-scripts --include=dev --include=optional --audit-level=low
```

Use the affected package directory in `--prefix` for a raw report. Run `node scripts/npm-audit.mjs` for every tracked lockfile with the scoped policy below. The workflow first runs `node --test scripts/npm-audit.test.mjs`. Update compatible dependencies first, then run package tests and packaging checks before committing the manifest and lockfile together.

Overrides replace vulnerable upstream selections:

- VS Code packaging: `@azure/identity` uses `@azure/msal-node` 7.1.1 for [GHSA-xcx6-jx8p-x3h5](https://github.com/advisories/GHSA-xcx6-jx8p-x3h5). Remove the override when its parent selects a patched version. Verify extension tests and VSIX packaging after changing it.
- OpenCode compatibility tests: `@opencode-ai/plugin` uses `zod` 4.6.6 for [GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5). Remove the override when the plugin updates its exact Zod dependency. Run `make opencode-hooks-test` after changing it.
- VS Code packaging: `@vscode/vsce` uses `minimatch` 11.0.3 for [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249). Remove the override when its parent selects a patched version. Verify the matching API tests, extension tests, and VSIX packaging.
- OpenCode compatibility tests: the vendor fixture overrides `glob` to 14.0.3 and `minimatch` to 11.0.3 for [glob cancellation](https://github.com/advisories/GHSA-3g3m-2fjr-6pw4) and [pattern amplification](https://github.com/advisories/GHSA-g3hv-75wg-m249). This covers both the util and Google authentication/rimraf branches and keeps clean installs compatible with npm 10 and 11. Remove the overrides when every affected descendant selects patched versions. Run `make opencode-hooks-test`, which checks the public scan/matching APIs and rimraf's resolved glob API.

### Temporary language-client audit exception

The audit wrapper permits [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249) only for `minimatch` 5.1.9 at `node_modules/vscode-languageclient/node_modules/minimatch` under client 9.0.1 in `editors/vscode/package-lock.json`. It permits the client notice only when that leaf is its sole cause. The rule expires on **2026-11-14 at 00:00 UTC**. Other advisories, versions, affected paths, causes, lockfiles, and process or report errors fail the check.

The exception pins the official registry tarball and its lockfile integrity:

```text
https://registry.npmjs.org/minimatch/-/minimatch-5.1.9.tgz
sha512-7o1wEA2RyMP7Iu7GNba9vc0RWWGACJOCZBJX2GJWip0ikV+wcOsgVuY9uE8CPiyQhkGFSlhuSkZPavN7u1c2Fw==
```

The downloaded `minimatch.js` matches the installed file, SHA-256 `1339bca8b70e33847860daa8fc3ccfa62487e143670da01a3b718d40d5fd424f`. Its `make()` at lines 192-235 expands braces, maps fixed path segments, and filters parse failures. It has no optimization-level preprocessing or growing-array transformation from this advisory.

The [maintainer's regression](https://github.com/isaacs/minimatch/commit/77a0f5c4b80bdc641c3c930a1d9edf5ad2db57f8) uses `Array(30).fill('**/../a/b').join('/')` with optimization level 2 and `braceExpandMax: 64`. In subprocesses bounded to two seconds and 96 MiB, 5.1.9 kept one pattern entry and completed; 10.2.6 exhausted memory; 11.0.3 stopped at 64 entries. The older release does not implement that optimization and retains its own matching semantics. This evidence excludes the reported amplification mechanism in these exact bytes, not every possible matching defect.

The client needs `Minimatch.makeRe()` for diagnostic, notebook, and file-operation filters. Patched minimatch 11 removes both instance and top-level `makeRe`. Published clients 9.0.1, 10.1.2, 11.0.0, and current next still need it. Retaining the original client dependency preserves its API and VS Code 1.85 support.

Remove the exception when the advisory range is corrected or the client selects a compatible patched dependency. Extending it requires a fresh source review and bounded reproducer. Clean reports need no exception after expiry. Tests reject a new advisory, version, hash, registry source, branch, extra affected path, additional cause, and expired rule.

## Update the release npm toolchain

`scripts/npm-toolchain-pin.json` records the npm version, archive URL, and SHA-512 integrity value together. The current 11.5.1 values come from [official npm registry metadata](https://registry.npmjs.org/npm/11.5.1). Update all three values together after checking the new version's official metadata and verifying the downloaded archive. The release jobs do not fetch a replacement checksum at runtime.

`scripts/npm-toolchain.sh` verifies the complete archive before passing it to the installer. It uses separate empty npm configuration files and an empty installation cache, preserves the global installation directory, and checks the installed npm version from that directory. The helper receives no publication token; only the later Publish step receives it.

## Replay a vendor load check

The Tool load workflow uploads `tool-load-dependencies`, containing `package.json`, `package-lock.json`, `selected-packages.txt`, and `environment.txt`. The lock records all selected npm packages, their dependencies, and registry checksums. Those checksums verify that downloaded files match the lock; they do not verify who published the package or which source produced it.

To repeat a check from a downloaded workflow artifact, use an existing agnostic-ai binary:

```sh
TOOL_LOAD_LOCK_DIR=/absolute/path/to/tool-load-dependencies \
  scripts/tool-load.sh --bin /absolute/path/to/agnostic-ai codex gemini opencode
```

This uses `npm ci` with the saved manifest and lock. Keep the same operating system, Node/npm versions, source revision, and selected tools when comparing results. Vendor commands can still depend on external services or configuration outside the npm graph.

To test a particular release without a saved lock, set `TOOL_LOAD_CODEX_VERSION`, `TOOL_LOAD_GEMINI_VERSION`, or `TOOL_LOAD_OPENCODE_VERSION` to an exact version. Unset variables follow current releases. Selecting an exact release alone does not fix the versions of its dependencies. `TOOL_LOAD_REPORT_DIR` chooses where a run saves its dependency evidence.

The [npm ci contract](https://docs.npmjs.com/cli/v11/commands/npm-ci/) requires a matching manifest and lock. The [Gradle checksum guidance](https://docs.gradle.org/current/userguide/gradle_wrapper.html#sec:verification) covers wrapper and distribution verification. See the [runtime experiment](rtk-caveman-runtime.md) for its separate engine verification and reproducibility limits.
