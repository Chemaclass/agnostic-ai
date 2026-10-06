# Shipped built-ins

Read this when a batch includes a target that emits a shipped built-in, or when a vendor change touches skills, hooks, session events, or command launchers. All paths are repository-root relative.

Built-ins are specs that agnostic-ai ships inside the binary, such as the handoff skill and its hooks. They depend on native behavior beyond a declared capability: a hook needs the event to fire at the right time, its launcher to run, and its output to reach the right place. A target can keep `KindHook` in `caps.Supports` and still break a built-in.

## Repository evidence

The built-in specs are the support list. Never copy a target table into this skill or a report.

- Specs: `internal/builtins/data/<built-in>/`. A spec's `target:`, `targets:`, `target-exclude:`, and `targets-exclude:` select where it emits, as for any spec; with no include list it emits to every target that supports its kind. `x-<target>` blocks carry per-target settings, such as a different event name.
- Per target: the `--- shipped built-ins this target emits ---` section of `scripts/target-facts.sh <target>` lists each spec, its event, and its overrides.
- Docs: the pages under `docs/site/content/docs/` that name the built-in or its specs (for handoff, `handoff.md` and `configuration.md`).
- Behavior: `tests/integration/builtin*_test.go`, their fixtures under `tests/integration/fixtures/builtin-*`, and `internal/builtins/builtins_test.go`.

## What to check per target

For a skill: the tool still loads project skills from the path we write, with the frontmatter we emit, and invokes them the way the docs promise.

For a hook, check each item against the target's own vendor page:

- Event timing: the native event still exists under that name and fires at the moment the built-in assumes (before compaction, at session end, at session start).
- Execution: whether the tool waits for the hook or runs it asynchronously, and its timeout.
- Launcher: the shell or interpreter the tool uses, and how the command string is passed.
- Output routing: where stdout and stderr go, and whether a JSON reply is required, parsed, or shown to the user or the model.
- Configuration scope: the project file we write is the one the tool reads, and its precedence over user scope.
- Required settings: any flag, feature gate, or trust prompt that must be set before the hook runs.

## Classify

- **Built-in drift**: a target the spec already emits to no longer behaves as the built-in needs. File it as a finding with the usual severity: `breaking` when the hook or skill no longer runs, `degraded` when it runs with lost output or wrong timing.
- **Coverage opportunity**: a target the spec does not list now documents compatible native support. Report it under Cross-target opportunities with the vendor quote for each item above. It is not drift and not a fix bucket on its own.

## Fixes

Adding a target to a built-in requires that target's own vendor evidence for every hook item above and a behavioral test in `tests/integration/` that runs the emitted hook or loads the emitted skill. Hook support in `caps.Supports` alone never enables a target. Never infer one target's event semantics from another's.
