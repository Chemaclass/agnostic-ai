# Triage and fast paths

Choose triage after classifying pending targets. Pass only pending deep targets using the script's target arguments; omit reused and unchanged-sweep targets. Skip triage when that list is empty. If every pending deep target is in `builtin-deep:`, use `scripts/jev-triage.sh --lexical <run> <pending-deep-target>...`. It produces lexical and trim-1 leads without API calls. Those targets require deep review regardless of Jev scores, so paid triage cannot reduce their required coverage. Record the selected mode in metrics; this saves triage requests, not a measured amount of review time.

Otherwise use `scripts/jev-triage.sh <run> <pending-deep-target>...`. Do not reduce coverage, alter thresholds, or claim a fast path from lexical leads. If Jev has no key or fails, keep its offline fallback and review every changed page.

Run triage before dispatch. The script always exits 0; inspect its output and metrics. Its `triage.tsv` columns are target, url, delta, claim, verdict, probability, p_contradicts, via. With `TYPESAFE_API_KEY`, Jev checks every target claim against each changed page and scores project-config additions (`noul`). Without a key or after API failure, lexical `paired` leads match paths, keys, or terms; unjudged page parts also get trim-1 top-five changed-line matches against the target's other claims. Trim needs Go; stderr records skipped reasons, method, and input tokens. Leads are questions, never findings. Empty leads never clear a page.

Prioritize targets with Jev leads or `noul >= 0.5`. Pass only each batch's leads. Jev keeps `contradicts` or `p_contradicts >= 0.4`. `triage-pages.tsv` statuses control eligibility:

- `clear`: all questions answered, no retained lead, `noul < 0.1`, and no emitted path moved.
- `lead`: retained lead.
- `judged`: no lead, but a `mentions:` page or `noul >= 0.1`.
- `unjudged`: capped or failed request.
- `lexical`: Jev did not run.

Deep review is the default. A Jev fast path requires every `new`/`changed` row to be `clear` and the target absent from `builtin-deep:`. First read its changed deltas against the built-ins reference's hook and skill checklist (event, launcher, output, project/global paths, root discovery), which Jev never sees. Any hit keeps deep review. Only then remove it from its batch and mark `fast path by Jev`. Any other status or missing row, including no delta or failed fetch, requires deep review. Without Jev, every changed row is read.

Randomly select one Jev-fast-path target for deep review and record whether it was clean. A finding disables that fast path until a replay case passes. After filing each confirmed finding or signal, add its exact vendor quote to the matching `triage-cases.tsv` or `surface-cases.tsv`. Recall checks are `scripts/jev-triage.sh --replay scripts/target-audit/triage-cases.tsv` (Jev, lexical, trim-1) and `--replay-surface scripts/target-audit/surface-cases.tsv` (`noul`). Setup and cutoff measurements: `docs/internal/jev-triage.md`.

