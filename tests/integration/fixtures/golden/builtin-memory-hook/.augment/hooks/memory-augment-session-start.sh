#!/bin/sh
dir=$(pwd)
until [ -x "$dir/node_modules/.bin/agnostic-ai" ] || [ -f "$dir/agnostic-ai.yaml" ] ||
  [ -f "$dir/agnostic.config.yaml" ] || [ "$dir" = "$(dirname "$dir")" ]; do
  dir=$(dirname "$dir")
done
bin=$dir/node_modules/.bin/agnostic-ai
[ -x "$bin" ] || bin=agnostic-ai
command -v "$bin" >/dev/null || exit 0
AGNOSTIC_AI_NO_UPDATE_CHECK=1 "$bin" project --help >/dev/null 2>&1 || {
  echo "agnostic-ai: this version has no project command; upgrade to 0.83.0 or newer" >&2
  exit 1
}
exec "$bin" project -- hook memory --target augment
