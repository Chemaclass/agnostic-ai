#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <site-output-dir>" >&2
  exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
output_dir=$1
docs_dir="$root/docs/site/content/docs"

mkdir -p "$output_dir"

render_doc() {
  awk '
    NR == 1 && $0 == "+++" { in_frontmatter = 1; next }
    in_frontmatter && $0 == "+++" { in_frontmatter = 0; next }
    !in_frontmatter && $0 ~ /^\{\{ <[a-z_]+ \/> \}\}$/ { next }
    !in_frontmatter { print }
  ' "$1" | sed -E \
    -e 's|\]\(@/docs/([^)#]+)/_index\.md(#[^)]+)?\)|](https://agnostic-ai.org/docs/\1/\2)|g' \
    -e 's|\]\(@/docs/([^)#]+)\.md(#[^)]+)?\)|](https://agnostic-ai.org/docs/\1/\2)|g'
}

# render_section prints a section index, then every page in it in weight order.
render_section() {
  render_doc "$docs_dir/$1/_index.md"
  for page in $(awk -F' = ' 'FNR == 1 { w = "" } /^weight = / && w == "" { w = $2; print w, FILENAME }' "$docs_dir"/"$1"/[!_]*.md | sort -n | cut -d' ' -f2); do
    echo
    render_doc "$page"
  done
}

render_doc "$docs_dir/agent-setup.md" > "$output_dir/agent-setup.txt"

revision=${GITHUB_SHA:-$(git -C "$root" rev-parse --verify HEAD 2>/dev/null || printf unknown)}
{
  echo "# agnostic-ai: full documentation"
  echo
  echo "Generated from docs/site/content/docs/ at ${revision}. Source of truth:"
  echo "https://github.com/Chemaclass/agnostic-ai"
  for doc in _index installation agent-setup getting-started migration troubleshooting \
             trace spec-format targets target-updates configuration \
             cli-reference ci packs git-hooks graph errors \
             why-agnostic-ai; do
    echo
    echo "---"
    echo
    if [[ $doc == targets || $doc == cli-reference || $doc == spec-format ]]; then
      render_section "$doc"
    else
      render_doc "$docs_dir/${doc}.md"
    fi
  done
} > "$output_dir/llms-full.txt"
