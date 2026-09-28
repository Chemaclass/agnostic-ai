#!/bin/sh
file=$(jq -r '.tool_input.file_path // empty')
[ -z "$file" ] || npx prettier --write "$file"
