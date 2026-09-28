#!/usr/bin/env bash
# Renders docs/site/og/card.html to the social preview PNG with headless Chrome.
# Platforms cache og:image by URL, so a new design ships under a new file name:
# pass it as the argument and point extra.image_url in docs/site/config.toml at it.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
source_file="$root/docs/site/og/card.html"
output_file=${1:-"$root/docs/site/static/og-v2.png"}

find_chrome() {
  if [[ -n "${CHROME:-}" ]]; then
    printf '%s\n' "$CHROME"
    return
  fi
  local candidate
  for candidate in \
    "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
    "/Applications/Chromium.app/Contents/MacOS/Chromium"; do
    if [[ -x "$candidate" ]]; then
      printf '%s\n' "$candidate"
      return
    fi
  done
  for candidate in google-chrome google-chrome-stable chromium chromium-browser; do
    if command -v "$candidate" >/dev/null 2>&1; then
      command -v "$candidate"
      return
    fi
  done
  echo "Chrome or Chromium not found. Set CHROME to its binary." >&2
  exit 1
}

chrome=$(find_chrome)
targets=$(grep -c '^\[\[targets\]\]' "$root/docs/site/data/capabilities.toml")

profile=$(mktemp -d)
screenshot="$profile/card.png"
log="$profile/chrome.log"
chrome_pid=""

cleanup() {
  if [[ -n "$chrome_pid" ]]; then
    kill "$chrome_pid" 2>/dev/null || true
    wait "$chrome_pid" 2>/dev/null || true
  fi
  rm -rf "$profile"
}
trap cleanup EXIT

"$chrome" --headless=new --disable-gpu --hide-scrollbars --no-first-run \
  --no-default-browser-check --disable-background-networking \
  --disable-component-update --disable-extensions --disable-sync \
  --use-mock-keychain --password-store=basic --force-device-scale-factor=1 \
  --user-data-dir="$profile" --window-size=1200,630 \
  --screenshot="$screenshot" "file://$source_file?targets=$targets" >"$log" 2>&1 &
chrome_pid=$!

# Some Chrome builds keep running after they write the screenshot, so wait for
# the file instead of for the process.
for _ in $(seq 1 60); do
  if grep -q 'bytes written to file' "$log" || ! kill -0 "$chrome_pid" 2>/dev/null; then
    break
  fi
  sleep 1
done

if [[ ! -s "$screenshot" ]]; then
  echo "Chrome did not write the screenshot:" >&2
  tail -n 20 "$log" >&2
  exit 1
fi

mv "$screenshot" "$output_file"
printf 'wrote %s (%s targets)\n' "$output_file" "$targets"
