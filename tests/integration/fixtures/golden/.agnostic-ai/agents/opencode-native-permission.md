---
name: opencode-native-permission
description: Review files with native permission rules.
targets: [opencode]
can: [read, shell]
x-opencode:
  permission:
    read:
      secrets/public/*: allow
      secrets/*: deny
    bash:
      git status: allow
      git *: ask
    github_create_issue: allow
    github_*: deny
---

Review project files.
