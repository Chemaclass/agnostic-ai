# Issue triage

[Contributor docs](README.md) · [Feature map](../../.github/issue-features.json) · [Approved decision](https://github.com/Chemaclass/agnostic-ai/issues/3955)

Start an idea with a short account of what someone wants to do or what gets in the way. A proposed design, acceptance list and implementation plan can follow after the outcome is clear. Keep the current outcome, scope and acceptance criteria in the issue body. Comments supply evidence and discussion; move settled changes into the body so a contributor can read the current plan in one place.

Give every issue one feature label from the checked-in map. The map holds the 11 groups, 43 exact names, scopes and starting code paths. Choose by the outcome a person uses, across interfaces. Do not infer the label from a title, file path or existing area label alone. Read the body and every comment before classifying it.

## Choose one feature

- Session identity and state use `sessions:catalog`. Search, rename and archive use `sessions:organize`. Starting, resuming, switching and copying a resume command use `sessions:open-session`, even when the change is in the Companion or web list.
- Acting on waiting work from the Companion uses `companion:attention`. Permission policy uses `runtime:approvals`. Who may read or call a web route uses `web-ui:access`.
- Transferring responsibility and claiming files use `mailbox:handoffs`. Carrying a task into another chat through notes, a branch or the cloud uses `sessions:handoff`. Reusable spec installation uses `specs:packs`.
- One tool's native mapping uses `targets:adapters`. A portable contract change uses its `specs:` feature. A new target uses `targets:new`; detecting vendor changes uses `targets:audit`.
- A feature's fix, documentation or tests inherit that feature. Use `delivery:code-health` for shared code that spans features without one owner.
- For a historic umbrella, use its stated outcome, or its first acceptance outcome when no parent states one. For new work, link separate issues when their outcomes can ship separately. Record ambiguous pilot cases in #3955 before expanding the migration.

Type, priority, status, security, performance and source markers describe separate facts. Keep them. `target-audit`, `vendor-watch`, `tool-load` and dependency markers remain source labels. Legacy `area:` labels never satisfy the one-feature check. Pull requests retain their existing labels and are outside this migration.

## Verify a selected window

The verifier reads GitHub through `gh` and prints JSON. It creates, edits and classifies nothing. It is a manual report, with no label enforcement workflow. It paginates all issue and label definitions through the REST API, excludes pull requests, then selects issues by creation time in UTC. There is no search-result cap.

The approved pilot covers the complete week November 18 through November 24, 2026. The lower bound is inclusive and the upper bound exclusive:

```sh
scripts/verify-issue-features.sh --repo Chemaclass/agnostic-ai \
  --created-from 2026-11-18T00:00:00Z \
  --created-before 2026-11-25T00:00:00Z
```

The report lists missing feature definitions, unexpected names in the approved group namespaces, issues with no allowed feature, issues with several, and unknown feature names. It counts every allowed label, including empty ones, and reports any label covering more than 25% of selected issues. Concentration and empty labels call for a scope review; they do not invalidate an otherwise complete window. A zero-issue selection fails instead of claiming coverage.

Two more review notes help the migration. `definitions.descriptionDrift` lists each feature label whose GitHub description differs from the map scope, with both texts. It is skipped for a label given by name only. `legacyArea` lists selected issues that have a feature label and still carry an `area:` label, so that label can go. Neither note changes `valid` or the exit code.

Only exact names in the map count. Definitions outside the approved group namespaces are left alone. Label colors are not checked. An exit code of 1 means the definitions or classifications need work; invalid input exits 2 or reports a JSON parsing error.

For saved exports or a manually selected set, supply both JSON arrays. This requires `jq` and makes no GitHub calls:

```sh
scripts/verify-issue-features.sh \
  --labels-file /tmp/feature-labels.json \
  --issues-file /tmp/pilot-issues.json
bashunit scripts/verify-issue-features_test.sh
```

Label arrays may contain names or `{ "name": "cli:sync" }` objects. Issues need `number` and `labels`; bounds also need `created_at` or `createdAt`. Date selection rejects missing, invalid or noncanonical UTC creation times before filtering the window. REST `pull_request`, GraphQL `__typename: "PullRequest"`, `isPullRequest: true` and pull-request page URLs are excluded. Include open and closed issues. The verifier does not decide whether a human chose the right feature: check each body and retain the reasoning for mixed cases.

## Migrate and close

Create the approved label definitions, classify the pilot by body, then inspect the verifier report before backfilling earlier issues. Include issues carrying `area:sync` or `area:verification`. Move a verified performance tag to the independent `performance` label. Keep legacy label definitions while pull requests still use them. Remove an issue's old area label only after assigning and checking its replacement.

Add available implementation and verification links to completed issue bodies, with the observed result. A link to a pull request, test run or screenshot is optional during capture; do not invent missing evidence for historic work.

The tool-load failure path already creates its `tool-load` source label before filing an alert in [the workflow](../../.github/workflows/tool-load.yml). Confirm this ordering by reading the workflow. Do not trigger a real failure or file an alert to test the migration.
