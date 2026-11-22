### General

- The local server deletes an archived conversation, stops a session or a stuck shell, and renames a Claude Code cloud session or a ChatGPT chat only with the companion's secret or the key of a tab that `view` opened. Other requests get 403 and nothing changes (#3938).
- When the server cannot check whether a terminal session still runs, an open no longer resumes it. This happens when the process or open-file read fails, such as a missing `lsof` on Linux or a failed lookup on Windows. The reply says why and gives the command to run by hand (#3938).
