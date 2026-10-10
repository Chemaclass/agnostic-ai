import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import tarfile

parser = argparse.ArgumentParser(description='Verify published trio archive bytes without extracting files.')
parser.add_argument('dataset', type=Path)
args = parser.parse_args()
artifacts = json.loads((args.dataset/'artifacts.json').read_text())
if set(artifacts) != {'final', 'original-pilot', 'fresh-pilot'}:
    raise ValueError('Unexpected published phases')
verified = 0
for phase, metadata in artifacts.items():
    folder = args.dataset/phase
    archive = folder/'raw.tar.gz'
    if hashlib.sha256(archive.read_bytes()).hexdigest() != metadata['archive_sha256']:
        raise ValueError(f'{archive}: archive hash mismatch')
    with tarfile.open(archive, 'r:gz') as tar:
        members = tar.getmembers()
        names = [member.name for member in members]
        if len(names) != len(set(names)) or any(
                not member.isfile() or PurePosixPath(member.name).is_absolute()
                or '..' in PurePosixPath(member.name).parts for member in members):
            raise ValueError(f'{archive}: unsafe or duplicate member')
        index = json.loads(tar.extractfile('export-manifest.json').read())
        if set(names) != set(index['files']) | {'export-manifest.json'} or len(names) != metadata['files']:
            raise ValueError(f'{archive}: file coverage mismatch')
        for name, expected in index['files'].items():
            content = tar.extractfile(name).read()
            if len(content) != expected['sanitized_bytes'] or hashlib.sha256(content).hexdigest() != expected['sanitized_sha256']:
                raise ValueError(f'{archive}: indexed content mismatch')
        for name in ('manifest.json', 'results.json', 'export-manifest.json'):
            if (folder/name).read_bytes() != tar.extractfile(name).read():
                raise ValueError(f'{folder/name}: standalone copy differs')
        verified += len(names)
print(json.dumps({'verified_phases': len(artifacts), 'verified_archive_files': verified}))
