+++
title = "Tab renames"
description = "How a name typed on a running chat's terminal tab becomes the chat's name, and which terminals support it on each system."
weight = 47

[extra]
group = "Reference"
+++

# Tab renames

On macOS, rename a running chat's tab in Ghostty 1.4 or later or in WezTerm 20230408 or later, and the chat takes that name on the next refresh or the one after. Only the companion and a user-level `serve` do this. A `serve` limited to one project cannot see a chat outside it that may share the tab, so it leaves tab names alone. The name goes through the same steps as a rename in the list: the tool first, then the marks file. So the row, a search and the mailbox show it, and it stays after the session ends.

- Companion reads Ghostty's `name override` or WezTerm's `tab_title`. Titles set by the shell or the tool leave the saved chat name alone.
- From another WezTerm tab, use `wezterm cli list` to find the running chat's `TABID`, then run `wezterm cli set-tab-title --tab-id <tab-id> "name"`. [Command reference](https://wezterm.org/cli/cli/set-tab-title.html)
- A shortcut can call `tab:set_title()`. The example's `initial_value` field requires a nightly build. [Shortcut example](https://wezterm.org/config/lua/keyassignment/PromptInputLine.html)
- If `wezterm.lua` calls `tab:set_title()` for automatic titles, Companion follows those changes too. An automatic title can replace a name you set in Companion.
- The tab must hold one pane, and only one running chat. A split tab, or a tab Companion cannot match to one chat for sure, changes nothing.
- Clearing the tab's name leaves the saved chat name alone.
- The first time Companion sees a tab that already has a name, it takes that name only when the chat has none of its own, and the name is not one an earlier chat in that tab had. A chat resumed in a new process counts as a first look too.
- A name the tool or the name check refuses (empty, longer than 120 characters, or a possible credential) is not saved. It is tried again only once you type another name. The server log says why, without the name.
- F2 still names the tab. The tab then shows the chat's name, so Companion renames nothing back.
- Each refresh reads a supported app's tabs once, while it has a running chat. WezTerm uses the instance its CLI can reach. It leaves a chat alone if its tty is missing from that instance.
- The first read may make macOS ask whether Companion may control Ghostty, even if you never used F2. The list still works if a read fails. If you leave the question open, the read stops after 3 seconds. After any failed read, Companion waits 5 minutes before trying again.

## Verification by terminal

On macOS 27.2.1, a script drove two scratch tabs in Ghostty 1.4.0 and the conversation server's handler at `34b82c7b6`. Setting one tab's override renamed its chat and left the second chat alone. Search found the name. The request behind F2 then updated the matching tab and saved the new name in the transcript. Shell-set titles, clearing an override and splitting a tab kept the saved chat name. Replacing the fixture's session ID on the same tty kept the replacement chat's title until the tab got a fresh override. [Tested commit](https://github.com/Chemaclass/agnostic-ai/commit/34b82c7b6819c5217c9ef12262ad951f58944f07)

The desktop walkthrough still needs a tab rename typed in Ghostty, F2 in Companion, and a check that the list loads after macOS denies Automation. [Terminal tracking issue](https://github.com/Chemaclass/agnostic-ai/issues/4037)

| OS | Terminal | Title and session evidence | Tab-to-chat sync |
|----|----------|----------------------------|------------------|
| macOS | Ghostty | AppleScript reads the tab's `name override`. The pane's tty identifies the chat. | Implemented. |
| macOS | Terminal.app | Terminal.app 2.15 on macOS 27.2.1 let AppleScript set and read a tab's `custom title`. A shell OSC 0 sequence overwrote it. Each tab has a tty. | Unavailable. Needs a separate signal for an Inspector rename. |
| macOS | iTerm2 | iTerm2 3.7.9 rejected an AppleScript write to the tab's `title`. The Python API exposes `titleOverride` per tab, and each session has a tty. | Unavailable. Check the Python API after a menu rename, a shell title change and clearing the tab title. |
| macOS | WezTerm | The GUI and mux in build `20240203-110809-5046fc22` both report the explicit `tab_title` separately from the pane title. A split repeats the tab ID and override with a separate tty per pane. Setting and clearing the override leaves shell-set titles alone. | Implemented. |
| Linux | WezTerm | The same CLI fields are available. The native Linux tab and chat check is pending. | Unavailable. |
| macOS, Linux | kitty | On Debian 13 with Xvfb, kitty 0.51.1 set `title_overridden` in `kitty @ ls` after a keyboard rename and cleared it when the title was cleared. Shell OSC 0 changed only the pane title. The other tab kept its name. The window PID identified the pane's tty. Debian's kitty 0.41.1 omitted the field. | Unavailable. Needs a native desktop and Companion check. |
| Linux | Konsole | In Konsole 25.04.2 on Debian 13 with Xvfb, Tab Properties and shell OSC 30 both changed `title(1)` and `tabTitleFormat(0)` on D-Bus. The process's service and session identify the tab. | Unavailable. Needs a user-set flag in D-Bus. |
| macOS, Linux | tmux | In a Debian 13 container under tmux 3.5a with `allow-rename` on, `rename-window` and a pane's shell escape yielded the same `window_name`, `automatic-rename` and tty values. The tmux window is separate from the outer terminal tab. | Unavailable. Needs a way to tell the two updates apart. |
| macOS | Warp | Companion has no way to reach Warp's tabs. | Unavailable. |
| Linux | GNOME Terminal, Ptyxis, Alacritty, foot, xterm, Tilix, Xfce Terminal | Companion has no way to reach these terminals' tabs. | Unavailable. |
| Windows | Windows Terminal, classic console | Companion cannot match a running chat to a tab or console on Windows. | Unavailable. |
