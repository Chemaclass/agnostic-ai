<a id="rtk-and-caveman-runtime-composition-issue-1960"></a>

# Running RTK and Caveman together, issue #1960

This local experiment tested RTK 0.51.0 with Caveman CLI 2.1.0 and its signed `bin-v2.1.0` engine in a Linux arm64 Debian trixie container. The [runner](../../scripts/rtk-caveman-runtime.py) generates fixed test output, runs each command once, and writes [machine-readable results](rtk-caveman-runtime-results.json). The [Linux installation records](rtk-caveman-runtime-linux-provenance.json) list the pinned image, release checksums, signed installer result, and unchanged inherited home. The runner uses temporary stores for command history and retrieving saved output, passes no provider credentials to either tool, and never starts a proxy or coding agent.

## Recommendation

Keep RTK as the command-output filter and keep Caveman command shrinking off in the initial project recipe. For the ordinary failing test fixture, RTK reduced 23,865 original bytes to a 240-byte summary, and Caveman left that summary unchanged. A constructed case with four repeated 1,000-character success lines in RTK's failure tail gave Caveman more to remove, reducing 4,248 RTK-summary bytes to 2,215 bytes. The one-command `tools shrink -- rtk test ...` path emitted 2,367 bytes after adding its recovery status line. That proves a two-step output reduction can run, not that a second stage helps typical coding tasks. It adds a second recovery store and another failure path. The task-quality and provider-cost effect is unmeasured.

If a user deliberately combines the tools for a supported command, use RTK's command wrapper with its SQLite recall store enabled. Preserve the RTK recall hint through any Caveman transform and keep Caveman's recovery store available. Do not use `rtk pipe` as a substitute for this recovery path: its filtered text alone cannot restore removed command output. The tested `caveman tools compress` entry point exits 1 with no compact output when its recovery-store parent is unavailable, so a caller must retain the RTK summary on error. The command-running `caveman tools shrink -- ...` path kept the RTK summary and exit status in the same store-failure case. These are distinct interfaces and need separate handling.

The runner also sent a 21,219-byte Rust test transcript to `rtk pipe --filter cargo-test`. It returned 239 bytes with the failure path, no recall hint, and exit 0 because it filtered stdin rather than running the failing test command. A shell pipeline must preserve the original command's exit status separately. This input-filter path is useful for a known disposable transcript; it does not provide the two-store requirement for retrieving removed output above.

<a id="data-flow-and-exact-recovery"></a>

## Output at each step and retrieving the original

| Case | Output seen by the model | Output returned by each retrieval | Failure and exit |
| --- | --- | --- | --- |
| Ordinary failure | Original 23,865 bytes → RTK 240 bytes → Caveman 240 bytes | `rtk recall --full` returned the exact 23,865-byte original; Caveman created no retrieval reference | RTK exit 7; error and path visible |
| Repeated tail | Original 27,975 bytes → RTK 4,248 bytes → Caveman 2,215 bytes | `caveman tools retrieve` returned the exact 4,248-byte RTK summary; `rtk recall --full` separately returned the exact 27,975-byte original | RTK exit 7; error, path, and RTK recall hint visible after both steps |

Each command wrote a counter once. Retrievals, including attempts against the wrong stores, left the counter at one. The Caveman retrieval reference identifies only the bytes Caveman received. It did not recreate the original output before RTK filtered it. The runner records SHA-256 digests for originals and transformed outputs so a repeat can compare bytes without storing a large fixture in Git.

<a id="failure-and-no-op-cases"></a>

## Failures and unchanged output

- A wrong Caveman store could not retrieve the RTK summary. A wrong RTK store could not retrieve the original. Neither command was run again.
- A missing Caveman engine passed the RTK summary through unchanged. A six-byte input also passed through unchanged.
- With an unavailable Caveman recovery-store parent, `tools compress` exited 1, wrote no compact output, and named the store error. `tools shrink -- rtk test ...` still ran the command once, retained exit 7, and passed the exact RTK summary through.
- With RTK recall disabled for the child process through `RTK_RECALL=0`, RTK still ran the failing command once and returned a filtered response without a recall hint. That mode cannot support a claim that all removed bytes remain recoverable.
- A missing RTK executable under `tools shrink -- ...` failed without running the target command. The error was not treated as a successful run.
- A combined `tools shrink -- rtk test ...` wrapper ran the harmless failing command once, returned exit 7, and kept `tests/payment_test.go:42` plus the expected/actual values visible.
- The same wrapper on the repeated-tail case reduced the RTK summary, preserved both recovery hints, returned exit 7, and retrieved the exact RTK summary without rerunning the command. Its appended status line explains the difference between its 2,367 output bytes and the direct compressor's 2,215 bytes.

The local runner checked immediate retrieval only. [RTK 0.51.0 documents](https://github.com/rtk-ai/rtk/blob/v0.51.0/docs/guide/getting-started/configuration.md) default SQLite recall limits of 10 MiB per entry, 200 entries, and 30 days. Store loss, eviction, or a different store makes a prior hint unusable. [Caveman's recovery documentation](https://github.com/JuliusBrussee/caveman/blob/main/docs/technical/context-recovery.md) likewise says a handle needs the same local store and is not an archive.

## Reproduce

Run these commands from the repository root with Python 3 and Docker available. The [asset helper](../../scripts/rtk-caveman-runtime-linux/fetch-assets.py) downloads official pinned release metadata and assets to scratch, checks required assets and SHA-256 digests, and uses 60-second network timeouts. The [container helper](../../scripts/rtk-caveman-runtime-linux/setup-linux.sh) refuses execution outside a Linux arm64 Docker container running Debian trixie. It installs dependencies in the disposable container and Caveman under the mounted scratch directory. The CLI dependency graph comes from the committed npm lockfile and installs with `npm ci --ignore-scripts`. Debian package installation still selects current repository packages; engine verification remains separate from the npm lock. The unchanged upstream installer verifies Caveman's signed checksum manifest, its release binding, and binary checksums. No host home or provider configuration is mounted.

```sh
linux_lab=$(mktemp -d)
python3 scripts/rtk-caveman-runtime-linux/fetch-assets.py --output "$linux_lab"
cp scripts/rtk-caveman-runtime.py "$linux_lab/rtk-caveman-runtime.py"
cp scripts/rtk-caveman-runtime-linux/setup-linux.sh "$linux_lab/setup-linux.sh"
cp -R scripts/rtk-caveman-runtime-linux/caveman-cli "$linux_lab/caveman-cli"
docker run --rm --platform linux/arm64 \
  --env RTK_CAVEMAN_CONTAINER=1 \
  --mount "type=bind,src=$linux_lab,dst=/lab" \
  node@sha256:154ba2f4d6fec323d28e4f4bb86bba4677f1223391a1979cf521304e03a98dfa \
  sh /lab/setup-linux.sh
```

Results are written to `$linux_lab/rtk-caveman-runtime-linux-results.json` and `$linux_lab/linux-provenance.json`. The tracked results came from this Linux runtime, with Python 3.13.5 and glibc 2.41. A fresh scratch directory and signed installation through the documented helpers reproduced the tracked results byte for byte. The installation records describe that fresh installation. An initial Debian bookworm attempt failed because the official RTK Linux archive requires glibc 2.39; the pinned trixie image supplies a compatible version.

For an existing local installation, pass absolute paths and write results to a separate file. The Caveman CLI argument names its installed `dist/index.js`, and the Node argument names a real executable rather than a version-manager shim. The runner reads RTK's current recall mode and requires SQLite without changing that configuration. On macOS, RTK 0.51.0 reads its configuration under the caller's home even when `XDG_CONFIG_HOME` is set. If that configuration selects another recall mode, the runner stops; use the container recipe above. History and recovery stores are redirected explicitly. The disabled-recall case uses `RTK_RECALL=0` in the child process.

```sh
python3 scripts/rtk-caveman-runtime.py \
  --rtk /absolute/path/to/rtk \
  --node /absolute/path/to/node \
  --caveman-cli /absolute/path/to/@caveman-ai/cli/dist/index.js \
  --caveman-engine /absolute/path/to/caveman-engine \
  --output /absolute/scratch/path/rtk-caveman-runtime-results.json
```

The result applies to these synthetic terminal fixtures and local command interfaces. It does not establish proxy routing, real coding-agent behavior, provider input reduction, task quality, or billed savings. The project setup remains independent: agnostic-ai writes configuration, RTK owns command filtering and its recall store, and Caveman owns its runtime transform and recovery store.
