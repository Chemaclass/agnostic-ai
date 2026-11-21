### General

- The disk image companion only installs an update signed with the same project certificate as the app that runs, so the update no longer rests on `gh`. An app opened from the disk image or a read-only folder stops before downloading and says to move it to Applications. A failure while the app is replaced shows on the next start. The release stops when a disk image fails that signer check (#3850).
- The companion checks for updates through `agnostic-ai companion update-status`, which reads the `releases/latest` redirect and `checksums.txt` instead of the rate-limited GitHub API, and keeps the answer for 24 hours in the cache `agnostic-ai upgrade` shares. **Check for updates** asks again at once with `--refresh` (#3850).
