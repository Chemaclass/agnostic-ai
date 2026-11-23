# macOS companion signing

[Contributor docs](README.md) · [Release process](release-process.md) · [Decision 023](decisions.md#023-the-macos-companion-is-signed-with-self-signed-certificates)

The macOS companion app is signed with self-signed certificates, without an Apple Developer ID. This page explains why, how each install path signs, and how to make, store, rotate and check the release certificate by hand.

## Why the signature matters

The companion needs Accessibility access to paste into a terminal. macOS stores that grant against the app's designated requirement: a rule, written into the signature, that says which code counts as this app.

| Signature | Designated requirement | After an update |
|---|---|---|
| Ad hoc (`codesign --sign -`) | The code hash | A new app to macOS. The grant is lost. |
| Ad hoc with an identifier rule | `identifier "dev.agnostic-ai.companion"` | Same app, but so is any program that copies the identifier. Unsafe. |
| Self-signed certificate (current) | `identifier "dev.agnostic-ai.companion" and certificate leaf = H"<sha1>"` | Same app. A copy needs the private key to match. |

`H"<sha1>"` is the SHA-1 hash of the certificate itself, so only code signed with that certificate's private key meets the rule.

## The two signers

| Install path | Certificate | Where the key lives | Code |
|---|---|---|---|
| `agnostic-ai companion install` | One made on each Mac on the first install, named `agnostic-ai companion (this Mac)` | `signer.keychain-db` in the companion folder (`~/Library/Application Support/agnostic-ai/companion/`), with `signer.cer` beside it | `internal/cli/companion_sign_darwin.go` |
| Release disk image (`.dmg`) | The project certificate, named `agnostic-ai companion release` | The GitHub secret `COMPANION_SIGNING_P12`, and private copies kept by maintainers | `scripts/companion-sign.sh`, `.github/workflows/release.yml` |

The two certificates differ, so a user who switches between the disk image and the terminal installer grants Accessibility once more. Uninstalling the companion deletes the per-Mac certificate; the next install makes a new one.

### Terminal installer

1. On the first install, `makeCompanionSigner` runs the system `openssl` (`/usr/bin/openssl`) to make an RSA 2048 certificate valid for 100 years, with the code signing usage, and bundles it with its key into a `.p12`.
2. It creates `signer.keychain-db` with an empty password, imports the bundle (`-T /usr/bin/codesign`), and deletes the scratch files. This is the same path the release signing takes. The login keychain is not touched.
3. It writes `signer.cer` last. A keychain without `signer.cer`, or one that lacks the certificate `signer.cer` names, is deleted and made again. An interrupted or failed lookup keeps both files.
4. Each install unlocks the keychain and sets the key's partition list so `codesign` uses it without a dialog.
5. It adds the keychain to the user's keychain search list, runs `codesign --force --keychain <path> --sign <sha1> --identifier dev.agnostic-ai.companion -r=<requirement>`, then removes its temporary entry. Some macOS setups find a signing identity only in a listed keychain, whatever `--keychain` says: a macOS 26.7 CI runner failed with `no identity found` while `security find-identity` listed the identity. A Mac at macOS 27 signed without it. The search list lives under `HOME`, so the Go tests that sign switch back to the real home directory, which the package's `TestMain` replaces.

The keychain has no password because the installer must sign without a prompt. A program running as the same user can read the key and sign a copy that macOS accepts. A password stored on the same Mac would not change that.

### Release build

Two jobs in `release.yml` handle the companion:

1. `companion` builds the archives and disk images with `scripts/companion-release.sh`, signed ad hoc, and uploads them as an artifact. It fetches and runs dependency code, so it reads no secret.
2. `companion-sign` downloads that artifact by its id and runs `scripts/companion-sign.sh` with `COMPANION_SIGNING_P12`. It sets up no toolchain and runs only this repository's scripts. It uploads the signed files as a new artifact, whose id the candidate build (`goreleaser`) downloads.

`scripts/companion-sign.sh`:

1. Imports the secret into a temporary keychain and stops unless it holds the certificate whose hash is pinned at the top of the script (`companion_release_cert`). A lookup by name would also match a certificate whose name only contains the expected one.
2. Adds that keychain to the search list, and prints the hash and the certificate's end date in the job log.
3. For each disk image: copies the app out, signs the bundled CLI and then the app, pins the hash in the app's requirement, checks the app against it with `codesign --verify -R`, and writes the disk image again.
4. Mounts each written disk image and checks its app against the pinned requirement again, so an image signed ad hoc or with another certificate stops the release.
5. Deletes the temporary keychain on exit, which also removes it from the search list.

A stable release needs both disk images and both archives, so a failed build or signing stops it before anything is published. A prerelease ships without them. CI runs the same script on every pull request that touches the companion, with a throwaway certificate passed through `COMPANION_SIGNING_SHA1`.

## Make the release certificate

Run on a Mac. The system `openssl` (`/usr/bin/openssl`, LibreSSL) writes a `.p12` that macOS `security` can import; a Homebrew OpenSSL 3 needs `-legacy` for that.

```bash
dir=~/.agnostic-ai/local/release-signing   # any private folder outside the repository
mkdir -p "$dir" && chmod 700 "$dir" && cd "$dir"
umask 077
cat > cert.cnf <<'EOF'
[req]
distinguished_name=dn
x509_extensions=ext
prompt=no
[dn]
CN=agnostic-ai companion release
[ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature
extendedKeyUsage=critical,codeSigning
EOF
/usr/bin/openssl req -x509 -newkey rsa:3072 -nodes -keyout key.pem -out cert.pem -days 10950 -config cert.cnf
/usr/bin/openssl pkcs12 -export -inkey key.pem -in cert.pem \
  -name "agnostic-ai companion release" -out companion-release.p12 -passout pass:agnostic-ai
rm key.pem cert.cnf
/usr/bin/openssl x509 -in cert.pem -outform der | shasum -a 1 | cut -d' ' -f1 | tr a-f A-F > cert.sha1
```

- The hash in `cert.sha1` must match the hash pinned in `scripts/companion-sign.sh`. The name helps identify the certificate when inspecting the keychain.
- The password `agnostic-ai` is fixed and public. macOS cannot import a `.p12` without a password. The `.p12` file is the secret.
- `-days 10950` is 30 years. Without `-days`, `openssl` makes a certificate that expires in 30 days, and `codesign` refuses an expired one.

## Store it

Upload it as the repository secret. The value goes through standard input, so it does not appear in the command line:

```bash
base64 -i companion-release.p12 | gh secret set COMPANION_SIGNING_P12 --repo Chemaclass/agnostic-ai
```

Keep the folder: `companion-release.p12` (key and certificate), `cert.pem` (public certificate) and `cert.sha1` (the pinned hash). To use it on another machine, copy the folder over a private channel such as AirDrop or `scp`, then `chmod 700` the folder and `chmod 600` its files. Never commit it.

GitHub secrets cannot be read back. Losing every private copy means making a new certificate.

## Rotate it

Rotate only when the key may have leaked or the certificate nears its end date. A new certificate changes the pinned hash, so every disk image user grants Accessibility once more after updating.

1. Make a new certificate as above.
2. Set `companion_release_cert` in `scripts/companion-sign.sh` to the new `cert.sha1`, and merge that change.
3. Replace the secret with the new `.p12`. Do it after the merge and before the next tag, since the script refuses any other certificate.
4. Note the rotation in the changelog, so users expect the prompt.

The app's own update (`agnostic-ai companion update-app`) accepts only an app that meets the designated requirement of the app that runs. After a rotation, that check refuses the new release, so each user downloads the first disk image with the new certificate by hand once. Say so in the changelog with the rotation.

## Check a released app

```bash
hdiutil attach agnostic-ai-companion_darwin_arm64.dmg
codesign -d -r- "/Volumes/agnostic-ai companion/agnostic-ai companion.app"
# designated => identifier "dev.agnostic-ai.companion" and certificate leaf = H"<sha1>"
codesign --verify --deep --strict "/Volumes/agnostic-ai companion/agnostic-ai companion.app"
hdiutil detach "/Volumes/agnostic-ai companion"
```

The hash must match `cert.sha1`. To check that the grant survives, grant Accessibility to one version, install the next one over it, and check that **System Settings > Privacy & Security > Accessibility** still lists the app as allowed.

## First live update

Use two consecutive published releases with disk images. Test in a separate macOS login, or copy the older app to a writable scratch folder and launch it with a scratch `HOME`. The companion runs once per desktop session, so stop the existing instance before opening a scratch copy and restore it afterward.

1. Check that the `Sign the macOS companion` job log of each prints the hash pinned in `scripts/companion-sign.sh`.
2. Install the older disk image in Applications, open it, and grant Accessibility.
3. Choose **Update to vX.Y.Z**, then **Update and restart**. The list should show each step, and the app should come back on the new version.
4. Check the version in the app and with the bundled CLI's `--version`. Open the settings and confirm the old choices are still shown.
5. Check that the new app still has Accessibility access. Record any Gatekeeper prompt during the update. Check the first launch of a quarantined disk image install separately. After verifying that download, use **Open Anyway** if Gatekeeper blocks the launch.
6. Confirm the release checksum and, when `gh` is available and logged in, the build provenance. Check that an app signed with a different certificate is refused before swap, and install it by hand to verify certificate rotation.
7. Check that `~/Library/Application Support/agnostic-ai/companion-update.log` has the line `Replaced the companion app.`

Record the result in the update details of `docs/site/content/docs/cli-reference/companion.md`.

On macOS 27.2.1 (arm64), the checksums and release workflow attestations matched both published disk images. Both bundles passed strict verification against the pinned release certificate. The same check refused them against a different certificate requirement. Both signing jobs printed the pinned certificate fingerprint. [v0.97.0 signing job](https://github.com/Chemaclass/agnostic-ai/actions/runs/45083126805/job/137397071244) · [v0.98.0 signing job](https://github.com/Chemaclass/agnostic-ai/actions/runs/45131337779/job/137548771551)

With the v0.97.0 app running from a scratch folder and using a scratch home, its bundled CLI ran `companion update-app --app <scratch-app> --version 0.98.0`. The update verified the downloaded image's checksum and build provenance using the published attestation bundle, then checked its signature and swapped the app. A new process started from the same path with the same scratch home. Both executable versions were 0.98.0. The log reported `Replaced the companion app.` The small circle, animation and automatic update preferences survived. The running app saved the circle's position, so the scratch settings file changed. The original companion was restarted and its settings file kept the same checksum.

The published npm CLI upgraded from v0.97.0 to v0.98.0 in a scratch prefix using `upgrade --companion-confirmed-release 0.98.0`. The scratch installation held only the CLI. With a Finder-like `PATH` that lacked npm, the v0.97.0 CLI still offered the update. The upgrade failed with `npm: command not found` and exit status 127. `--version` still reported v0.97.0. Browser tests exercised **Copy command** and **Release page** in the fallback and failure states.

A quarantined scratch copy of the published v0.98.0 app passed `codesign --verify --deep --strict`. Gatekeeper assessment with `spctl --assess --type execute` rejected it, and `syspolicy_check distribution` reported a missing notarization ticket. Strict signature verification also passed for the terminal-installed app with its per-Mac certificate. Each app failed verification against the other's certificate requirement.

The remaining Mac desktop checks are the panel update for both install types, the Finder launch and copy fallback, Accessibility after the swap, the first-launch Gatekeeper prompt and **Open Anyway**, and a manual install after certificate rotation.

## Troubleshooting

| Message | Cause | Fix |
|---|---|---|
| `MAC verification failed during PKCS12 import (wrong password?)` | The `.p12` has another password, or none | Export it again with `-passout pass:agnostic-ai`. |
| `COMPANION_SIGNING_P12 does not hold the certificate <sha1>` | The secret holds another certificate, broken base64, or a failed import | Compare `cert.sha1` with the hash pinned in `scripts/companion-sign.sh`, then upload the matching `.p12` again. |
| `<sha1>: no identity found` | The keychain lacks that certificate or key, the certificate expired, or `--keychain` got a relative path | Check the end date; pass an absolute keychain path. |
| A dialog: "codesign wants to use the "signer" keychain" | `codesign` reached a locked keychain | Cancel it. The code unlocks before each signature; a dialog means something ran `codesign` without that step. |
| `CSSMERR_TP_NOT_TRUSTED` in `security find-identity` | The certificate is self-signed | Expected. `codesign` signs with it anyway. |
