# Audit evidence and output contract

Coordinator and auditors read this contract before research or synthesis. Paths are repository-root relative. The auditor owns per-target evidence and records; the coordinator owns independent acceptance, coverage finalization, and publication.

## Evidence rules

A reportable finding requires:

- a vendor URL **and** the quoted sentence that proves the claim,
- the `file:line` in this repo that contradicts it,
- what a user loses today (not "we could also support X"),
- the smallest fix.

Do not report features behind a waitlist or an unreleased beta, unrelated
user preferences, stylistic or preference-only wording changes, or anything
you could not open with your own tools. Current vendor evidence contradicting
repository documentation can establish a `cosmetic` finding when the impact
is wrong user guidance. It still requires the vendor quote, repository line,
concrete impact, and smallest fix above. Existing global built-in output is
covered by the built-ins reference.

A capability signal is not a finding and does not need a contradicting
`file:line`. It does require a vendor URL with a quoted sentence, a
released project scope, concrete user value, and the repository locations
checked for an existing representation. Do not turn model agreement,
marketing language, product rankings, or an undocumented analogy into a
signal.

## Auditor output

Each auditor writes one markdown report per target at `<run>/<target>.md` and checkpoints it as `.agnostic-ai/agents/target-auditor.md` describes. Its reply to the coordinator is at most 20 lines of artifact paths, per-target outcomes and blockers; agent messages can be truncated, so the coordinator reads findings from the reports and checks these records before synthesis. In each report, first write one
block per finding, most severe first. Keep the existing finding schema
unchanged:

```
### <target>: <one-line claim>
- kind: path-drift | new-surface | schema-drift | deprecation | docs-moved | unconfirmed
- severity: breaking | degraded | missing-feature | cosmetic
- evidence: <url> : "<quoted sentence>"
- ours: <file:line> : <what we currently do>
- impact: <what a user of this target loses today>
- fix: <the smallest change that closes it>
```

Link each behavior claim to its run-directory confirmation record using
the template in the behavior-evidence reference. Keep the finding block's
schema above unchanged.

Then report capability signals in a separate section:

```
## Capability signals

### <stable semantic name>
- signal-id: cap-<stable-lowercase-slug>
- targets: <independently verified targets only>
- evidence: <target> <url> : "<quoted sentence>"
- observed: <release date or date the source was checked>
- availability: stable | preview | experimental, plus any access gate
- project-scope: <native project path or project behavior>
- user-value: <what a team can now configure or automate>
- ours: <repository paths inspected and what they model today>
- representation-test: <reproduction and result, or why no executable test applies>
- semantics: <per-target similarities and differences, without inference>
- suggested-disposition: adapter-gap | spec-candidate | target-extension | watch
- confidence: confirmed | conflicting
```

Use `None` when the batch produced no capability signals. The coordinator owns the final cross-target disposition. For signals, both roles read
`.agnostic-ai/skills/target-audit/references/capability-intelligence.md`
for the detailed signal contract.

End with:

```
### Clean
- <target>: verified against <url> (<date on the changelog entry you read>)
- <target>: fast path, docs and changelog unchanged since <lock changed date>, changelog <url> read
```

Resolve every assigned review item before marking a target Clean. Pending built-in checks block Clean even when vendor text is unchanged.

Use the first row for a target you audited deeply and the second for one
whose rows were all `unchanged`. The orchestrator copies the marker into
the report so a human can see which targets got a hash-only sweep.

Severity: `breaking` means we emit to a path the tool no longer reads.
`degraded` means it still works, but a documented better native feature exists.
`missing-feature` means a native feature exists that we skip with a warning. `cosmetic`
means docs only.
