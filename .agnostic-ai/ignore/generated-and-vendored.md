---
name: generated-and-vendored
description: Build output, vendored dependencies, and local state an agent should not read or index in this repository.
---

# Built site and release files. `make` regenerates them; never edit them as source.
/_site/
/docs/site/public/
/dist/
/build/
/bin/
/agnostic-ai
/agnostic-ai.exe

# Compiled playground bundle and the Go helper script copied beside it.
# Rebuilt by `make playground-build`.
/docs/playground/agnostic-ai.wasm
/docs/playground/wasm_exec.js

# Dependency trees, lockfiles, and editor-plugin build output.
node_modules/
package-lock.json
/editors/vscode/out/
/editors/jetbrains/build/
/editors/jetbrains/.gradle/
/npm/platforms/

# Coverage, profiles, and local run state.
coverage.out
coverage.html
coverage.txt
*.prof
*.test
/.bashunit/
/.golangci-cache/
/.agnostic-ai/.sync-state

# Working notes and scratch space, not part of the project.
/tmp/
/local/
/NOTES.md

# Local secrets. Keep them out of git and out of prompts.
.env
.env.local
.env.*.local
*.pem
*.key
