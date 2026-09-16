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
  committed_date=$(git log -1 --format=%cs -- "$@")
  if [[ -n "$committed_date" ]]; then
    printf '%s\n' "$committed_date"
    return
  fi
  date -u +%F
}

write_url() {
  local location=$1
  local frequency=$2
  local priority=$3
  shift 3

  printf '  <url>\n'
  printf '    <loc>%s%s</loc>\n' "$site_url" "$location"
  printf '    <lastmod>%s</lastmod>\n' "$(last_modified "$@")"
  printf '    <changefreq>%s</changefreq>\n' "$frequency"
  printf '    <priority>%s</priority>\n' "$priority"
  printf '  </url>\n'
}

{
  printf '%s\n' '<?xml version="1.0" encoding="UTF-8"?>'
  printf '%s\n' '<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">'
  write_url / weekly 1.0 \
    docs/site/content/_index.md docs/site/data/landing.toml \
    docs/site/templates/base.html docs/site/templates/index.html \
    docs/site/static/assets/styles/base.css docs/site/static/assets/styles/landing.css \
    docs/site/static/assets/scripts/theme.js docs/site/static/assets/scripts/landing.js
  write_url /playground/ weekly 0.8 \
    docs/playground/index.html docs/playground/playground.js docs/playground/style.css \
    docs/site/static/assets/styles/base.css docs/site/static/assets/scripts/theme.js
  write_url /docs/ weekly 0.9 \
    docs/site/content/docs/_index.md docs/site/content/docs/[!_]*.md \
    docs/site/templates/base.html docs/site/templates/docs/section.html \
    docs/site/templates/docs/page.html docs/site/templates/macros/docs.html \
    docs/site/static/assets/styles/base.css docs/site/static/assets/styles/docs.css \
    docs/site/static/assets/scripts/theme.js
  write_url /updates/ weekly 0.9 \
    docs/site/content/updates/_index.md docs/site/content/updates/[0-9]*.md \
    docs/site/data/updates.toml \
    docs/site/templates/base.html docs/site/templates/updates/section.html \
    docs/site/static/assets/styles/base.css docs/site/static/assets/styles/updates.css \
    docs/site/static/assets/scripts/theme.js docs/site/static/assets/scripts/updates.js

  for article in docs/site/content/updates/[0-9]*.md; do
    [[ -e "$article" ]] || continue
    article_name=${article##*/}
    article_slug=${article_name%.md}
    write_url "/updates/${article_slug}/" never 0.7 "$article"
  done

  for guide in docs/site/content/docs/[!_]*.md; do
    [[ -e "$guide" ]] || continue
    guide_name=${guide##*/}
    guide_slug=${guide_name%.md}
    write_url "/docs/${guide_slug}/" monthly 0.8 "$guide"
  done

  printf '%s\n' '</urlset>'
} > "$output_file"
