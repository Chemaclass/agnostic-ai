#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <output-file>" >&2
  exit 2
fi

output_file=$1
site_url=https://chemaclass.github.io/agnostic-ai

last_modified() {
  local committed_date
  committed_date=$(git log -1 --format=%cs -- "$1")
  if [[ -n "$committed_date" ]]; then
    printf '%s\n' "$committed_date"
    return
  fi
  date -u +%F
}

write_url() {
  local location=$1
  local source_file=$2
  local frequency=$3
  local priority=$4

  printf '  <url>\n'
  printf '    <loc>%s%s</loc>\n' "$site_url" "$location"
  printf '    <lastmod>%s</lastmod>\n' "$(last_modified "$source_file")"
  printf '    <changefreq>%s</changefreq>\n' "$frequency"
  printf '    <priority>%s</priority>\n' "$priority"
  printf '  </url>\n'
}

{
  printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>'
  printf '%s\n' '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'
  write_url / docs/site/index.html weekly 1.0
  write_url /playground/ docs/playground/index.html weekly 0.8
  write_url /updates/ docs/site/updates/index.html weekly 0.9

  for article in docs/site/updates/[0-9]*.html; do
    [[ -e "$article" ]] || continue
    write_url "/updates/${article##*/}" "$article" never 0.7
  done

  printf '%s\n' '</urlset>'
} > "$output_file"
