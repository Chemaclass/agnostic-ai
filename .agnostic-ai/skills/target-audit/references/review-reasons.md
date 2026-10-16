# Review reasons

Save the work behind the assignment before delegating:

```bash
scripts/target-facts.sh --review-reasons <run>/docfetch.tsv --builtins-since <rev> > <run>/review-reasons.tsv
```

Use the same run file and built-in baseline as `--changed`. Omit `--builtins-since` when no completed audit proves a baseline. This output explains the existing deep batches. It changes no assignment, required page coverage or fast-path eligibility, and establishes no speedup.

The TSV has `target`, `reason`, `baseline`, `evidence`, `state` and `checklist` columns. A target can have several work items. Pass each batch only its targets' rows, together with `builtins.txt`. The rows identify required work; they do not track its completion or prove native support.

| Reason | Evidence and required work |
|---|---|
| `vendor-delta` | A new or changed vendor URL. `baseline` is the lock's previous content hash, or `-` if absent. `vendor-review`: read the delta and relevant current text using the auditor's normal method. |
| `built-in` | A shipped built-in path, including assets and registration. `baseline` is the resolved repository commit. `built-in-review`: apply the existing skill and hook checklist in `.agnostic-ai/skills/target-audit/references/builtins.md`. |
| `shared-repository` | A shared runtime, selection, config, emission, test or documented claim, or a target's own adapter or page. `target-claims`: trace the changed repository claim to current vendor evidence. Also complete `built-in-review`. |
| `source-recovery` | A vendor row whose resolved status is neither unchanged nor a new or changed source, or a requested target with no fetched rows. `baseline` is the previous locked hash if available. `recover-source`: use `.agnostic-ai/skills/target-audit/references/fetch-playbook.md`; record unresolved access as a research limit. |

A target listed in `requested-targets.txt` with no fetched rows gets `evidence=requested-targets.txt`, `state=missing-source` and `baseline=-`. Recover the missing fetch coverage before capture or finalization. Normal batching keeps that target deep.

Repository ownership follows the current classifier: a registered adapter package or target page belongs to its requested target; shipped built-ins, shared paths and unknown adapter packages belong to every requested target. Selector exclusions never narrow these assignments. No supported-target table is maintained here.

For repository rows, `state` is `added`, `changed` or `removed` relative to the baseline and current worktree, including staged and untracked inputs and ignored assets the binary embeds. Compare the baseline and current file for every changed spec, including removed selectors, exclusions, events, native overrides and registration entries. `previous-blob` calls out a removed path: inspect `git show <baseline>:<path>` before deciding who previously received it. Renames retain both paths. Backslashes, tabs, newlines and carriage returns in repository paths use `\\`, `\t`, `\n` and `\r` escapes so evidence stays on one TSV row.

A missing or unusable baseline produces `built-in` rows with `evidence=baseline-unavailable`. Their state is `missing`, `invalid` or `compare-failed`. `baseline-recovery` means recover a qualifying baseline under the built-ins reference or complete the full built-in review for every requested target. A vendor lock update cannot establish that baseline.

Keep vendor and repository evidence separate in the assignment and result. Unchanged vendor text clears no pending built-in or shared repository check. A target with pending built-in checks cannot be Clean or take either fast path. Any future coverage reduction needs validated completion accounting and demonstrated equivalent coverage in a separate change.
