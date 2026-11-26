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

# Prints the absolute Git common dir of a folder, which every worktree of
# one repository shares.
common_dir() {
  command -v git >/dev/null 2>&1 || return 1
  dir=$(git -C "$1" rev-parse --git-common-dir 2>/dev/null) || return 1
  case "$dir" in /*) ;; *) dir="$1/$dir" ;; esac
  CDPATH='' cd -- "$dir" 2>/dev/null && pwd -P
}

# hook paths prints project files relative to the root and keeps files
# outside it absolute. An absolute path is formatted only when it sits in
# another worktree of this repository.
root_common=$(common_dir "$root") || root_common=''
files=$("$bin" hook paths 2>/dev/null) || exit 0
printf '%s\n' "$files" | while IFS= read -r file; do
  case "$file" in
    *.go) [ -f "$file" ] || continue ;;
    *) continue ;;
  esac
  case "$file" in
    ../*) continue ;;
    /*)
      [ -n "$root_common" ] || continue
      [ "$(common_dir "$(dirname -- "$file")")" = "$root_common" ] || continue
      ;;
  esac
  gofmt -s -w -- "$file" 2>/dev/null
done
exit 0
