#!/bin/sh
# Regenerates the tool files from .agnostic-ai/ so each new session, branch
# switch and pull starts from current specs. Git hooks and the agent
# session-start hook call it. It keeps files edited since the last sync and
# never fails the checkout, pull or session: a missing or stale binary or a
# failed sync prints a short note to stderr and exits 0.

# Git hooks export GIT_DIR, which points Git at the hook's repository
# whatever the folder, so clear it and the other repository variables.
unset $(git rev-parse --local-env-vars 2>/dev/null)

# Tools copy this script to different depths (.claude/hooks/,
# .github/hooks/scripts/, ...), so ask Git for the root.
root=$(CDPATH='' cd -- "$(dirname -- "$0")" && git rev-parse --show-toplevel 2>/dev/null) || exit 0
cd "$root" || exit 0
[ -f agnostic-ai.yaml ] || exit 0

# A binary built in this checkout or in the main worktree may predate the
# code checked out here; PATH and AGNOSTIC_AI_BIN binaries are the user's
# choice and skip that check.
local_bin=
bin=${AGNOSTIC_AI_BIN:-}
if [ -z "$bin" ]; then
  # Git before 2.31 has no --path-format, and a relative common dir is
  # relative to the root we are in.
  main_bin=
  common=$(git rev-parse --git-common-dir 2>/dev/null) || common=
  case $common in
    '') ;;
    /*) main_bin="$(dirname -- "$common")/agnostic-ai" ;;
    *) main_bin="$(dirname -- "$root/$common")/agnostic-ai" ;;
  esac
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
  # A Claude Code cloud session builds and syncs in its install hook, which
  # runs beside this one.
  [ "${CLAUDE_CODE_REMOTE:-}" = true ] && exit 0
  echo "agnostic-ai: binary not found; run make build, then agnostic-ai sync" >&2
  exit 0
fi

# Stale when the binary was not built from a commit that contains the last
# change to the Go code. Go records the commit in the binary's build info
# (vcs.revision, also shown by `go version -m`). A release built without
# Git records only its version, such as v0.99.0, and skips the check.
if [ -n "$local_bin" ]; then
  rev=$(LC_ALL=C grep -a -o 'vcs\.revision=[0-9a-f]\{40\}' "$bin" 2>/dev/null | head -n 1)
  rev=${rev#vcs.revision=}
  if [ -z "$rev" ]; then
    mod=$(LC_ALL=C grep -a -o 'github\.com/chemaclass/agnostic-ai	v[0-9][0-9a-z.+-]*' "$bin" 2>/dev/null | head -n 1)
    version=${mod#*	}
    version=${version%+dirty}
    case $version in
      '') ;;
      *-*[0-9]-[0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f][0-9a-f])
        rev=${version##*-} ;;
      v[0-9]*.[0-9]*.[0-9]*) rev=release ;;
    esac
  fi
  code=$(git log -1 --format=%H -- cmd internal go.mod go.sum 2>/dev/null) || code=
  if [ -z "$rev" ] || { [ "$rev" != release ] && ! git cat-file -e "$rev^{commit}" 2>/dev/null; }; then
    echo "agnostic-ai: cannot tell which commit built $bin; run make build, then agnostic-ai sync" >&2
    exit 0
  fi
  if [ "$rev" != release ] && [ -n "$code" ] && ! git merge-base --is-ancestor "$code" "$rev" 2>/dev/null; then
    echo "agnostic-ai: $bin was built from older code than this checkout; run make build, then agnostic-ai sync" >&2
    exit 0
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
