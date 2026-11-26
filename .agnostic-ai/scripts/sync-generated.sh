#!/bin/sh
# Regenerates the ignored tool files from .agnostic-ai/ so each new session,
# branch switch and pull starts from current specs. Git hooks and the agent
# session-start hook call it. It keeps files edited since the last sync and
# never fails the checkout, pull or session: a missing binary or a failed
# sync prints one line to stderr and exits 0.

root=$(CDPATH='' cd -- "$(dirname -- "$0")/../.." && pwd) || exit 0
cd "$root" || exit 0
[ -f agnostic-ai.yaml ] || exit 0

bin=${AGNOSTIC_AI_BIN:-}
if [ -z "$bin" ]; then
  if [ -x "$root/agnostic-ai" ]; then
    bin="$root/agnostic-ai"
  else
    bin=agnostic-ai
  fi
fi
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "agnostic-ai: binary not found; run make build, then agnostic-ai sync" >&2
  exit 0
fi

if ! out=$("$bin" sync --keep-edits 2>&1); then
  echo "agnostic-ai: sync failed; run agnostic-ai sync to see why" >&2
  printf '%s\n' "$out" | tail -n 3 >&2
fi
exit 0
