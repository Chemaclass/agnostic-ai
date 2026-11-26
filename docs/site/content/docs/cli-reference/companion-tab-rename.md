+++
title = "Tab renames"
description = "How a name typed on a running chat's terminal tab becomes the chat's name, and which terminals support it on each system."
weight = 47

[extra]
group = "Reference"
+++

# Tab renames

Rename a running chat's Ghostty tab on macOS, and the chat takes that name on the next refresh or the one after. Only the companion and a user-level `serve` do this. A `serve` limited to one project cannot see a chat outside it that may share the tab, so it leaves tab names alone. The name goes through the same steps as a rename in the list: the tool first, then the marks file. So the row, a search and the mailbox show it, and it stays after the session ends.

- Companion reads only the tab's `name override`, the name Ghostty keeps for a tab you renamed. A title the shell or the tool sets should not land there, so it should not be copied. This is not yet checked by hand.
- The tab must hold one pane, and only one running chat. A split tab, or a tab Companion cannot match to one chat for sure, changes nothing.
- An empty `name override` never changes the chat's name, so clearing the tab's name should keep it. This is not yet checked by hand.
- The first time Companion sees a tab that already has a name, it takes that name only when the chat has none of its own, and the name is not one an earlier chat in that tab had. A chat resumed in a new process counts as a first look too.
- A name the tool or the name check refuses (empty, longer than 120 characters, or a possible credential) is not saved. It is tried again only once you type another name. The server log says why, without the name.
- F2 still names the tab. The tab then shows the chat's name, so Companion renames nothing back.
- Each refresh asks Ghostty once for all its tabs, only while a running chat is in Ghostty. The read is skipped when Ghostty is not running.
- The first read may make macOS ask whether Companion may control Ghostty, even if you never used F2. If you refuse, or leave the question open for 3 seconds, the list still works, no name changes, and Companion waits 5 minutes before it asks Ghostty again.

| OS | Terminal | Tab name read | Status |
|----|----------|---------------|--------|
| macOS | Ghostty | The tab's `name override` through AppleScript, matched to the chat by each pane's tty | Implemented. Read checked on Ghostty 1.4.0. Shell title changes, clearing and the two-tab walkthrough are still to be checked by hand. |
| macOS | Terminal.app | Not read yet | Not supported yet: a manual rename and a title the shell sets are not yet told apart on a real host. |
| macOS | iTerm2 | Not read yet | Not supported yet: which of the tab title and the session name a manual rename changes is not yet checked. |
| macOS, Linux | WezTerm | Not read yet | Not supported yet: the `tab_title` field of `wezterm cli list` is not yet checked on a real host. |
| macOS, Linux | kitty | Not read yet | Not supported yet: the `title_overridden` field of `kitty @ ls` is not yet checked on a real host. |
| Linux | Konsole | Not read yet | Not supported yet: a manual rename cannot yet be told apart from the tab title format. |
| macOS, Linux | tmux | Not read yet | Not supported yet: a window name you typed cannot yet be told apart from one a program set. |
| macOS | Warp | None | Not supported: Companion has no way to reach Warp's tabs. |
| Linux | GNOME Terminal, Ptyxis, Alacritty, foot, xterm, Tilix, Xfce Terminal | None | Not supported: Companion has no way to reach these terminals' tabs. |
| Windows | Windows Terminal, classic console | None | Not supported: Companion cannot match a running chat to a tab or console on Windows. |
