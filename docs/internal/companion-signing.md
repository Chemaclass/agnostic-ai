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
| Release disk image (`.dmg`) | The project certificate, named `agnostic-ai companion release` | The GitHub secret `COMPANION_SIGNING_P12`, and private copies kept by maintainers | `scripts/companion-release.sh`, `.github/workflows/release.yml` |

The two certificates differ, so a user who switches between the disk image and the terminal installer grants Accessibility once more. Uninstalling the companion deletes the per-Mac certificate; the next install makes a new one.

### Terminal installer

1. On the first install, `companionSigner` makes an RSA 2048 certificate valid for 100 years, with the code signing usage.
2. It creates `signer.keychain-db` with an empty password and imports the key (`-T /usr/bin/codesign`) and the certificate. The keychain is not added to the user's search list, and the login keychain is not touched.
3. It writes `signer.cer` last. A keychain without `signer.cer`, or one that lacks the certificate `signer.cer` names, is deleted and made again.
4. Each install unlocks the keychain, since it locks itself after five minutes and on sleep, then runs `codesign --force --keychain <path> --sign <sha1> --identifier dev.agnostic-ai.companion -r=<requirement>`.

The keychain has no password because the installer must sign without a prompt. A program running as the same user can read the key and sign a copy that macOS accepts. A password stored on the same Mac would not change that.

### Release build

The `macOS companion` job in `release.yml` passes `COMPANION_SIGNING_P12` and `COMPANION_SIGNING_REQUIRED=1` to `scripts/companion-release.sh`, which:

1. Decodes the secret into a temporary keychain and reads the certificate's SHA-1 hash. It stops with an error when the secret is empty or holds no certificate with the expected name.
2. Prints the hash and the certificate's end date in the job log.
3. Signs the bundled CLI and then the app with that certificate, pins the hash in the app's requirement, and checks the app against it with `codesign --verify -R`.
4. Deletes the temporary keychain on exit.

Without the secret, CI and local builds sign ad hoc. In a tag release the companion job fails, and the release publishes without the companion files.

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

- The name must be exactly `agnostic-ai companion release`; the script looks the certificate up by it.
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
2. Replace the secret with the new `.p12`.
3. Note the rotation in the changelog, so users expect the prompt.

## Check a released app

```bash
hdiutil attach agnostic-ai-companion_darwin_arm64.dmg
codesign -d -r- "/Volumes/agnostic-ai companion/agnostic-ai companion.app"
# designated => identifier "dev.agnostic-ai.companion" and certificate leaf = H"<sha1>"
codesign --verify --deep --strict "/Volumes/agnostic-ai companion/agnostic-ai companion.app"
hdiutil detach "/Volumes/agnostic-ai companion"
```

The hash must match `cert.sha1`. To check that the grant survives, grant Accessibility to one version, install the next one over it, and check that **System Settings > Privacy & Security > Accessibility** still lists the app as allowed.

## Troubleshooting

| Message | Cause | Fix |
|---|---|---|
| `MAC verification failed during PKCS12 import (wrong password?)` | The `.p12` has another password, or none | Export it again with `-passout pass:agnostic-ai`. |
| `COMPANION_SIGNING_P12 holds no certificate named agnostic-ai companion release` | Wrong name, broken base64, or a failed import | Check the name with `/usr/bin/openssl pkcs12 -in companion-release.p12 -passin pass:agnostic-ai -nokeys \| /usr/bin/openssl x509 -noout -subject`, then upload again. |
| `<sha1>: no identity found` | The keychain lacks that certificate or key, the certificate expired, or `--keychain` got a relative path | Check the end date; pass an absolute keychain path. |
| A dialog: "codesign wants to use the "signer" keychain" | `codesign` reached a locked keychain | Cancel it. The code unlocks before each signature; a dialog means something ran `codesign` without that step. |
| `CSSMERR_TP_NOT_TRUSTED` in `security find-identity` | The certificate is self-signed | Expected. `codesign` signs with it anyway. |
