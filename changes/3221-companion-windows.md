### General

- On Windows, a click in the companion list opens editor chats, Codex threads and cloud sessions in their apps, and resumes an ended terminal chat in a new PowerShell window. A terminal chat written in the last day is not resumed, because Windows cannot tell whether it still runs; the list offers its command instead (#3221).
- The companion lists Claude Code cloud sessions on Windows, read from Claude Code's `.credentials.json` (#3221).
- The companion list says when Codex, Cursor or ChatGPT chats are missing because the `sqlite3` command is not installed (#3221).
