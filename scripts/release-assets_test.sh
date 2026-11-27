#!/usr/bin/env bash

asset_script="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/release-assets.py"

function set_up() {
  work="$(mktemp -d)"
  mkdir -p "$work/dist/homebrew/Casks" "$work/dist/scoop" "$work/dist/winget" "$work/companion"
  printf 'cask fixture\n' > "$work/dist/homebrew/Casks/agnostic-ai.rb"
  printf 'manifest fixture\n' > "$work/dist/scoop/agnostic-ai.json"
  local suffix name
  for suffix in .yaml .installer.yaml .locale.en-US.yaml; do
    printf 'winget fixture %s\n' "$suffix" > "$work/dist/winget/Chemaclass.agnostic-ai$suffix"
  done
  for name in agnostic-ai_{darwin,linux}_{amd64,arm64}.tar.gz agnostic-ai_windows_{amd64,arm64}.zip; do
    printf 'archive %s\n' "$name" > "$work/dist/$name"
    printf 'SBOM %s\n' "$name" > "$work/dist/$name.sbom.json"
  done
  update_checksums
}

function tear_down() {
  rm -rf "${work:?}"
}

function add_companion() {
  local name
  for name in agnostic-companion_darwin_{amd64,arm64}.{tar.gz,dmg} agnostic-ai-companion_darwin_{amd64,arm64}.dmg; do
    printf 'companion %s\n' "$name" > "$work/companion/$name"
  done
  update_checksums
}

function update_checksums() {
  (
    cd "$work/dist" || exit 1
    shasum -a 256 agnostic-ai_*
  ) > "$work/dist/checksums.txt"
  if compgen -G "$work/companion/*" > /dev/null; then
    (
      cd "$work/companion" || exit 1
      shopt -s nullglob
      shasum -a 256 agnostic-companion_* agnostic-ai-companion_*
    ) >> "$work/dist/checksums.txt"
  fi
}

function release_assets() {
  python3 "$asset_script" "$@"
}

function stage_candidate() {
  local status=0 output
  output="$(release_assets stage "$1" "$work/dist" "$work/companion" "$work/candidate" 2>&1)" || status=$?
  printf '%s\n' "$output"
  return "$status"
}

function test_stable_candidates_include_six_companion_assets_and_manifests() {
  add_companion
  local tag name
  for tag in v0.99.2 v1.0.0 v1.1.0; do
    assert_successful_code "$(stage_candidate "$tag")"
    for name in agnostic-companion_darwin_{amd64,arm64}.{tar.gz,dmg} agnostic-ai-companion_darwin_{amd64,arm64}.dmg; do
      assert_file_exists "$work/candidate/assets/$name"
    done
    mv "$work/candidate" "$work/$tag"
  done
  mv "$work/v1.1.0" "$work/candidate"
  assert_file_exists "$work/candidate/assets/agnostic-ai_windows_amd64.zip.sbom.json"
  assert_file_exists "$work/candidate/channels/Chemaclass.agnostic-ai.installer.yaml"
  local subjects
  subjects="$(release_assets subjects v1.1.0 "$work/candidate")"
  assert_contains "  checksums.txt" "$subjects"
  assert_contains "  agnostic-ai.rb" "$subjects"
  assert_contains "  agnostic-ai-companion_darwin_arm64.dmg" "$subjects"
  assert_contains "  agnostic-companion_darwin_amd64.tar.gz" "$subjects"
}

function test_stable_candidate_rejects_a_missing_legacy_image() {
  add_companion
  rm "$work/companion/agnostic-ai-companion_darwin_amd64.dmg"
  update_checksums
  local output status=0
  output="$(stage_candidate v1.2.3)" || status=$?
  assert_same 1 "$status"
  assert_contains 'missing agnostic-ai-companion_darwin_amd64.dmg' "$output"
}

function test_staging_rejects_any_extra_companion_output_even_without_a_checksum() {
  local name output status
  for name in agnostic-ai-companion_darwin_amd64.tar.gz agnostic-companion_darwin_other.dmg notes.txt; do
    add_companion
    printf 'unexpected asset\n' > "$work/companion/$name"
    status=0
    output="$(stage_candidate v1.2.3)" || status=$?
    assert_same 1 "$status"
    assert_contains "unexpected $name" "$output"
    rm "$work/companion/$name"
    mv "$work/candidate" "$work/failed-$name"
  done
}

function test_stable_candidate_rejects_a_missing_renamed_archive() {
  add_companion
  rm "$work/companion/agnostic-companion_darwin_amd64.tar.gz"
  update_checksums
  local output status=0
  output="$(stage_candidate v1.2.4)" || status=$?
  assert_same 1 "$status"
  assert_contains 'missing agnostic-companion_darwin_amd64.tar.gz' "$output"
}

function test_prerelease_can_include_all_or_no_companion_files() {
  rmdir "$work/companion"
  assert_successful_code "$(stage_candidate v1.2.3-rc.1)"
  assert_file_not_exists "$work/candidate/assets/agnostic-companion_darwin_arm64.dmg"
  mv "$work/candidate" "$work/without-companion"
  mkdir "$work/companion"
  add_companion
  assert_successful_code "$(stage_candidate v1.2.3-rc.2)"
  assert_file_exists "$work/candidate/assets/agnostic-ai-companion_darwin_amd64.dmg"
}

function test_prerelease_cannot_include_a_partial_companion_set() {
  add_companion
  rm "$work/companion/agnostic-ai-companion_darwin_arm64.dmg"
  update_checksums
  local output status=0
  output="$(stage_candidate v1.2.3-rc.1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'missing agnostic-ai-companion_darwin_arm64.dmg' "$output"
}

function test_subjects_rejects_an_extra_asset_added_after_staging() {
  add_companion
  assert_successful_code "$(stage_candidate v1.2.3)"
  printf 'extra asset\n' > "$work/candidate/assets/agnostic-ai-companion_darwin_arm64.tar.gz"
  local output status=0
  output="$(release_assets subjects v1.2.3 "$work/candidate" 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'unexpected agnostic-ai-companion_darwin_arm64.tar.gz' "$output"
}

function test_subjects_rejects_an_archive_changed_after_staging() {
  add_companion
  assert_successful_code "$(stage_candidate v1.2.3)"
  printf 'changed bytes\n' >> "$work/candidate/assets/agnostic-ai_linux_amd64.tar.gz"
  local output status=0
  output="$(release_assets subjects v1.2.3 "$work/candidate" 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'checksum mismatch for agnostic-ai_linux_amd64.tar.gz' "$output"
}

function test_staging_rejects_a_missing_or_duplicate_checksum() {
  add_companion
  grep -v 'agnostic-companion_darwin_arm64.dmg' "$work/dist/checksums.txt" > "$work/checksums"
  mv "$work/checksums" "$work/dist/checksums.txt"
  local output checksum status=0
  output="$(stage_candidate v1.2.3)" || status=$?
  assert_same 1 "$status"
  assert_contains 'checksums.txt: missing agnostic-companion_darwin_arm64.dmg' "$output"
  mv "$work/candidate" "$work/failed"
  update_checksums
  read -r checksum < "$work/dist/checksums.txt"
  printf '%s\n' "$checksum" >> "$work/dist/checksums.txt"
  output="$(stage_candidate v1.2.3)" || status=$?
  assert_same 1 "$status"
  assert_contains 'duplicate checksum' "$output"
}

function start_release_fixture() {
  add_companion
  mkdir "$work/bin"
  cat > "$work/bin/gh" <<'PY'
#!/usr/bin/env python3
import hashlib
import json
import os
from pathlib import Path
import sys

directory = Path(os.environ["RELEASE_FIXTURE"])
state = directory / "remote"
args = sys.argv[1:]
command = " ".join(args[:2])
with (directory / "operations").open("a") as log:
    print(command, file=log)
if command == "attestation verify":
    assert args[args.index("--source-ref") + 1] == "refs/tags/v1.2.3"
    assert args[args.index("--source-digest") + 1] == "tested-commit"
    if (directory / "bad-attestation").exists():
        sys.exit("invalid signature")
elif args[0] == "api" and "/commits/" in args[1]:
    assert args[1] == "repos/owner/repo/commits/v1.2.3"
    print("other-commit" if (directory / "wrong-tag-commit").exists() else "tested-commit")
elif args[0] == "api" and "/actions/runs/" in args[1]:
    print((directory / "artifacts").read_text())
elif command == "run list":
    print(json.dumps([
        {"databaseId": 1, "headBranch": "v1.2.4", "headSha": "tested-commit"},
        {"databaseId": 2, "headBranch": "v1.2.3", "headSha": "tested-commit"},
    ]))
elif command == "release view":
    assert args[2] == "v1.2.3"
    if not state.exists():
        sys.exit("release not found")
    print(state.read_text())
elif command == "release create":
    assert "--draft" in args and "--verify-tag" in args
    state.write_text(json.dumps({"isDraft": True, "isPrerelease": False, "tagName": args[2], "assets": []}))
elif command == "release upload":
    release = json.loads(state.read_text())
    assert release["isDraft"]
    assert "--clobber" not in args
    for asset in map(Path, args[5:]):
        data = b"different upload" if (directory / "corrupt-upload").exists() else asset.read_bytes()
        release["assets"].append({
            "name": asset.name, "state": "uploaded",
            "digest": f"sha256:{hashlib.sha256(data).hexdigest()}",
        })
    state.write_text(json.dumps(release))
elif command == "release edit":
    release = json.loads(state.read_text())
    assert release["isDraft"] and "--draft=false" in args
    release["isDraft"] = False
    state.write_text(json.dumps(release))
else:
    sys.exit(f"unexpected call {args!r}")
PY
  chmod +x "$work/bin/gh"
  stage_candidate v1.2.3 || return 1
  printf 'portable bundle fixture\n' > "$work/candidate/assets/agnostic-ai.intoto.jsonl"
  printf 'release notes\n' > "$work/notes.md"
}

function publish_fixture() {
  RELEASE_FIXTURE="$work" PATH="$work/bin:$PATH" release_assets publish v1.2.3 \
    "$work/candidate" owner/repo tested-commit "$work/notes.md"
}

function test_recovery_requires_the_legacy_images_to_have_an_upload_digest() {
  start_release_fixture
  assert_successful_code "$(publish_fixture)"
  local output status=0
  python3 - "$work/remote" <<'PY'
import json
from pathlib import Path
import sys

path = Path(sys.argv[1])
release = json.loads(path.read_text())
asset = next(asset for asset in release["assets"] if asset["name"] == "agnostic-ai-companion_darwin_arm64.dmg")
asset["digest"] = None
path.write_text(json.dumps(release))
PY
  output="$(recover_fixture npm 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'agnostic-ai-companion_darwin_arm64.dmg on v1.2.3 has no verified upload digest' "$output"
}

function test_verified_candidate_publishes_once_and_retry_leaves_it_intact() {
  start_release_fixture
  assert_successful_code "$(publish_fixture)"
  assert_contains '"isDraft": false' "$(cat "$work/remote")"
  assert_successful_code "$(publish_fixture)"
  assert_same 1 "$(grep -c '^release edit$' "$work/operations")"
}

function test_bad_provenance_stops_before_creating_a_release() {
  start_release_fixture
  touch "$work/bad-attestation"
  local output status=0
  output="$(publish_fixture 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'invalid signature' "$output"
  assert_file_not_exists "$work/remote"
}

function test_corrupt_remote_asset_stays_a_draft() {
  start_release_fixture
  touch "$work/corrupt-upload"
  local output status=0
  output="$(publish_fixture 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'GitHub has different bytes' "$output"
  assert_contains '"isDraft": true' "$(cat "$work/remote")"
  assert_not_contains 'release edit' "$(cat "$work/operations")"
}

function recover_fixture() {
  RELEASE_FIXTURE="$work" PATH="$work/bin:$PATH" release_assets recover v1.2.3 owner/repo tested-commit "$1"
}

function test_recovery_selects_the_original_candidate_by_published_bundle_digest() {
  start_release_fixture
  assert_successful_code "$(publish_fixture)"
  local digest output
  digest="$(shasum -a 256 "$work/candidate/assets/agnostic-ai.intoto.jsonl" | cut -d ' ' -f1)"
  cat > "$work/artifacts" <<JSON
[{"artifacts":[
  {"id":9,"expired":false,"name":"signed-release-candidate-other-digest-4"},
  {"id":8,"expired":true,"name":"signed-release-candidate-$digest-3"},
  {"id":7,"expired":false,"name":"signed-release-candidate-$digest-2"}
]}]
JSON
  output="$(recover_fixture scoop)"
  assert_contains 'run-id=2' "$output"
  assert_contains 'artifact=7' "$output"
  assert_same 1 "$(grep -c '^release edit$' "$work/operations")"
}

function test_npm_recovery_uses_public_inputs_without_a_retained_candidate() {
  start_release_fixture
  assert_successful_code "$(publish_fixture)"
  assert_successful_code "$(recover_fixture npm)"
  assert_not_contains 'run list' "$(cat "$work/operations")"
}

function test_recovery_requires_the_exact_tag_commit_and_original_artifact() {
  start_release_fixture
  assert_successful_code "$(publish_fixture)"
  touch "$work/wrong-tag-commit"
  local output status=0
  output="$(recover_fixture homebrew 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'no longer resolves to tested-commit' "$output"
  rm "$work/wrong-tag-commit"
  printf '[{"artifacts":[]}]\n' > "$work/artifacts"
  output="$(recover_fixture homebrew 2>&1)" || status=$?
  assert_same 1 "$status"
  assert_contains 'original signed candidate is unavailable' "$output"
  assert_same 1 "$(grep -c '^release edit$' "$work/operations")"
}
