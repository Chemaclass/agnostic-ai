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
    !in_frontmatter { print }
  ' "$1" | sed -E 's|\]\(@/docs/([^)#]+)\.md(#[^)]+)?\)|](https://agnostic-ai.org/docs/\1/\2)|g'
}

render_doc "$docs_dir/agent-setup.md" > "$output_dir/agent-setup.txt"

revision=${GITHUB_SHA:-$(git -C "$root" rev-parse --verify HEAD 2>/dev/null || printf unknown)}
{
  echo "# agnostic-ai: full documentation"
  echo
  echo "Generated from docs/site/content/docs/ at ${revision}. Source of truth:"
  echo "https://github.com/Chemaclass/agnostic-ai"
  for doc in _index installation agent-setup getting-started migration troubleshooting \
             why spec-format targets target-updates configuration \
             cli-reference ci packs git-hooks graph errors \
             alternatives-why-not-symlinks; do
    echo
    echo "---"
    echo
    render_doc "$docs_dir/${doc}.md"
  done
} > "$output_dir/llms-full.txt"
