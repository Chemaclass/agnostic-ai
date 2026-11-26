#!/bin/sh
# Regenerates the ignored tool files from .agnostic-ai/ so each new session,
# branch switch and pull starts from current specs. Git hooks and the agent
# session-start hook call it. It keeps files edited since the last sync and
# never fails the checkout, pull or session: a missing or stale binary or a
# failed sync prints a short note to stderr and exits 0.
#
# --skip-while-rebasing exits at once during a rebase, so the post-checkout
# hook leaves the sync to post-rewrite, which sees the final tree.

# Tools copy this script to different depths (.claude/hooks/,
# .github/hooks/scripts/, ...), so ask Git for the root.
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$root" || exit 0
[ -f agnostic-ai.yaml ] || exit 0

if [ "${1:-}" = --skip-while-rebasing ]; then
  for state in rebase-merge rebase-apply; do
    dir=$(git rev-parse --git-path "$state" 2>/dev/null) || continue
    [ -d "$dir" ] && exit 0
  done
fi

# A binary built in this checkout or in the main worktree may predate the
# code checked out here; PATH and AGNOSTIC_AI_BIN binaries are the user's
# choice and skip that check.
local_bin=
bin=${AGNOSTIC_AI_BIN:-}
if [ -z "$bin" ]; then
  common=$(git rev-parse --path-format=absolute --git-common-dir 2>/dev/null) || common=
  main_bin=
  [ -n "$common" ] && main_bin="$(dirname -- "$common")/agnostic-ai"
  if [ -x "$root/agnostic-ai" ]; then
    bin="$root/agnostic-ai"
    local_bin=1
  elif [ -n "$main_bin" ] && [ -x "$main_bin" ]; then
    bin=$main_bin
    local_bin=1
  else
    bin=agnostic-ai
  fi
fi
if ! command -v "$bin" >/dev/null 2>&1; then
  echo "agnostic-ai: binary not found; run make build, then agnostic-ai sync" >&2
  exit 0
fi

# Stale when the last commit touching the Go code is newer than the binary.
# touch -d with an ISO date and test -nt work on macOS and Linux alike.
if [ -n "$local_bin" ]; then
  changed=$(TZ=UTC git log -1 --date=format-local:%Y-%m-%dT%H:%M:%SZ \
    --format=%cd -- cmd internal go.mod go.sum 2>/dev/null) || changed=
  if [ -n "$changed" ] && ref=$(mktemp 2>/dev/null); then
    stale=
    touch -d "$changed" "$ref" 2>/dev/null && [ "$ref" -nt "$bin" ] && stale=1
    rm -f "$ref"
    if [ -n "$stale" ]; then
      echo "agnostic-ai: $bin is older than the checked-out code; run make build, then agnostic-ai sync" >&2
      exit 0
    fi
  fi
fi

if out=$("$bin" sync --keep-edits </dev/null 2>&1); then
  # Name the edited files sync left alone, so a stale one is not a surprise.
  printf '%s\n' "$out" | grep '^ *~ kept' >&2
  exit 0
fi
case $out in
  # Another sync holds the project lock and is doing this work.
  *.command-lock*|*"project is locked by agnostic-ai"*) exit 0 ;;
esac
echo "agnostic-ai: sync failed; run agnostic-ai sync to see why" >&2
printf '%s\n' "$out" | tail -n 3 >&2
exit 0
