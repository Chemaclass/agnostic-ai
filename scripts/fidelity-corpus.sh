#!/usr/bin/env bash
#
# fidelity-corpus.sh - copy the pinned native AI tool config listed in
# tests/fidelity/corpus.tsv into tests/fidelity/<name>/repo.tar.
#
# The files stay inside a tar so no AI tool working in this repository
# loads them: Gemini CLI reads every nested GEMINI.md, and Claude Code a
# nested CLAUDE.md, as instructions for this project.
#
# Each entry keeps only the listed paths at the pinned commit, plus the
# upstream LICENSE and a SOURCE.md naming where the files came from.
# Symlinks are dropped: Git on Windows checks one out as a text file, and
# the corpus must read the same on every OS. The test that uses the
# corpus is tests/integration/import_fidelity_test.go.
#
# Usage:
#   scripts/fidelity-corpus.sh             # every entry
#   scripts/fidelity-corpus.sh vscode      # only the named entries
#
# Needs git and network access. Portable: POSIX-ish bash, no GNU-only flags.

set -euo pipefail

FIDELITY_ROOT=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
FIDELITY_DIR="$FIDELITY_ROOT/tests/fidelity"

# fetch_entry <name> <repo> <commit> <license> <paths> copies one entry.
fetch_entry() {
  local name="$1" repo="$2" commit="$3" license="$4" paths="$5" tmp dest p
  local specs=()
  tmp=$(mktemp -d)
  dest="$FIDELITY_DIR/$name"
  git clone -q --filter=blob:none --no-checkout "https://github.com/$repo" "$tmp/src"
  git -C "$tmp/src" cat-file -e "$commit^{commit}" 2>/dev/null ||
    git -C "$tmp/src" fetch -q origin "$commit"
  # The paths are Git globs, never the shell's: `.gemini/**` must not
  # expand against whatever directory the script runs in.
  set -f
  for p in $paths; do specs+=(":(glob)$p"); done
  set +f
  case "$dest" in "$FIDELITY_DIR"/?*) ;; *) echo "refusing to write $dest" >&2; return 1 ;; esac
  mkdir -p "$dest" "$tmp/repo"
  # One archive per path: a path the commit does not have (an optional
  # opencode.jsonc) would otherwise fail every other path with it.
  for p in "${specs[@]}"; do
    if git -C "$tmp/src" archive -o "$tmp/part.tar" "$commit" -- "$p" 2>/dev/null; then
      tar -x -C "$tmp/repo" -f "$tmp/part.tar"
    fi
  done
  find "$tmp/repo" -type l -exec rm -f {} +
  # COPYFILE_DISABLE stops macOS tar from adding ._* metadata entries.
  (cd "$tmp/repo" && find . -type f | LC_ALL=C sort | COPYFILE_DISABLE=1 tar -cf "$dest/repo.tar" -T -)
  git -C "$tmp/src" show "$commit:LICENSE" >"$dest/LICENSE" 2>/dev/null ||
    git -C "$tmp/src" show "$commit:LICENSE.txt" >"$dest/LICENSE"
  cat >"$dest/SOURCE.md" <<EOF
Copied from https://github.com/$repo at commit $commit, under the
$license license in LICENSE. Only these paths were kept, without symlinks:
\`$paths\`. Refresh with \`scripts/fidelity-corpus.sh $name\`.
EOF
  printf '%s: %s files\n' "$name" "$(find "$tmp/repo" -type f | wc -l | tr -d ' ')"
  rm -rf "$tmp"
}

fidelity_main() {
  local name repo commit license targets paths
  while IFS=$'\t' read -r name repo commit license targets paths; do
    case "$name" in '#'* | '') continue ;; esac
    if [ "$#" -gt 0 ]; then
      case " $* " in *" $name "*) ;; *) continue ;; esac
    fi
    : "$targets"
    fetch_entry "$name" "$repo" "$commit" "$license" "$paths"
  done <"$FIDELITY_DIR/corpus.tsv"
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  fidelity_main "$@"
fi
