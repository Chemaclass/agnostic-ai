#!/usr/bin/env python3
"""Fetch pinned release assets into an explicit scratch directory."""

import argparse
import hashlib
import json
from pathlib import Path
import sys
import urllib.request


RTK_ARCHIVE = "rtk-aarch64-unknown-linux-gnu.tar.gz"
RTK_SHA256 = "8d6d1aad9e69b42481eda7039507d1f7ee93698f87713cecd873d287c1931632"


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def asset_named(metadata, name):
    matches = [asset for asset in metadata.get("assets", []) if asset.get("name") == name]
    require(len(matches) == 1, f"release must contain exactly one asset named {name}")
    return matches[0]


def verify_digest(data, asset):
    actual = hashlib.sha256(data).hexdigest()
    require(asset.get("digest") == "sha256:" + actual, f"{asset.get('name')}: GitHub asset digest mismatch")
    return actual


def download(url):
    request = urllib.request.Request(url, headers={"User-Agent": "agnostic-ai-runtime-fixture"})
    try:
        with urllib.request.urlopen(request, timeout=60) as response:
            return response.read()
    except Exception as error:
        raise RuntimeError(f"download {url}: {error}") from error


def fetch_release(root, repo, tag, filename):
    data = download(f"https://api.github.com/repos/{repo}/releases/tags/{tag}")
    metadata = json.loads(data)
    require(metadata.get("tag_name") == tag, f"{repo}: expected release {tag}")
    (root / filename).write_bytes(data)
    return metadata


def fetch_asset(root, metadata, name, destination):
    asset = asset_named(metadata, name)
    url = asset.get("browser_download_url", "")
    require(url.startswith("https://github.com/"), f"{name}: missing official download URL")
    data = download(url)
    verify_digest(data, asset)
    (root / destination).write_bytes(data)
    return data


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True, help="scratch directory for downloads")
    root = parser.parse_args().output.resolve()
    require(root != Path(__file__).resolve().parent, "output must be a scratch directory, not the helper directory")
    root.mkdir(parents=True, exist_ok=True)
    rtk = fetch_release(root, "rtk-ai/rtk", "v0.51.0", "rtk-release.json")
    archive = fetch_asset(root, rtk, RTK_ARCHIVE, RTK_ARCHIVE)
    require(hashlib.sha256(archive).hexdigest() == RTK_SHA256, f"{RTK_ARCHIVE}: pinned digest mismatch")
    checksums = fetch_asset(root, rtk, "checksums.txt", "checksums.txt")
    matches = [line.split()[0] for line in checksums.decode().splitlines()
               if len(line.split()) == 2 and line.split()[1].lstrip("*") == RTK_ARCHIVE]
    require(matches == [RTK_SHA256], f"checksums.txt: missing or mismatched {RTK_ARCHIVE}")
    caveman = fetch_release(root, "JuliusBrussee/caveman", "bin-v2.1.0", "caveman-release.json")
    for name in ("checksums.txt", "checksums.txt.keysig", "RELEASE"):
        fetch_asset(root, caveman, name, "caveman-" + name)
    print(f"Verified pinned release assets in {root}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError) as error:
        print(error, file=sys.stderr)
        sys.exit(1)
