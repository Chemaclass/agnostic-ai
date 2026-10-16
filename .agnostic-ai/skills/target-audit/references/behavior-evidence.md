# Confirm behavior before filing

Use this contract for drift claims about permission boundaries, overlapping
rule order or precedence, path discovery, and import fidelity. All paths are
repository-root relative. Keep one evidence record in the run directory;
link it from the auditor's finding and the report. Copy its relevant evidence
into a filed issue so an ignored local file is never the only record.

## Reproduce the claim

Reuse the run's one current built agnostic-ai binary and record its path,
audited commit, worktree changes, and hash. Use isolated temporary projects
with only the settings needed for the claim. Save fixture contents, commands,
emitted bytes, and expected and observed results. Do not run broad tests,
build a generic harness, or execute every vendor tool during research.

Choose the smallest check that distinguishes the claimed behavior:

| Claim | Check |
|---|---|
| Permission boundary | Exercise the affected operation and resource with the project and agent rules in scope. Distinguish allow, ask, and deny; a config that parses does not prove a boundary holds. |
| Order or precedence | Use overlapping rules with different outcomes. Trace load, merge, translation, and evaluation order, including defaults, rather than inferring behavior from file order. |
| Path discovery | Check the exact emitted path and the claimed project, nested-directory, or global scope. An existing file does not prove the tool discovers it. |
| Import fidelity | Start with hand-written native config, then import, sync, and inspect the result. Run check and lint, and compare meaning, including ordered rules and preserved native fields. A clean round trip alone does not prove runtime semantics. |

Capability opportunities keep the existing representation test in
`.agnostic-ai/skills/target-audit/references/capability-intelligence.md`.
URL corrections and source-only documentation changes need source evidence,
without artificial executable proof. If an opportunity also alleges drift,
apply this contract to that drift claim.

## Record one outcome

- `native-runtime`: reproduce the behavior using the actual vendor binary.
  Record tool version, resolved binary path and hash (or package identity),
  isolated fixture, working directory, invocation, relevant scope and
  environment, expected result, and observed result. Name what was exercised:
  a native parser, evaluator, discovery command, or actual operation. A parser
  invocation alone cannot prove permission enforcement.
- `source-evaluated`: reproduce current repository output, then evaluate it
  against pinned vendor source or schema. Pin the vendor commit and released
  version, show their relationship with release/package evidence, and trace
  the released runtime's relevant entry point through loading, translation,
  and evaluation. Record evaluator or validator identity, command, inputs,
  result, and omitted runtime behavior. A hand-written approximation is not
  the vendor evaluator. A schema validator can establish schema conformance;
  it cannot prove evaluation order, permissions, or path discovery. Confirm
  only the part this evidence establishes and state that no native runtime
  operation was observed.
- `unconfirmed`: the fixture cannot reproduce the claim, the runtime is
  unavailable without sufficient released-source evidence, or scope, version,
  or semantics remain unresolved. Record the missing evidence and the exact
  question or command that would settle it. Keep unresolved claims in
  `Needs a human`; do not file them as confirmed bugs or assign a fixer.
  Discard disproved leads and record why they failed instead.

Do not turn an unavailable proprietary runtime into a requirement to install
or execute it. Released-source evaluation can confirm a bounded claim when
it covers the behavior in question; unsupported extrapolation stays
unconfirmed.

## Independent check

Before accepting a high-risk behavior claim, have a checker other than its
author challenge the fixture, exact emitted bytes, vendor version and scope,
and semantic conclusion. Give the checker the record and cited sources;
ask for a counterexample, missing load or migration step, and a mismatch
between the tested operation and the claimed effect. The orchestrator can
check an auditor's claim independently. Record the checker, evidence inspected
or rerun, verdict, and unresolved questions. A checker need not run an
unavailable proprietary tool. Agreement alone adds no vendor evidence.

This check applies without `--compare-models`; that optional pass has its own
scope in the capability reference. A missing checker or unresolved substantive
objection keeps the claim unconfirmed. Absence and removal claims still need
the independent fetch route required by synthesis, even after a behavior check.

## Evidence record and issue template

Include this record with the normal vendor quote, repository contradiction,
user loss, and smallest fix. Fill applicable fields; explain exclusions.
For source-only corrections, omit this record and explain why the behavior
contract does not apply, rather than inventing a runtime outcome.

```markdown
### Confirmation
- confirmation: native-runtime | source-evaluated | unconfirmed
- claim-and-scope: <operation, resource/path, project/agent/global scope>
- repository: <audited commit, worktree changes, current binary path and hash>
- fixture: <minimal input files and relevant environment>
- reproduction: <working directory and exact commands>
- emitted: <relevant exact output bytes>
- expected: <result supported by the cited vendor evidence>
- observed: <actual result and saved output>
- vendor: <version, binary/package identity; or pinned source/schema and release relationship>
- evaluation: <native entry point or evaluator/validator, invocation and traced semantics>
- limits: <what was not exercised or proved, including no native operation for source evaluation>
- independent-check: <checker, inspected/rerun evidence, challenge, verdict>
- missing-evidence: <None, or settling question/command>
```
