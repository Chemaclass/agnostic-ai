# Target capability radar

[All docs](../README.md) · [Targets](targets.md) · [Changelog](../../CHANGELOG.md)

AI coding tools change their project configuration every week. The target capability radar turns those upstream changes into a developer digest you can act on.

The radar covers consequential changes only. That includes a project path that stops working, a safety boundary that changes, a native feature that removes a workaround, or a project capability that several tools now share.

## What each digest tells you

Every entry answers four questions up front:

- **What changed?** The documented vendor behavior and affected targets.
- **Why does it matter?** The workflow, safety, or compatibility impact for a project.
- **Where is agnostic-ai now?** Current support, a working target-specific extension, a lossy workaround, or no representation yet.
- **What happens next?** The smallest adapter fix, design decision, target extension, or research check.

Target names do not prove shared behavior. The audit checks each target's own project scope, lifecycle, defaults, and native file shape before it calls two features equivalent.

The digest also records a workaround when one exists. An `x-<target>` extension can expose a vendor-specific field today, but it does not prove that the field belongs in the shared schema.

## How observations are classified

| Disposition | Meaning |
|---|---|
| `adapter-gap` | The shared spec can represent the behavior, but one adapter does not emit or import it yet. |
| `spec-candidate` | Several targets share a verified project intent that the current spec cannot preserve. This needs a design decision before implementation. |
| `target-extension` | The capability is useful but vendor-specific. An `x-<target>` extension is the honest fit. |
| `watch` | The change matters, but the evidence or cross-target shape is not strong enough for a product decision. |

A proposed improvement is not shipped support. Design candidates stay out of automated fixes until their schema and scope are approved.

## Follow the rolling issue

[Find the Target capability radar issue](https://github.com/Chemaclass/agnostic-ai/issues?q=is%3Aissue%20in%3Atitle%20%22Target%20capability%20radar%22), then use GitHub's Subscribe button to receive each published audit comment.

The first published audit creates the issue. If the search is empty, no radar digest has been published yet.

Each update keeps the main change, impact, current position, and next action visible. Source evidence, target comparisons, research disagreements, and audit coverage sit in expandable sections below that summary.

## Know which record to trust

The radar records observations and proposed work. It can describe a vendor feature before agnostic-ai supports it.

[`targets.md`](targets.md) records support that ships in the current code, including native paths, capability limits, and opt-in settings.

[`CHANGELOG.md`](../../CHANGELOG.md) records what agnostic-ai released. Future vendor observations belong in the radar, not the changelog.

Observation, support, release. Keep those three states separate.
