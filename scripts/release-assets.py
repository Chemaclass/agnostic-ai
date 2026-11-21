#!/usr/bin/env python3
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import sys


CLI = {
    f"agnostic-ai_{os}_{arch}.{extension}"
    for os, extension in (("darwin", "tar.gz"), ("linux", "tar.gz"), ("windows", "zip"))
    for arch in ("amd64", "arm64")
}
CLI_ASSETS = CLI | {f"{name}.sbom.json" for name in CLI}
COMPANION = {
    f"agnostic-ai-companion_darwin_{arch}.{extension}"
    for arch in ("amd64", "arm64")
    for extension in ("tar.gz", "dmg")
}
CHANNELS = {
    "homebrew": {"agnostic-ai.rb"},
    "scoop": {"agnostic-ai.json"},
    "winget": {
        "Chemaclass.agnostic-ai.yaml",
        "Chemaclass.agnostic-ai.installer.yaml",
        "Chemaclass.agnostic-ai.locale.en-US.yaml",
    },
}
MANIFESTS = set().union(*CHANNELS.values())
CHECKSUMS = "checksums.txt"
BUNDLE = "agnostic-ai.intoto.jsonl"


def prerelease(tag):
    version = tag.split("+", 1)[0]
    return "-" in version


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def check_inventory(found, required, optional=frozenset()):
    missing = required - found
    if missing:
        raise ValueError("missing " + ", ".join(sorted(missing)))
    unexpected = found - required - optional
    if unexpected:
        raise ValueError("unexpected " + ", ".join(sorted(unexpected)))


def files(directory):
    paths = {}
    for path in directory.iterdir():
        if not path.is_file() or path.is_symlink() or path.stat().st_size == 0:
            raise ValueError(f"{path}: expected a nonempty regular file")
        paths[path.name] = path
    return paths


def read_checksums(path):
    digests = {}
    for line in path.read_text(encoding="ascii").splitlines():
        match = re.fullmatch(r"([a-f0-9]{64}) [ *]([\w.-]+)", line, re.ASCII)
        if match is None:
            raise ValueError(f"{path}: invalid checksum line {line!r}")
        digest, name = match.groups()
        if name in digests:
            raise ValueError(f"{path}: duplicate checksum for {name}")
        digests[name] = digest
    return digests


def validate(tag, directory, sealed=False):
    required = CLI_ASSETS | {CHECKSUMS}
    if sealed:
        required.add(BUNDLE)
    if not prerelease(tag):
        required.update(COMPANION)
    assets = files(directory / "assets")
    check_inventory(set(assets), required, COMPANION)
    channels = files(directory / "channels")
    check_inventory(set(channels), MANIFESTS)
    digests = read_checksums(assets[CHECKSUMS])
    names = set(assets) - {CHECKSUMS, BUNDLE}
    try:
        check_inventory(set(digests), names)
    except ValueError as error:
        raise ValueError(f"{CHECKSUMS}: {error}") from error
    for name in sorted(names):
        if sha256(assets[name]) != digests[name]:
            raise ValueError(f"checksum mismatch for {name}")
    return assets, channels


def stage(tag, dist, companion, directory):
    assets = directory / "assets"
    channels = directory / "channels"
    assets.mkdir(parents=True)
    channels.mkdir()
    for source, names, destination in (
        (dist, CLI_ASSETS | {CHECKSUMS}, assets),
        (companion, COMPANION, assets),
        (dist, MANIFESTS, channels),
    ):
        for path in sorted(source.rglob("*")):
            if path.name not in names:
                continue
            output = destination / path.name
            if output.exists() or not path.is_file() or path.is_symlink():
                raise ValueError(f"{path}: expected one regular file for {path.name}")
            shutil.copyfile(path, output)
    validate(tag, directory)


def verify_provenance(paths, bundle, tag, repo, commit):
    for name, path in sorted(paths.items()):
        try:
            gh(
                "attestation", "verify", str(path), "--bundle", str(bundle),
                "--repo", repo, "--signer-workflow", f"{repo}/.github/workflows/release.yml",
                "--source-ref", f"refs/tags/{tag}", "--source-digest", commit,
                "--deny-self-hosted-runners",
            )
        except ValueError as error:
            raise ValueError(f"{name}: {error}") from error
        print(f"Verified release provenance for {name}", flush=True)


def gh(*args):
    result = subprocess.run(
        ["gh", *args], check=False, text=True, capture_output=True,
    )
    if result.returncode:
        raise ValueError(result.stderr.strip() or f"gh {args[0]} failed")
    return result.stdout


def release_info(tag, repo):
    try:
        return json.loads(gh(
            "release", "view", tag, "--repo", repo,
            "--json", "assets,isDraft,isPrerelease,tagName",
        ))
    except ValueError as error:
        if str(error) != "release not found":
            raise
        return None


def missing_uploads(release, expected):
    uploaded = {}
    for asset in release["assets"]:
        name = asset["name"]
        if name in uploaded:
            raise ValueError(f"duplicate GitHub asset {name}")
        uploaded[name] = asset
    check_inventory(set(uploaded), set(), set(expected))
    for name, asset in uploaded.items():
        if asset["state"] != "uploaded" or asset["digest"] != expected[name]:
            raise ValueError(f"GitHub has different bytes for {name}")
    return sorted(set(expected) - set(uploaded))


def publish(tag, directory, repo, commit, notes):
    assets, _ = validate(tag, directory, sealed=True)
    bundle = assets[BUNDLE]
    verify_provenance(
        {name: path for name, path in assets.items() if name != BUNDLE},
        bundle, tag, repo, commit,
    )
    if gh("api", f"repos/{repo}/commits/{tag}", "--jq", ".sha").strip() != commit:
        raise ValueError(f"{tag} no longer resolves to {commit}")
    expected = {name: f"sha256:{sha256(path)}" for name, path in assets.items()}
    release = release_info(tag, repo)
    if release is None:
        extra = ["--prerelease", "--latest=false"] if prerelease(tag) else []
        gh(
            "release", "create", tag, "--repo", repo, "--draft", "--verify-tag",
            "--title", tag, "--notes-file", str(notes), *extra,
        )
        release = release_info(tag, repo)
    if release is None or release["tagName"] != tag or release["isPrerelease"] != prerelease(tag):
        raise ValueError(f"unexpected GitHub release for {tag}")
    missing = missing_uploads(release, expected)
    if not release["isDraft"]:
        if missing:
            raise ValueError(f"published release is missing {', '.join(missing)}")
        print(f"{tag} is already public with the verified assets")
        return
    if missing:
        gh("release", "upload", tag, "--repo", repo, *(str(assets[name]) for name in missing))
    release = release_info(tag, repo)
    if release is None or not release["isDraft"] or missing_uploads(release, expected):
        raise ValueError(f"draft {tag} does not contain all verified assets")
    gh("release", "edit", tag, "--repo", repo, "--draft=false", "--notes-file", str(notes))
    print(f"Published {tag} with {len(expected)} verified assets")


def main():
    parser = argparse.ArgumentParser(description="Stage, verify, or publish release assets")
    commands = parser.add_subparsers(dest="command", required=True)
    staging = commands.add_parser("stage")
    staging.add_argument("tag")
    staging.add_argument("dist", type=Path)
    staging.add_argument("companion", type=Path)
    staging.add_argument("directory", type=Path)
    for name in ("subjects", "inventory", "publish", "verify-channel"):
        command = commands.add_parser(name)
        command.add_argument("tag")
        command.add_argument("directory", type=Path)
        if name in ("publish", "verify-channel"):
            command.add_argument("repo")
            command.add_argument("commit")
        if name == "publish":
            command.add_argument("notes", type=Path)
        if name == "verify-channel":
            command.add_argument("channel", choices=CHANNELS)
    args = parser.parse_args()
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", args.tag):
        parser.error(f"invalid release tag {args.tag!r}")
    if args.command == "stage":
        stage(args.tag, args.dist, args.companion, args.directory)
    elif args.command == "publish":
        publish(args.tag, args.directory, args.repo, args.commit, args.notes)
    elif args.command == "verify-channel":
        assets, channels = validate(args.tag, args.directory, sealed=True)
        requested = {name: channels[name] for name in CHANNELS[args.channel]}
        verify_provenance(requested, assets[BUNDLE], args.tag, args.repo, args.commit)
    else:
        assets, channels = validate(args.tag, args.directory, sealed=args.command == "inventory")
        if args.command == "inventory":
            for name in sorted(assets):
                print(name)
        else:
            for name, path in sorted((assets | channels).items()):
                print(f"{sha256(path)}  {name}")


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError) as error:
        sys.exit(f"release assets: {error}")
