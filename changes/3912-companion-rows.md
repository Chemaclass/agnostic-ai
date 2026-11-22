### General

- Each row under Needs you in the companion list says why it is there: a permission, shells still running, a question, no activity, a wait, or a new reply, even when the reply is hidden (#3912).
- A forgotten shell's line shows its run time, then its command cut in the middle, so two shells whose commands share a long start read apart. A hidden command adds its process ID, and the hover shows the whole command, the start time, the process ID and the process count (#3912).
- The server sends a stuck shell's command through its own redaction: withheld when the credential check finds a password or key, a URL credential masked, and up to 2,000 characters for the hover (#3912).
- The list draws its own tooltip, above or below the row and never over it, and it follows the row under a still pointer when the list changes (#3912).
- The circle shows a terminal icon when every waiting chat waits on a shell, counts only rows the list shows under Needs you, and a click while a chat needs you opens the list on the first one, past a search, a place or a folded group (#3912).
- The approvals mode is a small chip beside the search, with **Ask me**, instead of a full row above the list (#3912).
