### General

- `agnostic-ai view --scope user` has a **Settings > Approvals and companion** tab with the same approvals mode, Approve and Deny switch, status lines and **Apply to my tools** as the companion menu, plus the mailbox switch. Only a tab that `view` opened can loosen approvals: it opens the page through a private one-time file, and the page trades its code for a session key kept in memory. Other tabs can make approvals stricter (#3913).
- **Apply to my tools**, in the companion and on the page, lists every hook and MCP server command the sync installs. Adding a user-level hook or MCP server with the page's add forms needs the companion or an unlocked page. Ask me keeps a local `companion: false` (#3913).
