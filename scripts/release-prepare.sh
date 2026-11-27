#!/usr/bin/env bash
#
# release-prepare.sh - do the scripted part of a release before curation.
#
# Usage:
#   scripts/release-prepare.sh [--skip-git-check] vX.Y.Z [YYYY-MM-DD]
#   scripts/release-prepare.sh --check vX.Y.Z
#   scripts/release-prepare.sh --check-changelog vX.Y.Z
#
# The first form folds changes/*.md into CHANGELOG.md, moves [Unreleased] into
# a dated section, archives the previous group of ten minor versions on an
# X.Y1.0 release, bumps the version pins, sets the size budget and signal
# dates, and writes the briefing skeleton. A rerun skips the steps already
# done, so it resumes after a failure and never rolls the changelog over twice.
# It refuses a branch other than main and, before the dated section exists, any
# uncommitted change; a resumed run allows edits to the files it owns.
# --skip-git-check drops these checks.
#
# --check validates the curated dated section and the briefing: bullets at
# most 160 characters, at most five bullets per section and per tool, no
# empty heading, and no TODO left in the briefing. --check-changelog skips
# the briefing.
#
# RELEASE_ROOT points it at another checkout. Portable: bash 3.2, BSD awk/sed.

set -euo pipefail

RELEASE_SCRIPTS=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

release_die() { printf 'error: %s\n' "$*" >&2; exit 1; }
release_step() { printf '%s\n' "$*"; }

# release_owned lists the paths a release run may leave modified.
release_owned() {
  case "$1" in
    CHANGELOG.md | docs/CHANGELOG-archive.md | changes/* | cmd/agnostic-ai/main.go | \
      docs/site/config.toml | agnostic-ai.yaml | scripts/binary-size-budget | \
      scripts/target-audit/signals.tsv | docs/site/content/updates/*) return 0 ;;
  esac
  return 1
}

# release_git_check requires main and a clean tree. On a resumed run
# (resume=1) it allows edits to the release files.
release_git_check() {
  local root="$1" resume="$2" branch line path
  branch=$(git -C "$root" rev-parse --abbrev-ref HEAD)
  [ "$branch" = main ] || release_die "on branch $branch; a release runs on main (--skip-git-check overrides)"
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    path=${line:3}
    path=${path#*" -> "}
    [ "$resume" = 0 ] && release_die "uncommitted change before the release started: $path"
    release_owned "$path" || release_die "uncommitted change outside the release files: $path"
  done <<EOF
$(git -C "$root" status --porcelain --untracked-files=all)
EOF
}

# release_section_date prints the date of the "## vX.Y.Z - DATE" heading.
release_section_date() {
  awk -v h="## $2 - " 'index($0, h) == 1 { print substr($0, length(h) + 1, 10); exit }' "$1"
}

# release_unreleased_body prints the lines between [Unreleased] and the next
# level-2 heading.
release_unreleased_body() {
  awk '/^## \[Unreleased\]$/{p=1;next} p && /^## /{exit} p' "$1"
}

# release_drop_empty_headings removes ### and #### headings with no entry
# beneath them, including a ### whose #### headings are all empty, and
# squeezes blank lines.
release_drop_empty_headings() {
  awk '
    { line[++n] = $0 }
    function level(s) {
      if (s ~ /^#### /) return 4
      if (s ~ /^### /) return 3
      return 0
    }
    END {
      for (i = 1; i <= n; i++) {
        keep[i] = 1
        l = level(line[i])
        if (l == 0) continue
        keep[i] = 0
        for (j = i + 1; j <= n; j++) {
          m = level(line[j])
          if (m != 0 && m <= l) break
          if (m == 0 && line[j] !~ /^[[:space:]]*$/) { keep[i] = 1; break }
        }
      }
      blank = 0; started = 0
      for (i = 1; i <= n; i++) {
        if (!keep[i]) continue
        if (line[i] ~ /^[[:space:]]*$/) { if (started) blank = 1; continue }
        if (blank) print ""
        print line[i]; blank = 0; started = 1
      }
    }
  '
}

# release_rollover moves the [Unreleased] body into "## VER - DATE".
release_rollover() {
  local changelog="$1" ver="$2" date="$3" body tmp
  body=$(release_unreleased_body "$changelog" | release_drop_empty_headings)
  [ -n "$body" ] || release_die "[Unreleased] in $changelog has no entries to release"
  tmp=$(mktemp)
  BODY="$body" awk -v head="## $ver - $date" '
    /^## \[Unreleased\]$/ {
      print; print ""; print head; print ""; print ENVIRON["BODY"]; print ""
      skip = 1; next
    }
    skip && /^## / { skip = 0 }
    !skip
  ' "$changelog" > "$tmp"
  cat "$tmp" > "$changelog"
  rm -f "$tmp"
}

# release_archive moves dated sections older than vMAJ.CUT.0 from the
# changelog to the top of the archive and updates both "Releases before" lines.
release_archive() {
  local changelog="$1" archive="$2" maj="$3" cut="$4" moved kept
  moved=$(mktemp); kept=$(mktemp)
  awk -v maj="$maj" -v cut="$cut" -v moved="$moved" '
    function older(s,   v, p) {
      v = s; sub(/^## v/, "", v); sub(/ .*/, "", v)
      split(v, p, ".")
      if (p[1] + 0 != maj + 0) return p[1] + 0 < maj + 0
      return p[2] + 0 < cut + 0
    }
    /^## / { out = ($0 ~ /^## v[0-9]+\.[0-9]+\.[0-9]+ - / && older($0)) }
    out { print > moved; next }
    { kept[++n] = $0 }
    END {
      while (n > 0 && kept[n] ~ /^[[:space:]]*$/) n--
      for (i = 1; i <= n; i++) print kept[i]
    }
  ' "$changelog" > "$kept"
  if [ ! -s "$moved" ]; then rm -f "$moved" "$kept"; return 0; fi
  cat "$kept" > "$changelog"
  awk -v moved="$moved" '
    !done && /^## / {
      while ((getline l < moved) > 0) { buf[++n] = l }
      while (n > 0 && buf[n] ~ /^[[:space:]]*$/) n--
      for (i = 1; i <= n; i++) print buf[i]
      print ""
      done = 1
    }
    { print }
    END {
      if (!done) {
        while ((getline l < moved) > 0) print l
      }
    }
  ' "$archive" > "$kept"
  cat "$kept" > "$archive"
  rm -f "$moved" "$kept"
  release_set_before "$changelog" "v$maj.$cut.0"
  release_set_before "$archive" "v$maj.$cut.0"
  release_step "archive: moved releases before v$maj.$cut.0 to $(basename -- "$archive")"
}

release_set_before() {
  local file="$1" ver="$2" tmp
  tmp=$(mktemp)
  sed -E -e "s/^(Releases before )v[0-9]+\.[0-9]+\.[0-9]+/\1$ver/" \
    -e "/^Releases before /s/(Everything from )v[0-9]+\.[0-9]+\.[0-9]+( on)/\1$ver\2/" "$file" > "$tmp"
  cat "$tmp" > "$file"
  rm -f "$tmp"
}

# release_sub applies a sed -E expression to a file and fails when the
# result does not contain the wanted text.
release_sub() {
  local file="$1" expr="$2" want="$3" tmp
  [ -f "$file" ] || release_die "$file not found"
  tmp=$(mktemp)
  sed -E "$expr" "$file" > "$tmp"
  if ! grep -Fq -- "$want" "$tmp"; then
    rm -f "$tmp"
    release_die "$file: could not set $want"
  fi
  cmp -s "$tmp" "$file" || cat "$tmp" > "$file"
  rm -f "$tmp"
}

release_bump() {
  local root="$1" num="$2"
  release_sub "$root/cmd/agnostic-ai/main.go" \
    "s/^var version = \"[^\"]*\"/var version = \"$num\"/" "var version = \"$num\""
  release_sub "$root/docs/site/config.toml" \
    "s/^version = \"v[^\"]*\"/version = \"v$num\"/" "version = \"v$num\""
  release_sub "$root/agnostic-ai.yaml" \
    "s/^requires: \">=[^\"]*\"/requires: \">=$num\"/" "requires: \">=$num\""
  release_sub "$root/agnostic-ai.yaml" \
    "s#(agnostic-ai/)v[0-9]+\.[0-9]+\.[0-9]+(/docs/schemas/)#\1v$num\2#" "agnostic-ai/v$num/docs/schemas/"
  release_step "version: set $num in main.go, config.toml and agnostic-ai.yaml"
}

# release_offset prints the local UTC offset on DATE as +HH:MM.
release_offset() {
  local off
  off=$(date -j -f '%Y-%m-%d %H:%M:%S' "$1 00:00:00" +%z 2>/dev/null) ||
    off=$(date -d "$1 00:00:00" +%z)
  printf '%s:%s' "${off%??}" "${off#???}"
}

# release_find_briefing prints the briefing file for VER, if there is one.
release_find_briefing() {
  local f
  for f in "$1"/docs/site/content/updates/*-"$2".md; do
    if [ -f "$f" ]; then printf '%s\n' "$f"; return 0; fi
  done
  return 0
}

release_previous() {
  awk -v ver="$2" '/^## v[0-9]+\.[0-9]+\.[0-9]+ - / { v = $2; if (v != ver) { print v; exit } }' "$1"
}

release_briefing() {
  local file="$1" ver="$2" date="$3" prev="$4" slug offset
  slug="$date-$ver"
  offset=$(release_offset "$date")
  cat > "$file" <<EOF
+++
title = "agnostic-ai $ver: TODO what you can now do"
description = "TODO one sentence naming the main capabilities."
date = ${date}T00:00:00$offset
slug = "$slug"
aliases = ["updates/$slug.html"]

[extra]
kind = "release"
version = "$ver"
dek = "TODO one sentence naming the main capabilities."
rss_guid = "https://agnostic-ai.org/updates/$slug.html"
archive_stats = "TODO theme · TODO theme · TODO theme"
targets = []

[[extra.signals]]
status = "shipped"
targets = ["agnostic-ai"]
title = "TODO highest-consequence change"
summary = "TODO what the reader gets"
+++

## Highlights {#highlights}

- TODO three to five bullets matching the changelog Highlights.

## Upgrading from ${prev:-vPREV} {#actions}

- TODO only when a reader must act; otherwise delete this section.

Then follow the [upgrade page](@/docs/cli-reference/upgrades.md#upgrade).

## What changed

### TODO reader outcome

TODO benefit, entry point, link.

See the [release notes](https://github.com/Chemaclass/agnostic-ai/releases/tag/$ver) for the remaining fixes.

## What changed in the targets

- TODO one bullet per target with its vendor source; otherwise delete this section.

Checked on $date against the linked vendor docs and releases; native behavior was not run.
EOF
}

# release_check_section prints one line per shape problem in the dated
# section and fails when there is any.
release_check_section() {
  local changelog="$1" ver="$2"
  awk -v head="## $ver - " '
    index($0, head) == 1 { p = 1; found = 1; next }
    p && /^## / { p = 0 }
    !p { next }
    function close_heading() {
      if (hline != "" && !hasentry) printf "empty heading: %s\n", hline
    }
    function close_parent() {
      if (h3line != "" && !h3entry) printf "empty heading: %s\n", h3line
    }
    /^### / {
      close_heading(); close_parent()
      h3line = $0; h3entry = 0; hline = $0; hasentry = 0; count = 0; next
    }
    /^#### / {
      if (hline != h3line) close_heading()
      hline = $0; hasentry = 0; count = 0; next
    }
    /^- / {
      hasentry = 1; h3entry = 1
      if (++count == 6) printf "over 5 bullets: %s\n", hline
      if (length($0) > 160) printf "over 160 characters (%d): %s\n", length($0), $0
      next
    }
    /[^[:space:]]/ { hasentry = 1; h3entry = 1 }
    END {
      if (!found) { printf "no \"## %s - \" section\n", substr(head, 4, length(head) - 6); exit }
      close_heading(); close_parent()
    }
  ' "$changelog"
}

# release_check checks the dated section and, unless only_changelog is 1,
# the briefing.
release_check() {
  local root="$1" ver="$2" only_changelog="$3" problems briefing
  problems=$(release_check_section "$root/CHANGELOG.md" "$ver")
  briefing=$(release_find_briefing "$root" "$ver")
  if [ "$only_changelog" = 1 ]; then
    :
  elif [ -z "$briefing" ]; then
    problems="${problems:+$problems
}no briefing docs/site/content/updates/*-$ver.md"
  elif grep -q 'TODO' "$briefing"; then
    problems="${problems:+$problems
}briefing still has TODO: ${briefing#"$root"/}"
  fi
  if [ -n "$problems" ]; then
    printf '%s\n' "$problems" >&2
    return 1
  fi
  if [ "$only_changelog" = 1 ]; then
    release_step "release check: $ver section passes"
  else
    release_step "release check: $ver section and briefing pass"
  fi
}

release_prepare() {
  local check=0 only_changelog=0 git_check=1 root ver date num maj min existing briefing prev pending
  while [ $# -gt 0 ]; do
    case "$1" in
      --check) check=1; shift ;;
      --check-changelog) check=1; only_changelog=1; shift ;;
      --skip-git-check) git_check=0; shift ;;
      -*) release_die "unknown flag $1" ;;
      *) break ;;
    esac
  done
  ver="${1:-}"
  date="${2:-$(date +%Y-%m-%d)}"
  root="${RELEASE_ROOT:-$(CDPATH='' cd -- "$RELEASE_SCRIPTS/.." && pwd)}"
  printf '%s\n' "$ver" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$' ||
    release_die "usage: release-prepare.sh [--check|--check-changelog|--skip-git-check] vX.Y.Z [YYYY-MM-DD]"
  printf '%s\n' "$date" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' || release_die "date $date is not YYYY-MM-DD"
  if [ "$check" = 1 ]; then
    release_check "$root" "$ver" "$only_changelog"
    return
  fi

  num=${ver#v}
  maj=${num%%.*}
  min=${num#*.}; min=${min%%.*}
  local changelog="$root/CHANGELOG.md"

  existing=$(release_section_date "$changelog" "$ver")
  if [ "$git_check" = 1 ]; then
    if [ -z "$existing" ]; then release_git_check "$root" 0; else release_git_check "$root" 1; fi
  fi
  if [ -z "$existing" ]; then
    "$RELEASE_SCRIPTS/changelog-assemble.sh" "$changelog" "$root/changes"
    release_rollover "$changelog" "$ver" "$date"
    release_step "changelog: moved [Unreleased] into ## $ver - $date"
  else
    [ "$existing" = "$date" ] || release_step "changelog: keeping the existing date $existing"
    date="$existing"
    [ -z "$(release_unreleased_body "$changelog" | tr -d '[:space:]')" ] ||
      release_die "## $ver exists and [Unreleased] has entries too; move them by hand"
    pending=$("$RELEASE_SCRIPTS/changelog-assemble.sh" --check "$changelog" "$root/changes")
    case "$pending" in
      "changelog: 0 "*) ;;
      *) release_die "## $ver exists and changes/ has entries too; fold them by hand" ;;
    esac
    release_step "changelog: ## $ver - $date already there, skipped"
  fi

  if [ $((min % 10)) -eq 1 ] && [ "$min" -gt 10 ]; then
    release_archive "$changelog" "$root/docs/CHANGELOG-archive.md" "$maj" $((min - 1))
  fi

  release_bump "$root" "$num"

  if git -C "$root" diff --quiet HEAD -- scripts/binary-size-budget 2>/dev/null; then
    make -C "$root" size-budget
  else
    release_step "size budget: already set, skipped"
  fi

  if [ -f "$root/scripts/target-audit/signals.tsv" ]; then
    "$RELEASE_SCRIPTS/signals-shipped.sh" "$changelog" "$root/scripts/target-audit/signals.tsv"
  fi

  briefing=$(release_find_briefing "$root" "$ver")
  if [ -n "$briefing" ]; then
    release_step "briefing: ${briefing#"$root"/} already there, skipped"
  else
    briefing="$root/docs/site/content/updates/$date-$ver.md"
    mkdir -p "$(dirname -- "$briefing")"
    prev=$(release_previous "$changelog" "$ver")
    release_briefing "$briefing" "$ver" "$date" "$prev"
    release_step "briefing: wrote ${briefing#"$root"/}"
  fi

  cat <<EOF

Left to do by hand:
  1. Curate "## $ver - $date" in CHANGELOG.md (.agnostic-ai/agents/changelog-curator.md).
  2. Review the rows set in: git diff -- scripts/target-audit/signals.tsv
  3. Write the prose in ${briefing#"$root"/} and replace every TODO.
  4. Run: make release-check VERSION=$ver
  5. Validate, commit, tag and verify as the cut-release skill says.
EOF
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  release_prepare "$@"
fi
