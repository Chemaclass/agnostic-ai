#!/usr/bin/env bash
#
# npm-smoke.sh - prove the npm packages resolve, without publishing anything.
#
# Usage:
#   scripts/npm-smoke.sh
#
# Cross-compiles the six binaries, generates the six platform packages, packs
# all seven, installs the parent plus this machine's platform package from the
# tarballs, and runs the result. That is the whole install path a user takes,
# minus the registry.
#
# Nothing here touches the network and nothing is published. The working tree
# is left as it was: the generator writes npm/package.json at the version it
# already carries, so the file does not change.

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BINARY="agnostic-ai"

TARGETS=(
  darwin_amd64
  darwin_arm64
  linux_amd64
  linux_arm64
  windows_amd64
  windows_arm64
)

note() { printf 'npm-smoke: %s\n' "$*"; }

build_binaries() {
  local dest="$1" target goos goarch out
  for target in "${TARGETS[@]}"; do
    goos="${target%_*}"
    goarch="${target#*_}"
    out="$dest/$target/$BINARY"
    [[ "$goos" == "windows" ]] && out="$out.exe"
    mkdir -p "$(dirname "$out")"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -trimpath -ldflags="-s -w" -o "$out" "$ROOT/cmd/$BINARY"
  done
}

# npm's own spelling of this machine, which is the platform package to install
# next to the parent.
host_package() {
  node -e '
    const { packageName, platformFor } = require(process.argv[1])
    const p = platformFor(process.platform, process.arch)
    if (!p) { console.error(`no platform package for ${process.platform}/${process.arch}`); process.exit(1) }
    console.log(packageName(p))
  ' "$ROOT/npm/lib/platforms.js"
}

main() {
  local work version host dir tarballs=()
  work="$(mktemp -d)"
  if [[ -z "$work" || ! -d "$work" ]]; then
    note "could not create a work directory"
    return 1
  fi

  version="$(node -p "require('$ROOT/npm/package.json').version")"
  note "building six binaries"
  build_binaries "$work/binaries"

  note "generating the platform packages"
  node "$ROOT/npm/scripts/build-platform-packages.js" --binaries "$work/binaries"

  note "packing all seven"
  for dir in "$ROOT"/npm/platforms/*/; do
    npm pack --pack-destination "$work" --silent "${dir%/}" >/dev/null
  done
  npm pack --pack-destination "$work" --silent "$ROOT/npm" >/dev/null

  # npm folds the scope into the tarball name: @agnostic-ai/darwin-arm64 packs
  # as agnostic-ai-darwin-arm64-<version>.tgz.
  host="$(host_package)"
  host="${host#@}"
  host="${host/\//-}"
  tarballs=("$work/${host}-${version}.tgz" "$work/agnostic-ai-${version}.tgz")

  note "installing ${tarballs[*]}"
  mkdir -p "$work/project"
  (cd "$work/project" && npm install --silent --no-audit --no-fund "${tarballs[@]}")

  note "resolved binary:"
  "$work/project/node_modules/.bin/$BINARY" --version

  rm -rf "$work"
  note "ok"
}

if [[ -z "${BASH_SOURCE[0]:-}" || "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
