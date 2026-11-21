#!/usr/bin/env python3
import base64
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from urllib.parse import parse_qs, urlsplit


SPEC = importlib.util.spec_from_file_location("channels", Path(__file__).with_name("release-channel.py"))
channels = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(channels)

UPSTREAM_OID = "a" * 40
FORK_OID = "b" * 40
WRITTEN_OID = "c" * 40
UPSTREAM = f"repos/{channels.WINGET_UPSTREAM}"
FORK = f"repos/{channels.WINGET_FORK}"
BRANCH = "agnostic-ai-0.96.0"
PULL_URL = f"https://github.com/{channels.WINGET_UPSTREAM}/pull/1"


def ref(oid):
    return {"object": {"sha": oid}}


def blob(content):
    return {"type": "file", "encoding": "base64", "content": base64.b64encode(content).decode("ascii")}


class Fixture(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = Path(directory.name)
        patcher = patch.object(channels, "api")
        self.addCleanup(patcher.stop)
        self.api = patcher.start()
        self.api.side_effect = self.respond
        self.responses = {}
        self.writes = []

    def respond(self, endpoint, payload=None, *, allow_missing=False):
        if payload is not None:
            self.writes.append((endpoint, payload))
            if endpoint == "graphql":
                return {"data": {"createCommitOnBranch": {"commit": {"oid": WRITTEN_OID}}}}
            if endpoint == f"{FORK}/git/refs":
                return ref(payload["sha"])
            if endpoint == f"{UPSTREAM}/pulls":
                return {"html_url": PULL_URL}
            self.fail(f"unhandled write {endpoint}")
        if endpoint in self.responses:
            return self.responses[endpoint]
        if urlsplit(endpoint).path == f"{UPSTREAM}/pulls":
            self.assertEqual(parse_qs(urlsplit(endpoint).query), {
                "head": [f"Chemaclass:{BRANCH}"], "base": ["master"], "state": ["all"],
            })
            return []
        if allow_missing:
            return None
        self.fail(f"unhandled read {endpoint}")

    def commit(self):
        commits = [payload["variables"]["input"] for endpoint, payload in self.writes if endpoint == "graphql"]
        self.assertEqual(len(commits), 1)
        return commits[0]


class PointerTests(Fixture):
    def cask(self, current):
        path = "Casks/agnostic-ai.rb"
        source = self.directory / "agnostic-ai.rb"
        source.write_bytes(b'cask "agnostic-ai" do\n  version "0.96.0"\nend\n')
        repo = "repos/Chemaclass/homebrew-tap"
        self.responses[f"{repo}/git/ref/heads/master"] = ref(UPSTREAM_OID)
        self.responses[f"{repo}/contents/{path}?ref={UPSTREAM_OID}"] = blob(current)
        return path, source.read_bytes()

    def test_homebrew_advances_from_the_commit_it_read(self):
        path, content = self.cask(b'  version "0.95.0"\n')
        self.assertEqual(channels.publish_pointer("homebrew", "v0.96.0", "0.96.0", self.directory)[0], "verified")
        commit = self.commit()
        self.assertEqual(commit["branch"], {"repositoryNameWithOwner": "Chemaclass/homebrew-tap", "branchName": "master"})
        self.assertEqual(commit["expectedHeadOid"], UPSTREAM_OID)
        self.assertEqual(commit["fileChanges"]["additions"], [
            {"path": path, "contents": base64.b64encode(content).decode("ascii")},
        ])

    def test_homebrew_never_moves_backwards_or_replaces_different_bytes_for_same_version(self):
        for current, status in [("0.100.0", "skipped"), ("0.96.0", "conflict"), ("0.96.0+build.2", "conflict")]:
            with self.subTest(current=current):
                self.cask(f'  version "{current}"\n'.encode())
                if status == "conflict":
                    with self.assertRaisesRegex(ValueError, "different bytes"):
                        channels.publish_pointer("homebrew", "v0.96.0", "0.96.0", self.directory)
                else:
                    self.assertEqual(channels.publish_pointer("homebrew", "v0.96.0", "0.96.0", self.directory)[0], status)
        self.assertEqual(self.writes, [])

    def test_scoop_rerun_skips_the_same_bytes_at_bucket_root(self):
        manifest = json.dumps({"version": "0.96.0"}).encode()
        (self.directory / "agnostic-ai.json").write_bytes(manifest)
        repo = "repos/Chemaclass/scoop-bucket"
        self.responses[f"{repo}/git/ref/heads/main"] = ref(UPSTREAM_OID)
        self.responses[f"{repo}/contents/agnostic-ai.json?ref={UPSTREAM_OID}"] = blob(manifest)
        status, message = channels.publish_pointer("scoop", "v0.96.0", "0.96.0", self.directory)
        self.assertEqual(status, "verified")
        self.assertIn("already contains", message)
        self.assertEqual(self.writes, [])

    def test_prerelease_leaves_stable_channels_untouched(self):
        for channel in ("homebrew", "scoop"):
            with self.subTest(channel=channel):
                self.assertEqual(channels.publish_pointer(channel, "v1.0.0-rc.2", "1.0.0-rc.2", self.directory)[0], "skipped")
        self.api.assert_not_called()

    def test_rejects_wrong_tag_before_reading_channel(self):
        self.cask(b'  version "0.95.0"\n')
        with self.assertRaisesRegex(ValueError, "manifest version does not match v0.97.0"):
            channels.publish_pointer("homebrew", "v0.97.0", "0.97.0", self.directory)
        self.api.assert_not_called()

    def test_homebrew_availability_distinguishes_current_older_and_newer_versions(self):
        for version, expected in [("0.96.0", "verified"), ("0.95.0", "pending"), ("0.100.0", "superseded")]:
            with self.subTest(version=version):
                self.cask(f'  version "{version}"\n'.encode())
                self.assertEqual(channels.pointer_status("homebrew", "v0.96.0", "0.96.0")[0], expected)
        self.assertEqual(self.writes, [])


class WingetTests(Fixture):
    def setUp(self):
        super().setUp()
        self.files = {}
        for name in channels.WINGET_FILES:
            content = b"PackageIdentifier: Chemaclass.agnostic-ai\nPackageVersion: 0.96.0\n"
            (self.directory / name).write_bytes(content)
            self.files[f"manifests/c/Chemaclass/agnostic-ai/0.96.0/{name}"] = content
        self.responses[f"{UPSTREAM}/git/ref/heads/master"] = ref(UPSTREAM_OID)

    def in_existing_branch(self, paths):
        self.responses[f"{FORK}/git/ref/heads/{BRANCH}"] = ref(FORK_OID)
        self.responses[f"{UPSTREAM}/compare/{UPSTREAM_OID}...Chemaclass:{FORK_OID}"] = {
            "files": [{"filename": path, "status": "added"} for path in paths],
        }
        for path in paths:
            self.responses[f"{FORK}/contents/{path}?ref={FORK_OID}"] = blob(self.files[path])

    def publish(self):
        return channels.publish_winget("v0.96.0", "0.96.0", self.directory)

    def pull_fixture(self, state):
        self.responses[f"{UPSTREAM}/pulls?head=Chemaclass%3A{BRANCH}&base=master&state=all"] = [{
            "state": state, "html_url": PULL_URL, "head": {"repo": {"full_name": channels.WINGET_FORK}},
        }]

    def test_creates_branch_then_commits_all_manifests_before_opening_pull_request(self):
        status, message = self.publish()
        self.assertEqual(status, "pending")
        self.assertIn(PULL_URL, message)
        self.assertEqual([endpoint for endpoint, _ in self.writes], [f"{FORK}/git/refs", "graphql", f"{UPSTREAM}/pulls"])
        self.assertEqual(self.writes[0][1], {"ref": f"refs/heads/{BRANCH}", "sha": UPSTREAM_OID})
        commit = self.commit()
        self.assertEqual(commit["branch"], {"repositoryNameWithOwner": channels.WINGET_FORK, "branchName": BRANCH})
        self.assertEqual(commit["expectedHeadOid"], UPSTREAM_OID)
        self.assertEqual({item["path"]: base64.b64decode(item["contents"]) for item in
                          commit["fileChanges"]["additions"]}, self.files)
        self.assertEqual(self.writes[-1][1]["head"], f"Chemaclass:{BRANCH}")
        self.assertEqual(self.writes[-1][1]["base"], "master")

    def test_retry_fills_only_missing_files_and_reuses_existing_pull_request(self):
        existing = next(iter(self.files))
        self.in_existing_branch([existing])
        self.pull_fixture("open")
        self.assertEqual(self.publish()[0], "pending")
        commit = self.commit()
        self.assertEqual(commit["expectedHeadOid"], FORK_OID)
        self.assertEqual({item["path"] for item in commit["fileChanges"]["additions"]}, self.files.keys() - {existing})
        self.assertEqual(len(self.writes), 1)

    def test_retry_after_commit_skips_writes_for_complete_open_pull_request(self):
        self.in_existing_branch(self.files)
        self.pull_fixture("open")
        self.assertEqual(self.publish()[0], "pending")
        self.assertEqual(self.writes, [])

    def test_duplicate_request_is_avoided_when_upstream_has_exact_bytes(self):
        for path, data in self.files.items():
            self.responses[f"{UPSTREAM}/contents/{path}?ref={UPSTREAM_OID}"] = blob(data)
        self.assertEqual(self.publish()[0], "verified")
        self.assertEqual(self.writes, [])

    def test_refuses_unrelated_branch_changes_and_closed_pull_request(self):
        self.in_existing_branch([])
        self.responses[f"{UPSTREAM}/compare/{UPSTREAM_OID}...Chemaclass:{FORK_OID}"]["files"] = [
            {"filename": "README.md", "status": "modified"},
        ]
        with self.assertRaisesRegex(ValueError, "branch contains changes outside"):
            self.publish()
        self.pull_fixture("closed")
        with self.assertRaisesRegex(ValueError, "existing pull request is closed"):
            self.publish()
        self.assertEqual(self.writes, [])

    def test_refuses_existing_manifest_conflict(self):
        path = next(iter(self.files))
        self.in_existing_branch([path])
        self.responses[f"{FORK}/contents/{path}?ref={FORK_OID}"] = blob(b"changed file")
        with self.assertRaisesRegex(ValueError, "existing manifest has different bytes"):
            self.publish()
        self.assertEqual(self.writes, [])


if __name__ == "__main__":
    unittest.main()
