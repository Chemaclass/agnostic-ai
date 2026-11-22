### General

- The companion finds an older standalone CLI even when a bundled command shadows it. Automatic checks keep a verified offer offline, and manual checks report release or checksum failures (#3888).
- When macOS refuses to open a disk image update, the updater tries to restore and open the earlier app. Disk image and terminal updates report recoverable failures and the log path on the next start. When the earlier app runs again, the failure says whether it was kept or restored (#3888).
