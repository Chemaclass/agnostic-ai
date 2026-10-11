# Build dependency checks

Release builds, editor packaging, vendor compatibility checks, and local experiments use different inputs. Review each dependency alert against the route that installs or executes it.

| Route | Inputs and verification | Alert review |
| --- | --- | --- |
| VS Code packaging | Exact @vscode/vsce 4.0.0 and the committed npm lock, built with Node 22. Its dependencies no longer include the braces package; package checks still scan for secrets. | Run a fresh dependency scan after merging and include development dependencies. |
| JetBrains bootstrap | The committed Gradle9 wrapper matches its official checksum. CI validates wrapper changes before execution. The distribution has its official SHA-256 pin. | Review the intentional committed binary alert against this evidence. |
| Local npm smoke | Validate seven locally packed package names, versions and exact platform pins. Install parent and host tarballs with an empty offline cache and scripts disabled. Compare installed binary bytes with the local build. | Review the variable local-path warning after the real package smoke passes. |
| Released npm install | Resolve the latest GitHub release, validate its exact version, wait for that version in npm, and pass it as a quoted shell argument. Compare the full reported CLI version with that release. | Preserve discovery of new releases between runs. Review the variable-version warning against the recorded version. |
| Release npm toolchain | Both release jobs download npm 11.5.1 from its exact archive URL and verify the committed SHA-512 before installation. Install the verified local archive offline with scripts disabled. The publication token is available only to the Publish step. | Verify the real empty-cache installation and refresh the dependency scan before closing its alerts. |
| Vendor load checks | Resolve current vendor releases to exact versions before installing. Create the lock in a temporary npm project with a fixed package name and version and private: true. Save the selected packages and their registry checksums. | Preserve the scheduled discovery check; review its intentional moving inputs separately from release builds. |
| Caveman experiment | Install the committed CLI 2.1.0 lock with `npm ci --ignore-scripts`, then retain the entrypoint digest and signed engine checks. | Review the fixed lock and experiment scope. Debian package installation still uses current repository packages. |

These checks provide evidence for reviewing alerts, not records of dismissed alerts. For each dependency alert, record a confirmed fix or a reviewed exception backed by a fresh scan.

## Update the release npm toolchain

`scripts/npm-toolchain-pin.json` records the npm version, archive URL, and SHA-512 integrity value together. The current 11.5.1 values come from [official npm registry metadata](https://registry.npmjs.org/npm/11.5.1). Update all three values together after checking the new version's official metadata and verifying the downloaded archive. The release jobs do not fetch a replacement checksum at runtime.

`scripts/npm-toolchain.sh` verifies the complete archive before passing it to the installer. It uses separate empty npm configuration files and an empty installation cache, preserves the global installation directory, and checks the installed npm version from that directory. The helper receives no publication token; only the later Publish step receives it.

## Replay a vendor load check

The Tool load workflow uploads `tool-load-dependencies`, containing `package.json`, `package-lock.json`, `selected-packages.txt`, and `environment.txt`. The lock records the complete selected npm dependency graph and its registry integrity values. Integrity checks downloaded bytes against the recorded lock; they do not verify publisher identity or source provenance.

To replay the dependency graph from a downloaded artifact with an existing agnostic-ai binary:

```sh
TOOL_LOAD_LOCK_DIR=/absolute/path/to/tool-load-dependencies \
  scripts/tool-load.sh --bin /absolute/path/to/agnostic-ai codex gemini opencode
```

This uses `npm ci` with the saved manifest and lock. Keep the same operating system, Node/npm versions, source revision, and selected tools when comparing results. Vendor commands can still depend on external services or configuration outside the npm graph.

To test a particular release without a saved lock, set `TOOL_LOAD_CODEX_VERSION`, `TOOL_LOAD_GEMINI_VERSION`, or `TOOL_LOAD_OPENCODE_VERSION` to an exact version. Unset variables follow current releases. Exact release selection alone does not freeze transitive dependencies. `TOOL_LOAD_REPORT_DIR` chooses where a run saves its dependency evidence.

The [npm ci contract](https://docs.npmjs.com/cli/v11/commands/npm-ci/) requires a matching manifest and lock. The [Gradle checksum guidance](https://docs.gradle.org/current/userguide/gradle_wrapper.html#sec:verification) covers wrapper and distribution verification. See the [runtime experiment](rtk-caveman-runtime.md) for its separate engine verification and reproducibility limits.
