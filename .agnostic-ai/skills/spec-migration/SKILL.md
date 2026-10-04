---
name: spec-migration
description: Decide whether a change to the spec format needs a migration, and write it so `agnostic-ai migrate` rewrites old specs safely. Use when a change renames, replaces, deprecates, or adds a preferred form for any field users write under .agnostic-ai/ or in agnostic-ai.yaml.
---

# spec-migration

A spec migration rewrites a user's old spec form into the new one without changing what sync writes. `agnostic-ai migrate` runs every pending migration from one registry (#1755). Users should never have to rewrite specs by hand after an upgrade.

## When a change needs one

Ask this for every change to a spec kind, a frontmatter field, a YAML key, or `agnostic-ai.yaml`:

| The change | What to ship |
| --- | --- |
| Renames a field, or adds a new preferred form for the same meaning | A migration, and keep the old form accepted |
| Deprecates a form that has an exact replacement | A migration, plus a lint warning that names it |
| Removes a form | Only in a major release, after a migration shipped at least one minor release earlier |
| Changes what sync writes for an unchanged spec | Not a migration: a behavior change with a CHANGELOG entry and, if opt-in, a lint note |
| A style preference with no change in meaning or output | Nothing, or a lint note. Never a migration |

If the old form cannot map one to one, the migration rewrites only the entries that do and leaves the rest with a reason.

## Rules

1. **Output stays the same.** `sync`, then `migrate`, then `sync --check` exits 0. If a rewrite would change a synced file, it is not a migration.
2. **Old form keeps working.** Never break a spec that still uses it. Removal waits for a major release.
3. **Idempotent and stateless.** A migration detects its own old form. Running it twice changes nothing.
4. **Touch only what you own.** Use the shared editor that works on parsed nodes, so comments, key order, quoting, and bodies stay as written.
5. **Explain every skip.** Each entry left alone gets a one-line reason in `--list` and `--dry-run`.
6. **Never print a secret.** Name the spec, field, and key, never the value.
7. **Packs are read-only.** List the pack to update instead of rewriting it.
8. **Import writes the new form.** A project that starts after the change never needs the migration.

## Steps

1. Name the migration after what it does, such as `hooks-portable-events`, and record the release that adds it.
2. Write the fixture first: old-form specs with comments and odd formatting, and the expected rewrite.
3. Implement the rewrite as a pure function from a parsed spec to a rewritten spec plus skip reasons. Register it.
4. Add or update the lint warning for the old form, and point its message at `agnostic-ai migrate`.
5. Run the shared invariant test (`sync`, `migrate`, `sync --check` on the fixtures) and `migrate` twice to prove idempotence.
6. Make `import` write the new form.
7. Document the new form first on its spec-format page, keep the old form as an alias there, and add a CHANGELOG line that says "run `agnostic-ai migrate`".
8. In the PR body, state the migration ID, what it rewrites, what it skips, and the release where the old form may be removed.

## Until #1755 lands

The registry does not exist yet. A PR that changes a spec form still keeps the old form working, adds the lint warning, and lists the migration it needs on #1755, so the first registry PR picks it up.
