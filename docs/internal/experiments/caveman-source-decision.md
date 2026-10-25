# Caveman source maintenance

**Recommendation:** Keep the built-in and example pack, with the existing parity check. Their duplicated source is seven files, 27,837 bytes per tree. Removing the built-in would lose working global setup and a required YAML switch. Generating both copies from one source remains an option if updates become frequent; this investigation does not justify adding that maintenance step today.

**Update, 2026-10-25:** The maintainer decided that Caveman ships as `SKILL.md` alone. The upstream license, notice, and readme files are removed from the repository, from both the built-in and the example pack, and both `UPSTREAM.json` manifests keep only the `SKILL.md` entry. People asked for no such files in their projects, and the skill works without them; the license and notices stay in the [upstream repository](https://github.com/JuliusBrussee/caveman). A sync after upgrading removes those files where an earlier sync owned them, and leaves same-named files a person wrote. The seven-file inventory, replay, and checks below describe the investigation before this change.

**Status:** Proposed maintainer decision, 2026-10-25. [Issue #2458](https://github.com/Chemaclass/agnostic-ai/issues/2458). Checked source: `4621c815`. The investigation itself changed no shipped behavior or source assets; the update above records the later output change.

## Contract and inventory

[Issue #1964](https://github.com/Chemaclass/agnostic-ai/issues/1964) requires separate, default-off `builtins: [rtk, caveman]` switches. Caveman supplies the default response skill without installing a binary, downloading content, or enabling runtime compression. A person invokes the skill. The example packs from [#1963](https://github.com/Chemaclass/agnostic-ai/issues/1963) remain examples, not replacements for that switch. [#1957](https://github.com/Chemaclass/agnostic-ai/issues/1957) records the delivery decision.

The copies are [embedded assets](../../../internal/builtins/data/caveman/) and the [example pack](../../examples/rtk-and-caveman/packs/caveman/). Both hold `UPSTREAM.json` and `SKILL.md`. Their current upstream revision is `2e08b9177c07bb7249a8a2d1a6758e5db281d002`. The manifest records the source URL and content hash of `SKILL.md`.

`TestToolBuiltins_PreservePinnedPackSources` compares every example file against the materialized built-in, verifies the manifest hash and the expected revision, checks that the skill folder holds only `SKILL.md`, and checks that Caveman loads as one skill. A pin update means reviewing the upstream skill, replacing `SKILL.md` in both trees, updating the manifest and expected revision, and running this test and the built-in lifecycle checks. Maintainers review and ship that update in an agnostic-ai release.

## Options

| Choice | Maintenance and reproducibility | Install, update, remove, and overrides | Offline and global use |
|---|---|---|---|
| Keep both copies and parity checks (recommended) | Review one upstream revision, update both trees and manifest, then run parity checks. Existing checks catch a changed example file or digest. | Existing YAML switch stays. Removing it removes owned outputs at the next sync. Project specs override the built-in. Example-pack users keep their existing pack owner. | Built-in is embedded and works offline in project and global sync. A copied pack also syncs offline after installation. |
| Generate both from one pinned source | Keep one complete asset tree and manifest. Add a deterministic generation command, a stale-output check, and contributor/release instructions. Review generated differences on every pin update. | Keep the YAML switch, output paths, notices, override order, and removal rules byte compatible. No user migration is needed if generated assets stay identical. | Generate during development or release, never fetch at sync time. Both embedded and example outputs remain available offline; global behavior stays. |
| Replace the built-in with a maintained pack | Choose a pack repository, maintainer, release/pin policy, notices, and distribution checks. Existing pack installation can pin a tag and record its resolved commit, but has limits below. | Requires approved compatibility and ownership migration. Users gain explicit pack installation/update/removal steps; their project specs must still win. | Initial remote install/update needs its source. Installed copies sync offline. Current pack commands cannot manage the global home, so this route cannot replace the whole built-in contract today. |

## Local proof

The [replay script](caveman-source-proof.py) runs entirely in a new output directory. It serves a copied pack through a Git daemon bound to localhost to exercise the real Git installation path. User home, cache directories, `AGNOSTIC_AI_HOME`, and `CLAUDE_CONFIG_DIR` point into the fixture. Git uses an empty fixture configuration instead of user or system settings. It makes no vendor requests or model calls.

```sh
python3 docs/internal/experiments/caveman-source-proof.py \
  --agnostic /absolute/path/to/current/agnostic-ai \
  --output /tmp/caveman-source-proof-new
```

Use a binary built from the checked source and an unused output path. Python 3 and Git are required. The script stops the temporary daemon and retains `evidence.json` with exact commands, statuses, output, and checks. The [recorded result](caveman-source-results.json) records 25 passing checks at `4621c815`, the binary hash, build command, and toolchain. Only the documentation and replay files were changed when building; native source and embedded inputs match that revision.

The replay establishes these boundaries:

- Both source trees match, all manifest hashes match, and the emitted skill keeps its full instruction body and all five companion files unchanged. Native frontmatter and the generated source header are tool-specific.
- A local-directory pack has no Git pin. A tagged Git pack records its ref and resolved commit. Repeat sync and update at the unchanged tag retain the lock and output bytes. A raw commit passed as the ref fails because this path uses `git clone --branch`; do not describe that spelling as supported. `packs update` follows the recorded ref, so a moved tag is not an immutable pin to the previously recorded hash.
- Removing the pack and syncing disables its owned skill. Packs have no separate enable/disable switch. A same-name pack and built-in emit one project skill, but removing the pack reveals the still-enabled built-in. Remove both suppliers to remove that output.
- A user-authored project source skill wins over both suppliers and survives their removal. Global built-in sync repeats cleanly and removes owned output when disabled. A different unmanaged global skill blocks enablement and remains byte identical.
- `packs add`, `packs update`, and `packs remove` each refuse the temporary global home. No global pack or lock is created. Global sync does not load project pack layers. Adding global pack support is outside this investigation.

Validation: the replay passed 25 checks on macOS arm64 with Go 1.27.3 and Python 3.9.6. Asset parity and built-in lifecycle checks also passed. These checks cover generated configuration, not activation inside a live coding tool.

## Ownership and approval

Keep one owner per skill and scope. Project and global copies are separate installations; same-name project layer precedence does not prove that a coding tool ignores a second global copy. Existing [owner-choice and removal instructions](../../examples/rtk-and-caveman/README.md#keep-or-transfer-existing-ownership) require moving a manually installed skill outside its discovery directory before transferring it. Preserve its full directory and restore it only after the generated copy is removed. Global sync can adopt byte-identical unmanaged output, after which disabling the supplier removes it; identical content is not proof that a user intended ownership transfer.

Any replacement proposal must explicitly inventory project and global owners, preserve authored overrides and license assets, prevent a second discoverable copy, define rollback and removal, and provide a working global route. It must also resolve immutable pinning and the pack's maintainer/update policy. No automatic ownership transfer or migration is approved here.

Chemaclass, the maintainer who required the built-in switch, must approve any change to that product contract. The remaining decision is whether the small current duplication merits a generation step. There is no accepted migration decision and no implementation follow-up is opened by this investigation.
