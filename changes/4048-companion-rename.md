### General

- **Breaking:** `make companion` builds `agnostic-companion`, which appears as `Agnostic Companion.app` on macOS (#4048).
- macOS downloads use `agnostic-companion_darwin_<arch>` for `.dmg` and `.tar.gz`. The old-layout disk image stays available in new releases through v1.0.0 and for at least 30 days after the first renamed release, whichever is later (#4048, #4060).
- `agnostic-ai companion` starts the renamed app with its existing settings in `agnostic-ai/` and bundle ID `dev.agnostic-ai.companion`. Disk-image updates keep the installed app's path (#4048).
