# Build dependency checks

Trace a dependency alert to the workflow that uses the package. Check its selected version and how the workflow verifies it.

| Workflow | Inputs and verification | Alert review |
| --- | --- | --- |
| VS Code packaging | Exact @vscode/vsce 4.0.0 and the committed npm lock, built with Node 26. Its dependencies no longer include the braces package; package checks still scan for secrets. | Run a fresh dependency scan after merging and include development dependencies. |
| JetBrains bootstrap | The committed Gradle 9 wrapper matches its official checksum. CI validates wrapper changes before execution. The distribution has its official SHA-256 pin. | Review the intentional committed binary alert against this evidence. |
| Local npm smoke | Validate seven locally packed package names, versions and exact platform pins. Install parent and host tarballs with an empty offline cache and scripts disabled. Compare installed binary bytes with the local build. | Review the variable local-path warning after the real package smoke passes. |
| Released npm install | Resolve the latest GitHub release, validate its exact version, wait for that version in npm, and pass it as a quoted shell argument. Compare the full reported CLI version with that release. | Preserve discovery of new releases between runs. Review the variable-version warning against the recorded version. |
| Release npm toolchain | Both release jobs download npm 11.5.1 from its exact archive URL and verify the committed SHA-512 before installation. Install the verified local archive offline with scripts disabled. The publication token is available only to the Publish step. | Verify the real empty-cache installation and refresh the dependency scan before closing its alerts. |
| Vendor load checks | Resolve current vendor releases to exact versions before installing. Create the lock in a temporary npm project with a fixed package name and version and private: true. Save the selected packages and their registry checksums. | Preserve the scheduled discovery check; review its intentional moving inputs separately from release builds. |
| Caveman experiment | Install the committed CLI 2.1.0 lock with `npm ci --ignore-scripts`, then retain the entrypoint digest and signed engine checks. | Review the fixed lock and experiment scope. Debian package installation still uses current repository packages. |

Record a confirmed fix or a reviewed exception for each alert. Include a fresh scan. The checks leave dismissal records to reviewers.

## Check npm dependencies

The [npm audit workflow](../../.github/workflows/npm-audit.yml) scans every tracked `package-lock.json` on pull requests, pushes to main, and daily. It includes development and optional dependencies, fails on findings outside the exact temporary exception below, and reads the lockfiles without installing packages. Dependabot alerts and security updates are enabled for the repository.

Repeat the scan locally with Node 26.

```sh
npm audit --prefix editors/vscode --package-lock-only --ignore-scripts --include=dev --include=optional --audit-level=low
```

Use the affected package directory in `--prefix` for a raw report. Run `node scripts/npm-audit.mjs` for every tracked lockfile with the scoped policy below. The workflow first runs `node --test scripts/npm-audit.test.mjs`. Update compatible dependencies first, then run package tests and packaging checks before committing the manifest and lockfile together.

Overrides replace vulnerable upstream selections:

- VS Code packaging: `@azure/identity` uses `@azure/msal-node` 7.1.1 for [GHSA-xcx6-jx8p-x3h5](https://github.com/advisories/GHSA-xcx6-jx8p-x3h5). Remove the override when its parent selects a patched version. Verify extension tests and VSIX packaging after changing it.
- OpenCode compatibility tests: `@opencode-ai/plugin` uses `zod` 4.6.6 for [GHSA-mrhx-c7xc-m3w5](https://github.com/advisories/GHSA-mrhx-c7xc-m3w5). Remove the override when the plugin updates its exact Zod dependency. Run `make opencode-hooks-test` after changing it.
- VS Code packaging: `@vscode/vsce` uses `minimatch` 11.0.3 for [GHSA-g3hv-75wg-m249](https://github.com/advisories/GHSA-g3hv-75wg-m249). Remove the override when its parent selects a patched version. Verify the matching API tests, extension tests, and VSIX packaging.
- OpenCode compatibility tests: the vendor fixture overrides `glob` to 14.0.3 and `minimatch` to 11.0.3 for [glob cancellation](https://github.com/advisories/GHSA-3g3m-2fjr-6pw4) and [pattern amplification](https://github.com/advisories/GHSA-g3hv-75wg-m249). This covers both the util and Google authentication/rimraf branches and keeps clean installs compatible with npm 10 and 11. Remove the overrides when every affected descendant selects patched versions. Run `make opencode-hooks-test`, which checks the public scan/matching APIs and rimraf's resolved glob API.

### Temporary picomatch audit exception

The audit wrapper permits [GHSA-fc5q-39xc-vjpf](https://github.com/advisories/GHSA-fc5q-39xc-vjpf) (high: a crafted glob pattern can make picomatch run for a very long time) only for `picomatch` 4.0.7 at `node_modules/picomatch`, marked as a development dependency, in three lockfiles:

| Lockfile | Excepted findings |
|---|---|
| `ui/package-lock.json` | picomatch, fdir, tinyglobby, vite, vitest, @vitejs/plugin-react |
| `editors/vscode/package-lock.json` | picomatch, fdir, tinyglobby, @vscode/vsce |
| `tests/vendor/opencode/package-lock.json` | picomatch, fdir, tinyglobby, node-gyp, @npmcli/run-script |

The rule expires on **2026-12-17 at 00:00 UTC**. It pins the registry tarball and its lockfile integrity:

```text
https://registry.npmjs.org/picomatch/-/picomatch-4.0.7.tgz
sha512-qcJu88Q2IWqJsDD529JKMdwGm/dvInW4HvQnRwiH9JtihJvzGOscDtHE3x1pBKeUOTysQ8kVmLnJ2kJu7yhcGA==
```

Each lockfile's findings must match the reviewed report exactly: advisory URL, severity, ranges, paths, causes, and downstream packages. Every excepted package must be a development dependency. Another advisory, picomatch version, tarball, path, new dependent, missing dependent, or lockfile rejects the whole exception. Unrelated findings still fail beside it.

The advisory covers every picomatch release up to 4.0.7, and no fixed release exists ([micromatch/picomatch#247](https://github.com/micromatch/picomatch/issues/247)). An override cannot help without a fixed version. npm's suggested fix moves vite, vitest and vsce back a major version, which is rejected. The risk is acceptable because only development tools use picomatch, never the shipped binary or packages, and the patterns they match come from our own config files, never from outside input.

When picomatch ships a fixed release, update the three lockfiles, run `node scripts/npm-audit.mjs` without findings, and remove the exception and its tests. Extending the end date requires a fresh check that no fixed release exists.

## Update the release npm toolchain

`scripts/npm-toolchain-pin.json` records the npm version, archive URL, and SHA-512 integrity value together. The current 11.5.1 values come from [official npm registry metadata](https://registry.npmjs.org/npm/11.5.1). Update all three values together after checking the new version's official metadata and verifying the downloaded archive. The release jobs do not fetch a replacement checksum at runtime.

`scripts/npm-toolchain.sh` verifies the complete archive before passing it to the installer. It uses separate empty npm configuration files and an empty installation cache, preserves the global installation directory, and checks the installed npm version from that directory. The helper receives no publication token; only the later Publish step receives it.

## Replay a vendor load check

The Tool load workflow uploads `tool-load-dependencies`, containing `package.json`, `package-lock.json`, `selected-packages.txt`, and `environment.txt`. The lock records all selected npm packages, their dependencies, and registry checksums. The checksums verify downloaded files against the lock, without identifying the publisher or source.

To repeat a check from a downloaded workflow artifact, use an existing agnostic-ai binary:

```sh
TOOL_LOAD_LOCK_DIR=/absolute/path/to/tool-load-dependencies \
  scripts/tool-load.sh --bin /absolute/path/to/agnostic-ai codex gemini opencode
```

This uses `npm ci` with the saved manifest and lock. Keep the same operating system, Node/npm versions, source revision, and selected tools when comparing results. Vendor commands can still depend on external services or configuration outside the npm graph.

To test a particular release without a saved lock, set `TOOL_LOAD_CODEX_VERSION`, `TOOL_LOAD_GEMINI_VERSION`, or `TOOL_LOAD_OPENCODE_VERSION` to an exact version. Unset variables follow current releases. Selecting an exact release alone does not fix the versions of its dependencies. `TOOL_LOAD_REPORT_DIR` chooses where a run saves its dependency evidence.

The [npm ci contract](https://docs.npmjs.com/cli/v11/commands/npm-ci/) requires a matching manifest and lock. The [Gradle checksum guidance](https://docs.gradle.org/current/userguide/gradle_wrapper.html#sec:verification) covers wrapper and distribution verification. See the [runtime experiment](rtk-caveman-runtime.md) for its separate engine verification and reproducibility limits.
