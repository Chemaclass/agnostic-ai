#!/bin/sh
# Formats the Go files an agent edits with gofmt -s, as make fmt-check
# expects. It never fails the edit: a missing tool, a payload it cannot read
# or a file gofmt cannot parse leaves every file as it is.

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd) || exit 0
cd "$root" || exit 0

bin=${AGNOSTIC_AI_BIN:-}
if [ -z "$bin" ]; then
  if [ -x "$root/agnostic-ai" ]; then
    bin="$root/agnostic-ai"
  else
    bin=agnostic-ai
  fi
fi
command -v "$bin" >/dev/null 2>&1 || exit 0
command -v gofmt >/dev/null 2>&1 || exit 0

# hook paths prints project files relative to the root and keeps files
# outside it absolute; only the project's own files are formatted.
files=$("$bin" hook paths 2>/dev/null) || exit 0
printf '%s\n' "$files" | while IFS= read -r file; do
  case "$file" in
    /* | ../*) continue ;;
    *.go) [ -f "$file" ] && gofmt -s -w "$file" 2>/dev/null ;;
  esac
done
exit 0
