#!/bin/sh
# The hook event arrives as JSON on stdin.
file=$(jq -r '.tool_input.file_path // empty')
[ -n "$file" ] || exit 0
npx prettier --write "$file"
