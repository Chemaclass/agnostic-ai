#!/usr/bin/env python3
"""Publish a verified release's generated package-manager manifests."""

import argparse
import base64
import json
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import quote, urlencode


POINTER_CHANNELS = {
    "homebrew": ("Chemaclass/homebrew-tap", "master", "Casks/agnostic-ai.rb"),
    "scoop": ("Chemaclass/scoop-bucket", "main", "agnostic-ai.json"),
}
WINGET_FORK = "Chemaclass/winget-pkgs"
WINGET_UPSTREAM = "microsoft/winget-pkgs"
WINGET_BASE = "master"
WINGET_PACKAGE = "Chemaclass.agnostic-ai"
WINGET_FILES = tuple(WINGET_PACKAGE + suffix for suffix in (
    ".yaml", ".installer.yaml", ".locale.en-US.yaml",
))


def version_order(version):
    match = re.fullmatch(
        r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
        r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
        r"(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?",
        version,
    )
    if not match:
        raise ValueError(f"invalid release version {version!r}")
    major, minor, patch, prerelease = match.groups()
    identifiers = []
    for identifier in prerelease.split(".") if prerelease else ():
        if identifier.isdigit():
            if identifier != str(int(identifier)):
                raise ValueError(f"invalid numeric prerelease identifier in {version!r}")
            identifiers.append((0, int(identifier)))
        else:
            identifiers.append((1, identifier))
    return int(major), int(minor), int(patch), not prerelease, tuple(identifiers)


def api(endpoint, payload=None, *, allow_missing=False):
    command = ["gh", "api", "--method", "GET" if payload is None else "POST", endpoint]
    if payload is not None:
        command.extend(["--input", "-"])
    result = subprocess.run(
        command, input=None if payload is None else json.dumps(payload),
        capture_output=True, text=True, timeout=60, check=False,
    )
    if result.returncode:
        if allow_missing and "(HTTP 404)" in result.stderr:
            return None
        raise RuntimeError(f"{endpoint}: {result.stderr.strip() or 'GitHub request failed'}")
    response = json.loads(result.stdout)
    if isinstance(response, dict) and response.get("errors"):
        raise RuntimeError(f"{endpoint}: {response['errors'][0]['message']}")
    return response


def head_oid(repository, branch, *, allow_missing=False):
    encoded = quote(branch, safe="/")
    ref = api(f"repos/{repository}/git/ref/heads/{encoded}", allow_missing=allow_missing)
    return None if ref is None else ref["object"]["sha"]


def read_file(repository, commit, path):
    ref = urlencode({"ref": commit})
    entry = api(f"repos/{repository}/contents/{quote(path)}?{ref}", allow_missing=True)
    if entry is None:
        return None
    if not isinstance(entry, dict) or entry.get("type") != "file" or entry.get("encoding") != "base64":
        raise ValueError(f"{repository}/{path}: expected a regular manifest file")
    return base64.b64decode("".join(entry["content"].split()), validate=True)


def write_files(repository, branch, head, files, message):
    # Compare-and-swap against the same commit used to decide which files to write.
    response = api("graphql", {
        "query": (
            "mutation($input: CreateCommitOnBranchInput!) { "
            "createCommitOnBranch(input: $input) { commit { oid } } }"
        ),
        "variables": {"input": {
            "branch": {"repositoryNameWithOwner": repository, "branchName": branch},
            "expectedHeadOid": head,
            "message": {"headline": message},
            "fileChanges": {"additions": [
                {"path": path, "contents": base64.b64encode(data).decode("ascii")}
                for path, data in files.items()
            ]},
        }},
    })
    if not response["data"]["createCommitOnBranch"]["commit"]["oid"]:
        raise RuntimeError(f"{repository}:{branch}: commit was not created")


def pointer_version(channel, content, path):
    if channel == "scoop":
        manifest = json.loads(content)
        version = manifest.get("version") if isinstance(manifest, dict) else None
    else:
        versions = re.findall(rb'^\s*version\s+"([^"\r\n]+)"[ \t]*$', content, re.MULTILINE)
        version = versions[0].decode("ascii") if len(versions) == 1 else None
    if not isinstance(version, str):
        raise ValueError(f"{path}: expected one version in the manifest")
    version_order(version)
    return version


def publish_pointer(channel, tag, version, directory):
    order = version_order(version)
    if not order[3]:
        return "skipped", f"{tag} is a prerelease. The channel tracks stable versions."
    repository, branch, path = POINTER_CHANNELS[channel]
    source = directory / Path(path).name
    content = source.read_bytes()
    if pointer_version(channel, content, source) != version:
        raise ValueError(f"{source}: manifest version does not match {tag}")

    head = head_oid(repository, branch)
    previous = read_file(repository, head, path)
    if previous == content:
        return "verified", f"{repository}:{branch}/{path} already contains {tag}."
    if previous is not None:
        previous_version = pointer_version(channel, previous, f"{repository}/{path}")
        previous_order = version_order(previous_version)
        if previous_order > order:
            return "skipped", f"{repository}:{branch} already points at newer v{previous_version}."
        if previous_order == order:
            raise ValueError(f"{repository}/{path}: v{previous_version} exists with different bytes")

    write_files(repository, branch, head, {path: content}, f"feat: bump agnostic-ai to {tag}")
    return "verified", f"{repository}:{branch}/{path} now contains {tag}."


def pointer_status(channel, tag, version):
    order = version_order(version)
    if not order[3]:
        return "skipped", f"{tag} is a prerelease. The channel tracks stable versions."
    repository, branch, path = POINTER_CHANNELS[channel]
    head = head_oid(repository, branch)
    current = read_file(repository, head, path)
    if current is None:
        return "pending", f"{repository}:{branch}/{path} is absent."
    current_version = pointer_version(channel, current, path)
    if current_version == version:
        return "verified", f"{repository}:{branch}/{path} serves {tag}."
    if version_order(current_version) > order:
        return "superseded", f"{repository}:{branch}/{path} serves newer v{current_version}."
    return "pending", f"{repository}:{branch}/{path} still serves v{current_version}."


def winget_manifests(version, directory):
    files = {}
    for name in WINGET_FILES:
        source = directory / name
        content = source.read_bytes()
        fields = re.findall(
            r"^(PackageIdentifier|PackageVersion):[ \t]*([^\r\n]+)\r?$",
            content.decode("utf-8-sig"),
            re.MULTILINE,
        )
        if sorted((name, value.strip(" \t\"'")) for name, value in fields) != [
            ("PackageIdentifier", WINGET_PACKAGE), ("PackageVersion", version),
        ]:
            raise ValueError(f"{source}: manifest identity does not match {WINGET_PACKAGE} {version}")
        files[f"manifests/c/Chemaclass/agnostic-ai/{version}/{name}"] = content
    return files


def missing_manifests(repository, commit, files):
    missing = {}
    for path, content in files.items():
        previous = read_file(repository, commit, path)
        if previous is None:
            missing[path] = content
        elif previous != content:
            raise ValueError(f"{repository}/{path}: existing manifest has different bytes")
    return missing


def winget_pull(branch):
    query = urlencode({"head": f"Chemaclass:{branch}", "base": WINGET_BASE, "state": "all"})
    pulls = api(f"repos/{WINGET_UPSTREAM}/pulls?{query}")
    for pull in pulls:
        if pull["state"] == "open":
            source = pull["head"]["repo"]
            if source is None or source["full_name"].casefold() != WINGET_FORK.casefold():
                raise ValueError(f"{pull['html_url']}: expected a pull request from {WINGET_FORK}")
            return pull["html_url"]
    if pulls:
        raise ValueError(f"{pulls[0]['html_url']}: existing pull request is closed. Review it before retrying.")
    return None


def publish_winget(tag, version, directory):
    files = winget_manifests(version, directory)
    upstream = head_oid(WINGET_UPSTREAM, WINGET_BASE)
    missing_upstream = missing_manifests(WINGET_UPSTREAM, upstream, files)
    if not missing_upstream:
        return "verified", f"{WINGET_UPSTREAM}:{WINGET_BASE} already contains the {tag} manifests."

    branch = f"agnostic-ai-{version}"
    pull = winget_pull(branch)
    head = head_oid(WINGET_FORK, branch, allow_missing=True)
    if head is None:
        if pull is not None:
            raise ValueError(f"{pull}: pull request branch is missing")
        api(f"repos/{WINGET_FORK}/git/refs", {"ref": f"refs/heads/{branch}", "sha": upstream})
        head = upstream
        missing = missing_upstream
    else:
        compare = api(f"repos/{WINGET_UPSTREAM}/compare/{upstream}...Chemaclass:{head}")
        if any(item["filename"] not in files or item["status"] != "added" for item in compare["files"]):
            raise ValueError(f"{WINGET_FORK}:{branch}: branch contains changes outside the release manifests")
        missing = missing_manifests(WINGET_FORK, head, files)

    message = f"New version: agnostic-ai {tag}"
    if missing:
        write_files(WINGET_FORK, branch, head, missing, message)
    if pull is None:
        pull = api(f"repos/{WINGET_UPSTREAM}/pulls", {
            "title": message,
            "head": f"Chemaclass:{branch}",
            "base": WINGET_BASE,
            "body": f"Release https://github.com/Chemaclass/agnostic-ai/releases/tag/{tag}",
        })["html_url"]
    return "pending", f"{tag} awaits catalog review at {pull}."


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("channel", choices=[*POINTER_CHANNELS, "winget"])
    parser.add_argument("tag")
    parser.add_argument("directory", nargs="?", type=Path)
    parser.add_argument("--check", action="store_true", help="Read the current channel version")
    args = parser.parse_args()
    if args.check and args.channel not in POINTER_CHANNELS:
        parser.error("--check requires homebrew or scoop")
    if not args.check and args.directory is None:
        parser.error("the manifest directory is required for publication")
    try:
        if not args.tag.startswith("v"):
            raise ValueError(f"expected a v-prefixed tag, got {args.tag!r}")
        version = args.tag[1:]
        version_order(version)
        if args.check:
            status, message = pointer_status(args.channel, args.tag, version)
        elif args.channel == "winget":
            status, message = publish_winget(args.tag, version, args.directory)
        else:
            status, message = publish_pointer(args.channel, args.tag, version, args.directory)
    except (OSError, ValueError, RuntimeError, KeyError, TypeError, subprocess.TimeoutExpired) as error:
        print(f"{args.channel}: failed. {error}", file=sys.stderr)
        return 1
    print(f"{args.channel}: {status}. {message}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
