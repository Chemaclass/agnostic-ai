### General

- The companion footer shows **File claims** only while a claim or a conflict exists, with the number of claimed paths and a warning mark when a claim's session is gone or two sessions want one path (#3914).
- A chat that holds file claims shows a chip such as "2 paths claimed" in its row, which opens its claims. The claim button under the row's threads is gone (#3914).
- The claims card and the handoff form say what a file claim is and that it does not block edits, with a "?" that opens the mailbox docs at "Hand off files". Each claim says who made it and when (#3914).
- Claims whose session is gone come first under **Session gone**, with **Release all** for two or more. A session counts as gone only when its tool's store was read in full, and the server checks again before it frees the claims. **Release all** sends at most 200 paths per request and says how many it freed if one fails (#3914).
- A refused claim's conflict can be dismissed from the claims card, so the footer warning goes away (#3914).
- When one session asks for a path another claims, both rows move under Needs you with a line that names the path and the other session, and count on the circle. The holder can release the path, which ends the conflict; the requester can **Dismiss** it. Those chats stay on the first page and count in `needsYou` whatever their age. A refused handoff whose chats left the list can be declined from the footer's card (#3914).
- The conversation list sends `claimedPaths` and `claimConflicts` per row and `mailboxClaims` totals on the view. Each conflict has an `id`; `POST /api/mailbox/conflict/dismiss` takes it and removes that one conflict with no thread (#3914).
