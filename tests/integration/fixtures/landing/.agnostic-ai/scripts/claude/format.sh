#!/bin/sh
files=$(agnostic-ai hook paths) || exit 1
printf '%s\n' "$files" | while IFS= read -r f; do
  [ -z "$f" ] || npx prettier --write "$f"
done
