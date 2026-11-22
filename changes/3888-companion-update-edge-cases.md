### General

- A terminal update on macOS restores and opens the earlier companion when the new one does not start. The CLI upgrade stays (#3888).
- When Homebrew or npm already serves a newer release than the one checked, the update keeps that CLI and installs the matching companion (#3888).
- A second disk image update of the same app stops until the first ends. CLI processes that run at once no longer overwrite each other's update check results (#3888).
