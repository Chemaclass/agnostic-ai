#!/usr/bin/env python3
"""Replay the Caveman source investigation without vendor or model requests."""
import argparse
import hashlib
import json
import os
import platform
from pathlib import Path
import shutil
import socket
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--agnostic', required=True, type=Path)
parser.add_argument('--output', required=True, type=Path)
args = parser.parse_args()
binary = args.agnostic.resolve()
output = args.output.resolve()
output.mkdir(parents=True, exist_ok=False)
repo = Path(__file__).resolve().parents[3]
example = repo / 'docs/examples/rtk-and-caveman/packs/caveman'
builtin = repo / 'internal/builtins/data/caveman'
source = output / 'source'
project = output / 'project'
global_source = output / 'global-source'
claude = output / 'global-claude'
home = output / 'home'
cache = output / 'cache'
temporary = output / 'temporary'
git_config = output / 'gitconfig'
git_config.write_text('')
for path in (project, global_source, claude, home, cache, temporary):
    path.mkdir()
(home / 'Library/Caches').mkdir(parents=True)
env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
env.update(HOME=str(home), USERPROFILE=str(home), XDG_CACHE_HOME=str(cache),
           LOCALAPPDATA=str(cache), GIT_CONFIG_GLOBAL=str(git_config), GIT_CONFIG_NOSYSTEM='1',
           TMPDIR=str(temporary), TMP=str(temporary), TEMP=str(temporary),
           AGNOSTIC_AI_HOME=str(global_source), CLAUDE_CONFIG_DIR=str(claude),
           AGNOSTIC_AI_NO_UPDATE_CHECK='1')
env.pop('AGNOSTIC_AI_PROFILE', None)
results = {'steps': [], 'checks': [], 'source_revision': subprocess.check_output(
    ['git', 'rev-parse', 'HEAD'], cwd=repo, env=env, text=True).strip()}
daemon = None


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def check(name, condition):
    results['checks'].append({'name': name, 'passed': bool(condition)})
    if not condition:
        raise AssertionError(name)


def run(argv, cwd=project, success=True):
    result = subprocess.run([str(arg) for arg in argv], cwd=cwd, env=env,
                            text=True, capture_output=True, timeout=30)
    results['steps'].append({'argv': [str(arg) for arg in argv], 'cwd': str(cwd),
                             'exit': result.returncode, 'stdout': result.stdout,
                             'stderr': result.stderr})
    if success and result.returncode != 0:
        raise RuntimeError(str(results['steps'][-1]))
    if not success and result.returncode == 0:
        raise AssertionError(f'Unexpected success: {argv}')
    return result


def cli(*argv, cwd=project, success=True):
    return run([binary, *argv], cwd=cwd, success=success)


def configure(root, enabled):
    (root / 'agnostic-ai.yaml').write_text(
        'version: 1\ntargets: [claude]\nbuiltins: [' + ('caveman' if enabled else '') + ']\n')


def skill_files(root):
    return {str(path.relative_to(root)): digest(path)
            for path in root.rglob('*') if path.is_file()}


def assert_assets(native):
    original = (example / 'skills/caveman/SKILL.md').read_text().split('---', 2)[2].strip()
    check('Emitted skill retains the complete pinned instruction body',
          original in (native / 'SKILL.md').read_text())
    check('Only SKILL.md is emitted', [path.name for path in native.iterdir()] == ['SKILL.md'])


try:
    results['binary_sha256'] = digest(binary)
    results['environment'] = {'python': platform.python_version(),
                              'os': platform.system(), 'architecture': platform.machine()}
    run([binary, '--version'])
    expected = json.loads((example / 'UPSTREAM.json').read_text())
    check('Both source trees are byte identical', skill_files(example) == skill_files(builtin))
    check('The pack holds only SKILL.md and its manifest',
          sorted(skill_files(example)) == ['UPSTREAM.json', 'skills/caveman/SKILL.md'])
    check('The upstream skill matches the recorded digest',
          all(digest(example / 'skills/caveman' / name) == entry['sha256']
              for name, entry in expected['files'].items()))
    results['upstream_revision'] = expected['revision']
    results['asset_bytes_per_tree'] = sum(path.stat().st_size for path in example.rglob('*') if path.is_file())
    shutil.copytree(example, source)
    run(['git', 'init', '-q', str(source)])
    run(['git', 'add', '.'], cwd=source)
    run(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
         '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Pinned Caveman fixture'], cwd=source)
    run(['git', '-c', 'tag.gpgsign=false', 'tag', 'proof-v1'], cwd=source)
    fixture_sha = run(['git', 'rev-parse', 'HEAD'], cwd=source).stdout.strip()
    results['fixture_sha'] = fixture_sha
    with socket.socket() as sock:
        sock.bind(('127.0.0.1', 0))
        port = sock.getsockname()[1]
    daemon = subprocess.Popen(['git', 'daemon', '--reuseaddr', '--export-all',
                              '--listen=127.0.0.1', f'--port={port}', f'--base-path={output}',
                              str(output)], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    for _ in range(100):
        try:
            with socket.create_connection(('127.0.0.1', port), timeout=0.1):
                break
        except OSError:
            time.sleep(0.02)
    git_source = f'git://127.0.0.1:{port}/source'
    configure(project, False)
    cli('packs', 'add', str(example), '--name', 'caveman')
    local_lock = (project / 'agnostic.packs.lock').read_text()
    check('Local-directory install has no Git pin', 'sha:' not in local_lock and 'ref:' not in local_lock)
    cli('packs', 'add', git_source + '@' + fixture_sha, '--name', 'raw-sha', success=False)
    cli('packs', 'add', git_source + '@proof-v1', '--name', 'caveman')
    lock = (project / 'agnostic.packs.lock').read_bytes()
    check('Tagged Git install records ref and resolved commit',
          b'ref: proof-v1' in lock and fixture_sha.encode() in lock)
    cli('sync', '--all')
    native = project / '.claude/skills/caveman'
    assert_assets(native)
    first = skill_files(native)
    cli('sync', '--all')
    cli('sync', '--check')
    check('Repeat sync retains native bytes and lock', first == skill_files(native)
          and lock == (project / 'agnostic.packs.lock').read_bytes())
    cli('packs', 'update', 'caveman')
    check('Update at the retained tag keeps the same commit', lock == (project / 'agnostic.packs.lock').read_bytes())
    (source / 'proof-update.txt').write_text('Moved tag fixture.\n')
    run(['git', 'add', 'proof-update.txt'], cwd=source)
    run(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
         '-c', 'commit.gpgsign=false', 'commit', '-qm', 'Move fixture tag'], cwd=source)
    moved_sha = run(['git', 'rev-parse', 'HEAD'], cwd=source).stdout.strip()
    run(['git', '-c', 'tag.gpgsign=false', 'tag', '-f', 'proof-v1'], cwd=source)
    cli('packs', 'update', 'caveman')
    check('Update follows a moved tag rather than the previous recorded commit',
          moved_sha != fixture_sha and moved_sha.encode() in
          (project / 'agnostic.packs.lock').read_bytes())
    cli('packs', 'remove', 'caveman')
    cli('sync', '--all')
    cli('sync', '--check')
    check('Pack removal disables and removes the owned skill', not (native / 'SKILL.md').exists())
    cli('packs', 'add', git_source + '@proof-v1', '--name', 'caveman')
    configure(project, True)
    cli('sync', '--all')
    check('Pack and builtin with the same name emit one project skill',
          len(list((project / '.claude/skills').glob('*/SKILL.md'))) == 1)
    cli('packs', 'remove', 'caveman')
    cli('sync', '--all')
    check('Removing the pack reveals the still-enabled builtin', (native / 'SKILL.md').exists())
    configure(project, False)
    cli('sync', '--all')
    check('Removing the last configured supplier removes the owned output',
          not (native / 'SKILL.md').exists())
    authored = project / '.agnostic-ai/skills/caveman/SKILL.md'
    authored.parent.mkdir(parents=True)
    authored.write_text('---\nname: caveman\ndescription: My own skill\n---\nMy authored instructions.\n')
    authored_before = authored.read_bytes()
    cli('packs', 'add', git_source + '@proof-v1', '--name', 'caveman')
    configure(project, True)
    cli('sync', '--all')
    check('Project skill overrides pack and builtin without a second native skill',
          'My authored instructions.' in (native / 'SKILL.md').read_text()
          and len(list((project / '.claude/skills').glob('*/SKILL.md'))) == 1)
    cli('packs', 'remove', 'caveman')
    configure(project, False)
    cli('sync', '--all')
    check('Removing both suppliers retains the authored source and its output',
          authored.read_bytes() == authored_before
          and 'My authored instructions.' in (native / 'SKILL.md').read_text())
    configure(global_source, False)
    for operation in [('add', str(example)), ('update', 'caveman'), ('remove', 'caveman')]:
        refused = cli('packs', *operation, cwd=global_source, success=False)
        check(f'Global home refuses packs {operation[0]}', 'global home' in refused.stderr)
    check('Refused global pack operations create no pack or lock',
          not (global_source / 'agnostic.packs.lock').exists()
          and not (global_source / '.agnostic-ai/packs').exists())
    configure(global_source, True)
    cli('sync', '--global', '-t', 'claude')
    global_native = claude / 'skills/caveman'
    assert_assets(global_native)
    first = skill_files(global_native)
    cli('sync', '--global', '-t', 'claude')
    cli('sync', '--global', '-t', 'claude', '--check')
    check('Global builtin repeat sync preserves output', first == skill_files(global_native))
    configure(global_source, False)
    cli('sync', '--global', '-t', 'claude')
    check('Disabling global builtin removes owned output', not global_native.exists())
    global_native.mkdir(parents=True)
    handwritten = global_native / 'SKILL.md'
    handwritten.write_text('---\nname: caveman\n---\nUnmanaged handwritten skill.\n')
    before = handwritten.read_bytes()
    configure(global_source, True)
    refused = cli('sync', '--global', '-t', 'claude', success=False)
    check('Global builtin refuses a different handwritten skill and preserves it',
          'unmanaged global skill collision' in refused.stderr and handwritten.read_bytes() == before)
    configure(global_source, False)
    cli('sync', '--global', '-t', 'claude')
    check('Disabled builtin leaves unowned global skill intact', handwritten.read_bytes() == before)
    results['status'] = 'PASS'
except Exception as error:
    results['status'] = 'FAIL'
    results['error'] = str(error)
finally:
    if daemon is not None:
        daemon.terminate()
        daemon.wait(timeout=5)
    (output / 'evidence.json').write_text(json.dumps(results, indent=2) + '\n')
    print(json.dumps({'status': results['status'], 'checks': len(results['checks']),
                      'evidence': str(output / 'evidence.json')}))
if results['status'] != 'PASS':
    raise SystemExit(1)
