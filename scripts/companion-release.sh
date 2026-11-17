#!/usr/bin/env bash

# Usage: scripts/companion-release.sh <version> <outdir>
#
# Builds the macOS companion for Apple silicon and Intel and packs each as
# <outdir>/agnostic-ai-companion_darwin_<arch>.tar.gz, the names
# `agnostic-ai companion install` downloads. Runs on macOS with the Xcode
# Command Line Tools: the companion draws its windows through C.

set -Eeuo pipefail

version="${1:?usage: $0 <version> <outdir>}"
outdir="${2:?usage: $0 <version> <outdir>}"
root="$(cd "$(dirname "$0")/.." && pwd)"
name=agnostic-ai-companion

# Without this the C parts target the build machine's own macOS, and the
# program refuses any older one. 13 is the oldest the Go toolchain in
# companion/go.mod supports. Go keys its build cache on CGO_CFLAGS, not on
# MACOSX_DEPLOYMENT_TARGET.
min_macos=13.0
export MACOSX_DEPLOYMENT_TARGET="$min_macos"
export CGO_CFLAGS="-O2 -g -mmacosx-version-min=$min_macos"
export CGO_LDFLAGS="-O2 -g -mmacosx-version-min=$min_macos"

mkdir -p "$outdir"
outdir="$(cd "$outdir" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

for arch in arm64 amd64; do
  mkdir -p "$work/$arch"
  (
    cd "$root/companion"
    CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" go build \
      -tags "production private_mac_apis" -trimpath \
      -ldflags "-s -w -X main.version=$version" \
      -o "$work/$arch/$name" .
  )
  want="$arch"
  [ "$arch" = amd64 ] && want=x86_64
  got="$(lipo -archs "$work/$arch/$name")"
  if [ "$got" != "$want" ]; then
    echo "$name for $arch is built for $got" >&2
    exit 1
  fi
  if [ "$(uname -m)" = "$arch" ] || { [ "$arch" = amd64 ] && [ "$(uname -m)" = x86_64 ]; }; then
    said="$("$work/$arch/$name" --version)"
    if [ "$said" != "$version" ]; then
      echo "$name for $arch says it is $said, not $version" >&2
      exit 1
    fi
  fi
  tar -czf "$outdir/${name}_darwin_${arch}.tar.gz" -C "$work/$arch" "$name"
  echo "$outdir/${name}_darwin_${arch}.tar.gz"
done
