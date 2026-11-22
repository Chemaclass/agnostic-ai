### General

- User-level servers serve the companion's settings as one document, `GET /api/views/settings`, with a tag for cheap polling, and take one change at a time with `POST /api/settings`. A change that makes approvals less strict (`review`, `yolo`, the companion's Approve and Deny buttons, turning off approval of agent messages) needs the companion's secret; `POST /api/settings/sync` plans or runs `sync --scope user` and needs it too. Loosening changes are logged with `decision: changed` (#3913).
- `POST /api/mailbox/settings` now refuses `{"approveAgentSends": false}` without the companion's secret. A script that turned approval off through the local server must change the setting in the companion instead (#3913).
- The companion saves `companion.json` under a lock file and reads it again first, so a value another program wrote stays. Its list poll also notices changes to the mailbox and companion settings files (#3913).
