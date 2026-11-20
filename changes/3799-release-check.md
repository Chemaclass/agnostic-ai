### General

- `serve` and the companion follow the approvals mode a tool carries out: in `review` with the tool's reviewer off, a permission wait shows as `ask`, and a `review` wait reaches you at once instead of after 10 seconds. The header names only a mode some tool carries out (#3799).
- A search or later page of the conversations list no longer counts a permission wait approvals still hold in `needsYou` (#3799).
- The approvals read and the conversations list leave out a logged approval detail that may hold a credential (#3799).
- `approvals ask` and **Ask me** refuse an approvals block that uses a YAML merge key or alias, which the rewrite would drop, and say to set it by hand (#3799).
- `POST /api/edit/hook` refuses unknown fields, like the other edit routes (#3799).
- A link to the `serve` web UI from another site opens the UI instead of a JSON error. API routes still refuse cross-site requests (#3799).
- A personal deny or ask list preview names the project rules it stops applying, in `dropped` (#3799).
- A settings field that only a permissions spec named `defaults` could take now says which file to create (#3799).
- The companion keeps keyboard focus on "N approved for you" and on a group title after a fold, and folds the approved list out inside its row (#3799).
