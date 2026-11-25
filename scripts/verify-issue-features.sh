#!/usr/bin/env bash
set -euo pipefail

feature_root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
feature_map="$feature_root/.github/issue-features.json"
repository=''
labels_file=''
issues_file=''
created_from=''
created_before=''

usage() {
  cat <<'EOF'
Usage: scripts/verify-issue-features.sh [options]

Read-only verification of label definitions and one feature per selected issue.
Prints a JSON report. Exit 1 means mismatched definitions, invalid classification
or an empty selection. Empty labels and concentration above 25% are review notes.

  --repo OWNER/REPO             Default: the checkout's GitHub repository
  --created-from UTC_TIMESTAMP  Include issues created at or after this time
  --created-before UTC_TIMESTAMP Exclude issues created at or after this time
  --labels-file FILE            Use a JSON label array instead of GitHub
  --issues-file FILE            Use a JSON issue array instead of GitHub
  --help                       Show this help

Timestamps use YYYY-MM-DDTHH:MM:SSZ. Issue files accept REST or gh issue list
fields. Pull requests are excluded. With both files supplied, gh is not needed.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --help) usage; exit 0 ;;
    --repo|--created-from|--created-before|--labels-file|--issues-file)
      if [ "$#" -lt 2 ] || [ -z "$2" ]; then
        printf 'verify-issue-features: %s needs a value\n' "$1" >&2
        exit 2
      fi
      case "$1" in
        --repo) repository=$2 ;;
        --created-from) created_from=$2 ;;
        --created-before) created_before=$2 ;;
        --labels-file) labels_file=$2 ;;
        --issues-file) issues_file=$2 ;;
      esac
      shift 2 ;;
    *) printf 'verify-issue-features: unknown option %s\n' "$1" >&2; exit 2 ;;
  esac
done

if ! command -v jq >/dev/null 2>&1; then
  printf 'verify-issue-features: jq is required\n' >&2
  exit 2
fi
jq -en --arg start "$created_from" --arg end "$created_before" '
  def timestamp: if type == "string" then . as $value | try ((fromdateiso8601 | todateiso8601) == $value) catch false else false end;
  ($start == "" or ($start | timestamp)) and
  ($end == "" or ($end | timestamp)) and
  ($start == "" or $end == "" or $start < $end)
' >/dev/null || { printf 'verify-issue-features: invalid UTC interval\n' >&2; exit 2; }

feature_tmp=$(mktemp -d)
trap 'rm -f "$feature_tmp/labels.json" "$feature_tmp/issues.json" "$feature_tmp/report.json"; rmdir "$feature_tmp"' EXIT
if [ -z "$labels_file" ] || [ -z "$issues_file" ]; then
  if ! command -v gh >/dev/null 2>&1; then
    printf 'verify-issue-features: gh is required for live reads\n' >&2
    exit 2
  fi
  if [ -z "$repository" ]; then
    repository=$(cd "$feature_root" && gh repo view --json nameWithOwner --jq .nameWithOwner)
  fi
  if [[ ! "$repository" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
    printf 'verify-issue-features: expected --repo OWNER/REPO\n' >&2
    exit 2
  fi
fi
if [ -z "$labels_file" ]; then
  labels_file="$feature_tmp/labels.json"
  gh api --paginate --slurp "repos/$repository/labels?per_page=100" | jq 'add' >"$labels_file"
fi
if [ -z "$issues_file" ]; then
  issues_file="$feature_tmp/issues.json"
  gh api --paginate --slurp "repos/$repository/issues?state=all&per_page=100" | jq 'add' >"$issues_file"
fi

jq -n --slurpfile map "$feature_map" --slurpfile labels "$labels_file" --slurpfile issues "$issues_file" \
  --arg start "$created_from" --arg end "$created_before" '
  def names: map(if type == "string" then . else .name end);
  def is_pr: .pull_request != null or .isPullRequest == true or .__typename == "PullRequest" or ((.html_url // .url // "") | test("/pull/[0-9]+$"));
  def created: .createdAt // .created_at;
  def timestamp: if type == "string" then . as $value | try ((fromdateiso8601 | todateiso8601) == $value) catch false else false end;
  $map[0] as $map |
  if ($map.groups | type) != "array" or ($map.groups | length) == 0 then error("feature map needs groups") else . end |
  [$map.groups[].id] as $groups |
  [$map.groups[] | .id as $group | .features[] |
    if (.label | startswith($group + ":")) and (.scope | type == "string" and length > 0) and
       (.paths | type == "array" and length > 0 and all(.[]; type == "string" and length > 0))
    then .label else error("invalid feature definition") end] as $allowed |
  if ($groups | unique | length) != ($groups | length) or ($allowed | unique | length) != ($allowed | length)
  then error("duplicate feature or group") else . end |
  if ($labels | length) != 1 or ($labels[0] | type) != "array" or ($issues | length) != 1 or ($issues[0] | type) != "array"
  then error("label and issue inputs must each contain one JSON array") else . end |
  def feature_namespace: . as $label | any($groups[]; . as $group | $label | startswith($group + ":"));
  ($labels[0] | names) as $defined |
  [$issues[0][] | select(is_pr | not) |
    if ($start != "" or $end != "") and (created | timestamp | not)
    then error("issue #\(.number) needs a canonical UTC creation time for date selection") else . end |
    select(($start == "" or created >= $start) and ($end == "" or created < $end)) |
    if (.number | type) != "number" or (.labels | type) != "array" then error("issue needs number and labels") else . end |
    (.labels | names) as $names |
    {number, title, url: (.html_url // .url),
     features: [$names[] | select(. as $name | $allowed | index($name))] | unique,
     unknown: [$names[] | select(feature_namespace) | select(. as $name | $allowed | index($name) | not)] | unique}
  ] as $selected |
  [$allowed[] | . as $label |
    ([$selected[] | select(.features | index($label))] | length) as $count |
    {label: $label, issues: $count, share: (if ($selected | length) > 0 then $count / ($selected | length) else 0 end)}] as $counts |
  {groups: ($groups | length), features: ($allowed | length),
   interval: {from: $start, before: $end},
   selectedIssues: ($selected | length), excludedPullRequests: ([$issues[0][] | select(is_pr)] | length),
   definitions: {missing: ($allowed - $defined), unexpected: ([$defined[] | select(feature_namespace)] - $allowed)},
   unmapped: [$selected[] | select(.features | length == 0) | del(.unknown)],
   multiple: [$selected[] | select(.features | length > 1) | del(.unknown)],
   unknownFeatures: [$selected[] | select(.unknown | length > 0)],
   counts: $counts,
   emptyLabels: [$counts[] | select(.issues == 0) | .label],
   concentration: [$counts[] | select(.share > 0.25)]} |
  .valid = (.selectedIssues > 0 and (.definitions.missing | length) == 0 and (.definitions.unexpected | length) == 0 and
    (.unmapped | length) == 0 and (.multiple | length) == 0 and (.unknownFeatures | length) == 0)
' >"$feature_tmp/report.json"
cat "$feature_tmp/report.json"
jq -e .valid "$feature_tmp/report.json" >/dev/null
