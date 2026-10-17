#!/bin/sh
set -eu

if [ "$(uname -s)" != Linux ] || [ "$(uname -m)" != aarch64 ] ||
   [ "${RTK_CAVEMAN_CONTAINER:-}" != 1 ] || [ ! -f /.dockerenv ] ||
   [ "$(id -u)" != 0 ]; then
    printf '%s\n' 'Run this helper only in the documented Linux arm64 Docker container.' >&2
    exit 2
fi
if ! grep -q '^VERSION_CODENAME=trixie$' /etc/os-release; then
    printf '%s\n' 'This recipe requires Debian trixie for the RTK release libc version.' >&2
    exit 2
fi
for asset in rtk-caveman-runtime.py rtk-release.json caveman-release.json \
    caveman-cli/package.json caveman-cli/package-lock.json \
    rtk-aarch64-unknown-linux-gnu.tar.gz checksums.txt \
    caveman-checksums.txt caveman-checksums.txt.keysig caveman-RELEASE; do
    if [ ! -s "/lab/$asset" ]; then
        printf 'Missing scratch asset: /lab/%s\n' "$asset" >&2
        exit 2
    fi
done

cd /lab
printf '%s\n' "$HOME" > container-home-before.txt
apt-get update > apt-install.log 2>&1
apt-get install -y --no-install-recommends python3 ca-certificates curl >> apt-install.log 2>&1
export CAVEMAN_HOME=/lab/caveman
export CAVEMAN_TELEMETRY=0
export DO_NOT_TRACK=1
export CAVE_SETUP_TIMEOUT=60
mkdir -p /lab/cli
cp /lab/caveman-cli/package.json /lab/caveman-cli/package-lock.json /lab/cli/
npm ci --prefix /lab/cli --ignore-scripts \
    --fetch-timeout=60000 --fetch-retries=1 > npm-install.log 2>&1
python3 - <<'PY'
import hashlib
import json
from pathlib import Path

cli = Path('/lab/cli/node_modules/@caveman-ai/cli')
if json.loads((cli / 'package.json').read_text())['version'] != '2.1.0':
    raise RuntimeError('Unexpected Caveman CLI version')
if hashlib.sha256((cli / 'dist/index.js').read_bytes()).hexdigest() != '2864f44410cd8a6b508be530cbcb2e4f8b7c93380dba9190f3977e21a7c98806':
    raise RuntimeError('Pinned Caveman CLI entry digest mismatch')
PY
node /lab/cli/node_modules/@caveman-ai/cli/dist/index.js setup --install --json \
    > caveman-install.json 2> caveman-install.log
if [ ! -f caveman-first-install.json ]; then
    cp caveman-install.json caveman-first-install.json
fi

python3 - <<'PY'
import hashlib
import json
from pathlib import Path, PurePosixPath
import tarfile

def require(condition, message):
    if not condition:
        raise RuntimeError(message)

p = Path('/lab')
name = 'rtk-aarch64-unknown-linux-gnu.tar.gz'
actual = hashlib.sha256((p / name).read_bytes()).hexdigest()
require(actual == '8d6d1aad9e69b42481eda7039507d1f7ee93698f87713cecd873d287c1931632', 'RTK pinned digest mismatch')
matches = [line.split()[0] for line in (p / 'checksums.txt').read_text().splitlines()
           if len(line.split()) == 2 and line.split()[1].lstrip('*') == name]
require(matches == [actual], 'RTK release checksum mismatch or missing entry')
assets = [asset for asset in json.loads((p / 'rtk-release.json').read_text())['assets'] if asset['name'] == name]
require(len(assets) == 1 and assets[0].get('digest') == 'sha256:' + actual, 'RTK GitHub digest mismatch or missing asset')
with tarfile.open(p / name) as archive:
    members = [item for item in archive.getmembers() if PurePosixPath(item.name).name == 'rtk' and item.isfile()]
    require(len(members) == 1, 'RTK archive must contain one regular executable')
    stream = archive.extractfile(members[0])
    require(stream is not None, 'RTK executable cannot be read')
    binary = stream.read()
rtk = p / 'rtk'
rtk.write_bytes(binary)
rtk.chmod(0o755)
(p / 'rtk-verified.json').write_text(json.dumps({
    'archive': name, 'sha256': actual, 'checksum_matches': True,
    'github_digest_matches': True, 'binary_sha256': hashlib.sha256(binary).hexdigest(),
}, indent=2) + '\n')
PY

python3 /lab/rtk-caveman-runtime.py --rtk /lab/rtk --node /usr/local/bin/node \
    --caveman-cli /lab/cli/node_modules/@caveman-ai/cli/dist/index.js \
    --caveman-engine /lab/caveman/bin/caveman-engine \
    --output /lab/rtk-caveman-runtime-linux-results.json
printf '%s\n' "$HOME" > container-home-after.txt

python3 - <<'PY'
import hashlib
import json
from pathlib import Path
import platform
import sys

def require(condition, message):
    if not condition:
        raise RuntimeError(message)

p = Path('/lab')
require((p / 'container-home-before.txt').read_bytes() == (p / 'container-home-after.txt').read_bytes(), 'Inherited home changed')
installer = json.loads((p / 'caveman-install.json').read_text())
require(installer.get('release') == 'bin-v2.1.0' and installer.get('platform') == 'linux/arm64', 'Unexpected Caveman release or platform')
release = json.loads((p / 'caveman-release.json').read_text())
for binary in installer.get('binaries', []):
    assets = [asset for asset in release['assets'] if asset['name'] == binary['name'] + '_linux_arm64']
    actual = hashlib.sha256(Path(binary['path']).read_bytes()).hexdigest()
    require(len(assets) == 1 and assets[0].get('digest') == 'sha256:' + actual and binary['sha256'] == actual,
            'Caveman installed binary digest mismatch: ' + binary['name'])
require(len(installer.get('binaries', [])) == 6, 'Expected six installed Caveman binaries')
metadata = {
    'platform': sys.platform, 'machine': platform.machine(), 'kernel': platform.release(),
    'python': platform.python_version(), 'home_unchanged': True,
    'home': (p / 'container-home-before.txt').read_text().strip(),
    'runner_sha256': hashlib.sha256((p / 'rtk-caveman-runtime.py').read_bytes()).hexdigest(),
    'caveman_cli_package': json.loads((p / 'cli/node_modules/@caveman-ai/cli/package.json').read_text())['version'],
    'caveman_installer_result': installer,
    'first_caveman_installer_result': json.loads((p / 'caveman-first-install.json').read_text()),
    'rtk_verification': json.loads((p / 'rtk-verified.json').read_text()),
    'npm_lock': json.loads((p / 'cli/package-lock.json').read_text()),
    'container_image_recipe': 'node@sha256:154ba2f4d6fec323d28e4f4bb86bba4677f1223391a1979cf521304e03a98dfa',
    'signature_verification_basis': 'Unmodified pinned Caveman CLI 2.1.0 setup --install validates its signed checksum manifest and release binding before downloading and installing binaries. Cached installations verify binary hashes.',
    'caveman_installed_hashes_match_github_digests': True,
    'retained_signed_release_files': {
        name: hashlib.sha256((p / name).read_bytes()).hexdigest()
        for name in ('caveman-checksums.txt', 'caveman-checksums.txt.keysig', 'caveman-RELEASE')
    },
}
(p / 'linux-provenance.json').write_text(json.dumps(metadata, indent=2) + '\n')
PY
