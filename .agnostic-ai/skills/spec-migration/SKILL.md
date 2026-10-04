---
name: spec-migration
description: Decide whether a change to the spec format needs a migration, and write it so `agnostic-ai migrate` rewrites old specs safely. Use when a change renames, replaces, deprecates, removes, or tightens any field users write under .agnostic-ai/ or in agnostic-ai.yaml.
---

# spec-migration

A spec migration rewrites a user's old spec form into the new one without changing what sync writes. `agnostic-ai migrate` runs every pending migration from one registry, `specMigrations` in `internal/cli/migrate.go` (#1755). Users should never have to rewrite specs by hand after an upgrade.

## When a change needs one

Ask this for every change to a spec kind, a frontmatter field, a YAML key, or `agnostic-ai.yaml`:

| The change | What to ship |
| --- | --- |
| Renames a field, or adds a new form that some or all entries map to one to one | A migration and a lint warning on the old form; the old form stays accepted |
| Deprecates a form that no entry maps to one to one | No migration: a lint warning that explains the manual rewrite |
| Adds a new optional field | Nothing |
| Rejects specs that used to load, where a rewrite that keeps their meaning makes them load again | A migration, plus a lint warning at least one release before the rejection |
| Changes a form's meaning or default | Not a migration: a behavior change with a CHANGELOG entry and a lint warning at least one release ahead |
| Removes a form | Only in a breaking release, after its migration shipped, no earlier than the migration's issue allows |
| A style preference with no change in meaning or output | Nothing, or a lint note. Never a migration |

While the project is 0.x, a breaking release is a minor release whose CHANGELOG has a breaking section.

## Rules

1. **Output stays the same.** `sync`, then `migrate`, then `sync --check` exits 0 for every target the spec already reached. The only exceptions are the ones the migration's issue names, such as a spec reaching new targets (which `sync` then lists) or a literal secret becoming a reference. Any other change to a synced file is not a migration.
2. **Old form keeps working until its removal release.** That release replaces it with an error that names the migration to run with the last version that has it. The migration and its fixture stay until then.
3. **Idempotent and stateless.** A migration detects its own old form. Running it twice changes nothing. The release it records is metadata only.
4. **Map one to one or skip.** Entries that do not map stay as written with a one-line reason that `migrate` reports. A spec that sets both the old and the new form is a skip, never a merge.
5. **Touch only what you own.** Edit parsed nodes, through the shared editor once #1755 provides it, so comments, key order, quoting, and bodies stay as written. Write atomically and keep the file mode.
6. **Never expose a secret.** Diffs, skip reasons, and errors show values of `env`, `headers`, URLs, and args as references or `<redacted>`. A migration never turns a reference into a literal and never moves a value into another file or into the global home.
7. **Stay inside the spec roots.** Resolve each file's real path. A file outside the project, `local/`, or global spec roots, such as a pack or a symlink into one, is a skip whose reason names the pack.
8. **Import writes the new form.** A project that starts after the change never needs the migration.

## Steps

1. Name the migration `<group>-<what>`, where `<group>` is the name `migrate --only` takes, such as `hooks-portable-events`. Record the next release version, the one the CHANGELOG's Unreleased section will become, as the one that adds it.
2. Write the fixture first, as a project under `internal/cli/testdata/migrate/<id>/`: old-form specs with comments and odd formatting, both-forms and unmappable cases, and the expected rewrite. Use placeholder values such as `${TOKEN}` or `REDACTED`, never a real credential.
3. Implement `Plan` in `internal/cli/migrate_<what>.go`: it reads the project and returns changes plus skip reasons, and never writes. Add it to `specMigrations` in release order.
4. Add or update the lint warning for the old form, pointing at `agnostic-ai migrate`.
5. Run `go test ./internal/cli -run Migrate`. The shared test runs `sync`, `migrate`, and `sync --check` on every registered migration's fixture, then `migrate` again to prove idempotence.
6. Make `import` write the new form.
7. Document the new form first on its spec-format page, keep the old form there as an alias, and add a CHANGELOG line that says "run `agnostic-ai migrate`".
8. In the PR body, state the migration ID, what it rewrites, what it skips, and the earliest release that may remove the old form.

## Until the shared editor lands

The format-preserving editor from #1755 does not exist yet. A migration that renames a file, or rewrites content byte for byte without reformatting, can ship now. One that must edit YAML nodes waits for the editor; until then, its PR keeps the old form working, adds the lint warning with the manual rewrite, and comments on #1755 (never an edit of its body) with the migration ID, fixture path, and mapping.
