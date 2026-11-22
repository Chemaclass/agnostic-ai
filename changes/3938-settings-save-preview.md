### General

- Approvals changes and the targets and builtins switches of `view --scope user` now take turns on the home config, so one no longer overwrites another; an approvals change also stops when the file was edited by hand after it was read. **Apply to my tools** now lists each hook and MCP server command as sync writes it for each tool, with its arguments, working folder, environment variables (secret values masked) and web hook URL, naming the tools when only some run a line (#3938).
