# Shipped built-ins

Use this for every target audit. All paths are repository-root relative.

Built-ins are specs that agnostic-ai ships inside the binary, such as the handoff skill and its hooks. They depend on native behavior beyond a declared capability: a hook needs the event to fire at the right time, its launcher to run, and its output to reach the right place. A target can keep `KindHook` in `caps.Supports` and still break a built-in.

## Repository evidence

Run `scripts/target-facts.sh --builtins` once and save its compact source inventory as `builtins.txt` in the run directory. Pass that path to each batch. Its metadata is evidence to inspect, not a computed support verdict.

- Registration and specs: `internal/builtins/builtins.go` and `internal/builtins/data/<built-in>/`. Read selectors, exclusions and `x-<target>` overrides. Confirm selection through `internal/spec/spec.go`, the adapter and emitted output; never copy a target table into the audit instructions.
- Docs: the matching pages under `docs/site/content/docs/`. The current handoff built-ins share `handoff.md` and `configuration.md`.
- Behavior: `internal/builtins/builtins_test.go`, `internal/cli/builtins_test.go`, `tests/integration/builtins_test.go` and `tests/integration/builtin_handoff_hooks_test.go`, with their fixtures under `tests/integration/fixtures/builtin-*`.

## Coverage since the last audit

Use the audited commit from a completed report covering the requested targets as `--builtins-since <rev>`. A date, vendor lock update or partial report cannot establish that baseline. Without a usable revision, `--changed` keeps the requested targets deep.

The `builtin-deep:` line names a reason for the numbered batches, not an extra assignment. Changes to built-in specs, assets or shared behavior conservatively invalidate the requested targets; adapter changes invalidate that adapter's requested target. This covers committed, staged, unstaged, untracked, removed and renamed paths. These targets cannot take either the unchanged-vendor or Jev-clear fast path. Other targets retain the vendor classification.

Give each affected batch the baseline and changed paths: `git diff --no-renames --name-only <rev>` plus `git ls-files --others --exclude-standard`, so untracked assets count. Read the previous blob when a spec, selector or registration was removed. Recheck relevant unchanged vendor pages against the changed repository behavior.

## What to check per target

For a skill: the tool still loads it from the path we write, accepts the emitted frontmatter and invokes it as documented. Check supported project and global installations in isolated temporary sources. Global paths that agnostic-ai already emits are in scope; unrelated user preferences remain outside it.

For a hook, check each item against the target's own vendor page:

- Event timing: resolve native overrides, then verify the event name, lifecycle moment and payload against emitted output. A portable event name is not a native event claim.
- Execution: whether the tool waits for the hook or runs it asynchronously, and its timeout.
- Launcher: the shell or interpreter the tool uses, and how the command string is passed.
- Output routing: where stdout and stderr go, and whether a JSON reply is required, parsed, or shown to the user or the model.
- Configuration scope: check the supported project and global files we write, their precedence, the launch directory and project-root discovery. Verify session identity and isolation between projects.
- Required settings: any flag, feature gate, or trust prompt that must be set before the hook runs.

## Classify

- **Built-in drift**: a target the spec already emits to no longer behaves as the built-in needs. File it as a finding with the usual severity: `breaking` when the hook or skill no longer runs, `degraded` when it runs with lost output or wrong timing.
- **Coverage opportunity**: a target the spec does not list now documents compatible native support. Report it under Cross-target opportunities with the vendor quote for each item above. It is not drift and not a fix bucket on its own.

## Fixes

Adding a target to a built-in requires that target's own vendor evidence for every hook item above and a behavioral test in `tests/integration/` that runs the emitted hook or loads the emitted skill. Hook support in `caps.Supports` alone never enables a target. Never infer one target's event semantics from another's.
