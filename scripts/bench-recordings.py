#!/usr/bin/env python3
"""Record paired command timings, or compare the resulting JSON files."""
import argparse
from dataclasses import dataclass
import hashlib
import json
import math
import os
from pathlib import Path
import platform
import re
import shutil
import statistics
import stat
import subprocess
import sys
import time
import uuid

ROOT = Path(__file__).resolve().parent.parent
SHELL = ROOT / 'scripts/bench-commands.sh'
SCHEMA = 1
METHOD = 'alternating-restored-tree-v3'
NO_LEDGER_WRITE = {'target': 'unledgered', 'path': '.vscode/settings.json',
                   'action': 'leftover', 'bytes': 0}


def execute(argv, cwd=None, env=None, expected=0):
    result = subprocess.run(argv, cwd=cwd, env=env, capture_output=True, timeout=600)
    if result.returncode != expected:
        raise ValueError(f'{argv[0]} {argv[1:]}: expected exit {expected}, got {result.returncode}')
    return result


def shell_function(function, args, env=None):
    return execute(['bash', '-c', 'source "$1"; shift; ' + function + ' "$@"',
                    'bench-recordings', str(SHELL), *map(str, args)], env=env)


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


@dataclass(frozen=True)
class SnapshotEntry:
    name: str
    kind: bytes
    mode: int
    atime_ns: int
    mtime_ns: int
    content: bytes
    children: tuple


def entry_kind(info):
    if stat.S_ISLNK(info.st_mode):
        return b'link'
    if stat.S_ISDIR(info.st_mode):
        return b'directory'
    require(stat.S_ISREG(info.st_mode), 'fixture contains a special file')
    return b'file'


def snapshot_entries(root):
    entries = []
    with os.scandir(root) as children:
        for child in sorted(children, key=lambda item: item.name):
            info = child.stat(follow_symlinks=False)
            kind = entry_kind(info)
            content = (os.fsencode(os.readlink(child.path)) if kind == b'link' else
                       Path(child.path).read_bytes() if kind == b'file' else b'')
            descendants = snapshot_entries(child.path) if kind == b'directory' else ()
            entries.append(SnapshotEntry(child.name, kind, stat.S_IMODE(info.st_mode),
                                         info.st_atime_ns, info.st_mtime_ns, content, descendants))
    return tuple(entries)


def snapshot_identity(mode, entries):
    digest = hashlib.sha256()
    digest.update(str(mode).encode())

    def include(children, prefix=''):
        for child in children:
            if child.name == '.git':
                continue
            relative = prefix + child.name
            for item in (relative.encode(), child.kind, str(child.mode).encode(), child.content):
                digest.update(len(item).to_bytes(8, 'big'))
                digest.update(item)
            include(child.children, relative + '/')

    include(entries)
    return digest.hexdigest()


@dataclass(frozen=True)
class Snapshot:
    mode: int
    atime_ns: int
    mtime_ns: int
    entries: tuple
    identity: str

    def __init__(self, root):
        info = root.stat()
        mode = stat.S_IMODE(info.st_mode)
        entries = snapshot_entries(root)
        object.__setattr__(self, 'mode', mode)
        object.__setattr__(self, 'atime_ns', info.st_atime_ns)
        object.__setattr__(self, 'mtime_ns', info.st_mtime_ns)
        object.__setattr__(self, 'entries', entries)
        object.__setattr__(self, 'identity', snapshot_identity(mode, entries))


def tree_identity(root):
    return Snapshot(root).identity


def restore_entries(expected, parent):
    with os.scandir(parent) as children:
        actual = {child.name: child for child in children}
    expected_names = {child.name for child in expected}
    for name in actual.keys() - expected_names:
        child = actual[name]
        if child.is_dir(follow_symlinks=False):
            print('Removing extra benchmark directory: ' + child.path, file=sys.stderr, flush=True)
            shutil.rmtree(child.path)
        else:
            os.unlink(child.path)
    for child in expected:
        path = parent / child.name
        current = actual.get(child.name)
        info = current.stat(follow_symlinks=False) if current else None
        require(info is None or entry_kind(info) == child.kind, 'fixture path changed type: ' + str(path))
        if child.kind == b'link':
            target = os.fsdecode(child.content)
            if info is None or os.readlink(path) != target:
                if info is not None:
                    path.unlink()
                path.symlink_to(target)
            require(os.fsencode(os.readlink(path)) == child.content and
                    stat.S_IMODE(path.lstat().st_mode) == child.mode, 'fixture link differs: ' + str(path))
            continue
        restored = info is None
        if child.kind == b'directory':
            if info is None:
                path.mkdir(mode=child.mode)
            if info is None or stat.S_IMODE(info.st_mode) != child.mode:
                path.chmod(child.mode)
            restore_entries(child.children, path)
        else:
            if info is not None and stat.S_IMODE(info.st_mode) != child.mode:
                path.chmod(child.mode)
            if info is None or path.read_bytes() != child.content:
                path.write_bytes(child.content)
                restored = True
                require(path.read_bytes() == child.content, 'fixture write differs: ' + str(path))
            if info is None:
                path.chmod(child.mode)
        require(stat.S_IMODE(path.stat().st_mode) == child.mode, 'fixture permissions differ: ' + str(path))
        if restored or child.kind == b'directory' or info.st_mtime_ns != child.mtime_ns:
            os.utime(path, ns=(child.atime_ns, child.mtime_ns))


def restore_tree(snapshot, active):
    snapshot = snapshot if isinstance(snapshot, Snapshot) else Snapshot(snapshot)
    if not active.exists():
        active.mkdir(mode=snapshot.mode)
    require(active.is_dir() and not active.is_symlink(), 'active fixture is not a directory')
    active.chmod(snapshot.mode)
    restore_entries(snapshot.entries, active)
    os.utime(active, ns=(snapshot.atime_ns, snapshot.mtime_ns))
    require(stat.S_IMODE(active.stat().st_mode) == snapshot.mode, 'active fixture permissions differ')
    return snapshot.identity


def binary_identity(binary, revision, dirty):
    metadata = execute(['go', 'version', '-m', str(binary)]).stdout.decode()
    fields = {}
    for line in metadata.splitlines():
        if line.strip().startswith('build\t') and '=' in line:
            key, value = line.strip().split('\t', 1)[1].split('=', 1)
            fields[key] = value
    embedded = fields.get('vcs.revision')
    require(not embedded or embedded == revision, 'supplied revision differs from binary build metadata')
    modified = fields.get('vcs.modified')
    require(modified is None or dirty == 'unknown' or (modified == 'true') == (dirty == 'dirty'),
            'supplied dirty state differs from binary build metadata')
    return {'sha256': sha256(binary), 'revision': revision,
            'revision_source': 'go-build' if embedded else 'operator',
            'dirty': ('dirty' if modified == 'true' else 'clean') if modified is not None else dirty}


def isolated_environment(home):
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('AGNOSTIC_AI_', 'CLAUDE_', 'CODEX_', 'GIT_'))}
    for name, subdir in [('HOME', 'home'), ('XDG_CONFIG_HOME', 'config'),
                         ('XDG_CACHE_HOME', 'cache'), ('XDG_DATA_HOME', 'data'),
                         ('CODEX_HOME', 'codex'), ('CLAUDE_CONFIG_DIR', 'claude'),
                         ('AGNOSTIC_AI_HOME', 'agnostic')]:
        path = home / subdir
        path.mkdir(mode=0o700, parents=True, exist_ok=True)
        env[name] = str(path)
    env.update(AGNOSTIC_AI_NO_UPDATE_CHECK='1', GIT_CONFIG_NOSYSTEM='1',
               GIT_CONFIG_GLOBAL=os.devnull, LC_ALL='C', TZ='UTC')
    return env


def command_plan():
    plan = []
    for mode in ('synced', 'no-ledger'):
        lines = shell_function('bench_commands_list', [mode]).stdout.decode().splitlines()
        for line in lines:
            argv = line.split()
            if argv == ['doctor']:
                argv.append('--json')
            plan.append({'mode': mode, 'argv': argv,
                         'expected_exit': 1 if argv == ['doctor', '--json'] else 0})
    return plan


def expected_doctor_summary(mode):
    require(mode in ('synced', 'no-ledger'), 'unknown doctor fixture mode')
    return {'kind': 'fixture-doctor-findings', 'mode': mode, 'untrusted_hooks': 5,
            'writes': [dict(NO_LEDGER_WRITE)] if mode == 'no-ledger' else []}


def doctor_diagnostic(stdout, mode='synced'):
    try:
        report = json.loads(stdout)
    except (ValueError, UnicodeDecodeError) as error:
        raise ValueError('doctor did not produce a diagnostic report') from error
    require(isinstance(report, dict) and report.get('version') == '1' and report.get('command') == 'doctor',
            'invalid doctor diagnostic report')
    summary = expected_doctor_summary(mode)
    require(report.get('errors') == [] and report.get('writes') == summary['writes'],
            'doctor reported an operational error or unexpected drift')
    lint = report.get('lint')
    require(isinstance(lint, list) and all(isinstance(item, dict) and item.get('severity') == 'warn' for item in lint),
            'doctor reported unexpected lint findings')
    findings = report.get('hook_trust')
    require(isinstance(findings, list) and len(findings) == 5, 'doctor did not report the five fixture hooks')
    commands = set()
    for index, finding in enumerate(findings):
        require(isinstance(finding, dict) and finding.get('status') == 'untrusted' and not finding.get('problem'),
                'doctor reported an operational hook trust failure')
        require(finding.get('event') == 'PostToolUse' and finding.get('group') == 0 and
                type(finding.get('handler')) is int and finding['handler'] == index,
                'doctor hook positions differ from fixture')
        command = finding.get('hook')
        require(isinstance(command, str), 'doctor hook lacks command')
        commands.add(command.removeprefix('export AGNOSTIC_AI_TARGET=codex; '))
    require(commands == {f'echo hook-{index:02d}' for index in range(5)}, 'doctor hook commands differ from fixture')
    return summary


def verify_binaries(binaries, records):
    for binary, result in zip(binaries, records):
        require(sha256(binary) == result['binary']['sha256'], 'binary changed during recording: ' + str(binary))


def record(args):
    if args.runs < 6 or args.specs not in (100, 500):
        raise ValueError('--runs must be at least 6; --specs must be 100 or 500')
    if not args.machine.strip() or '\n' in args.machine or '\r' in args.machine:
        raise ValueError('--machine needs one nonempty line')
    binaries = [Path(args.baseline).resolve(), Path(args.candidate).resolve()]
    revisions = [args.baseline_revision, args.candidate_revision]
    for binary, revision in zip(binaries, revisions):
        if not binary.is_file() or not os.access(binary, os.X_OK):
            raise ValueError(f'not an executable: {binary}')
        if not re.fullmatch(r'[0-9a-f]{40}', revision):
            raise ValueError('producing revisions must be full lowercase Git commit hashes')
    output = Path(args.record).resolve()
    output.mkdir(parents=True, exist_ok=False)
    env = isolated_environment(output / 'environment')
    identities = [binary_identity(binary, revision, dirty)
                  for binary, revision, dirty in zip(binaries, revisions, [args.baseline_dirty, args.candidate_dirty])]
    go_versions = [execute(['go', 'version', str(binary)]).stdout.decode().strip().split()[-1]
                   for binary in binaries]
    require(go_versions[0] == go_versions[1], 'incomparable producing Go versions')
    # Keep the compiler's existing caches while isolating all tool configuration.
    go_paths = json.loads(execute(['go', 'env', '-json', 'GOCACHE', 'GOMODCACHE', 'GOPATH']).stdout)
    setup_env = dict(env, **go_paths)
    fixture = output / 'synced'
    fixture.mkdir()
    plan = command_plan()
    session = str(uuid.uuid4())
    records = []
    for label, identity, go_version in zip(('baseline', 'candidate'), identities, go_versions):
        records.append({'schema': SCHEMA, 'method': METHOD, 'session': session, 'role': label,
                        'environment': {'machine': args.machine, 'os': platform.system(),
                                        'architecture': platform.machine(), 'go': go_version},
                        'binary': identity,
                        'fixture': {'specs': args.specs}, 'warmups': 1, 'runs': args.runs,
                        'commands': [dict(item, warmups=[], samples=[]) for item in plan],
                        'status': 'incomplete'})
    try:
        shell_function('bench_commands_prepare', [binaries[0], fixture, args.specs], setup_env)
        no_ledger = output / 'no-ledger'
        shutil.copytree(fixture, no_ledger, symlinks=True)
        shell_function('bench_commands_no_ledger', [no_ledger], env)
        snapshots = {mode: Snapshot(output / mode) for mode in ('synced', 'no-ledger')}
        identities = {mode: snapshot.identity for mode, snapshot in snapshots.items()}
        for result in records:
            result['fixture']['identities'] = identities
        active = output / 'active'
        current_mode = None
        for index, item in enumerate(plan):
            print(f"[{index + 1}/{len(plan)}] {item['mode']} / {' '.join(item['argv'])}",
                  file=sys.stderr, flush=True)
            if item['mode'] != current_mode:
                if active.exists():
                    shutil.rmtree(active)
                current_mode = item['mode']
            for iteration in range(args.runs + 1):
                for side, binary in enumerate(binaries):
                    require(restore_tree(snapshots[item['mode']], active) == identities[item['mode']],
                            'active fixture differs from snapshot')
                    start = time.perf_counter_ns()
                    result = subprocess.run([str(binary), *item['argv']], cwd=active, env=env,
                                            stdout=subprocess.PIPE if item['argv'] == ['doctor', '--json'] else subprocess.DEVNULL,
                                            stderr=subprocess.DEVNULL,
                                            timeout=600)
                    elapsed = (time.perf_counter_ns() - start) / 1_000_000
                    sample = {'milliseconds': elapsed, 'actual_exit': result.returncode}
                    records[side]['commands'][index]['warmups' if iteration == 0 else 'samples'].append(sample)
                    if item['argv'] == ['doctor', '--json']:
                        sample['diagnostic'] = doctor_diagnostic(result.stdout, item['mode'])
                    if result.returncode != item['expected_exit']:
                        raise ValueError(f"{item['mode']} {item['argv']} ({records[side]['role']}): "
                                         f"expected {item['expected_exit']}, got {result.returncode}")
        verify_binaries(binaries, records)
        for result in records:
            result['status'] = 'complete'
    finally:
        for result in records:
            (output / (result['role'] + '.json')).write_text(json.dumps(result, indent=2) + '\n')
    print(compare(records[0], records[1]))
    print(f'Raw recordings and retained fixtures: {output}')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(data):
    require(isinstance(data, dict), 'recording must be an object')
    require(type(data.get('schema')) is int and data['schema'] == SCHEMA, 'unsupported schema')
    require(data.get('method') == METHOD, 'unsupported measurement method')
    require(data.get('status') == 'complete', 'recording is incomplete')
    require(data.get('role') in ('baseline', 'candidate'), 'missing recording role')
    require(isinstance(data.get('session'), str) and bool(data['session']), 'missing session identity')
    require(type(data.get('warmups')) is int and data['warmups'] == 1, 'one warmup is required')
    require(type(data.get('runs')) is int and data['runs'] >= 6, 'at least six samples are required')
    env = data.get('environment')
    require(isinstance(env, dict) and set(env) == {'machine', 'os', 'architecture', 'go'}, 'missing environment metadata')
    require(all(isinstance(value, str) and value.strip() for value in env.values()), 'empty environment metadata')
    binary = data.get('binary')
    require(isinstance(binary, dict), 'missing binary metadata')
    for field, size in [('revision', 40), ('sha256', 64)]:
        require(isinstance(binary.get(field), str) and re.fullmatch('[0-9a-f]{' + str(size) + '}', binary[field]),
                'invalid binary ' + field)
    require(binary.get('revision_source') in ('go-build', 'operator') and
            binary.get('dirty') in ('clean', 'dirty', 'unknown'), 'missing binary provenance')
    fixture = data.get('fixture')
    require(isinstance(fixture, dict) and type(fixture.get('specs')) is int and fixture['specs'] in (100, 500), 'invalid fixture size')
    identities = fixture.get('identities')
    require(isinstance(identities, dict) and set(identities) == {'synced', 'no-ledger'}, 'missing fixture identities')
    require(all(isinstance(value, str) and re.fullmatch('[0-9a-f]{64}', value) for value in identities.values()), 'invalid fixture identity')
    commands = data.get('commands')
    require(isinstance(commands, list) and bool(commands), 'missing commands')
    keys = []
    for item in commands:
        require(isinstance(item, dict), 'invalid command')
        require(isinstance(item.get('mode'), str) and item['mode'] in identities and isinstance(item.get('argv'), list)
                and item['argv'] and all(isinstance(value, str) and value for value in item['argv']), 'invalid command identity')
        require(type(item.get('expected_exit')) is int and item['expected_exit'] in (0, 1), 'invalid expected exit')
        require(item['expected_exit'] == (1 if item['argv'] == ['doctor', '--json'] else 0), 'unexpected command exit policy')
        keys.append((item['mode'], tuple(item['argv'])))
        for field, count in [('warmups', 1), ('samples', data['runs'])]:
            samples = item.get(field)
            require(isinstance(samples, list) and len(samples) == count, 'missing ' + field)
            for sample in samples:
                require(isinstance(sample, dict), 'invalid sample')
                value = sample.get('milliseconds')
                require(type(value) in (int, float) and math.isfinite(value) and value > 0, 'invalid duration')
                if item['argv'] == ['doctor', '--json']:
                    require(sample.get('diagnostic') == expected_doctor_summary(item['mode']), 'unexpected doctor diagnostic')
                require(type(sample.get('actual_exit')) is int and sample['actual_exit'] == item['expected_exit'], 'unexpected command exit')
    expected_plan = command_plan()
    require([(item['mode'], item['argv'], item['expected_exit']) for item in commands] ==
            [(item['mode'], item['argv'], item['expected_exit']) for item in expected_plan],
            'incomplete or unsupported command list')
    require(len(set(keys)) == len(keys), 'duplicate commands')
    require(set(item[0] for item in keys) == {'synced', 'no-ledger'}, 'both fixture modes are required')
    return data


def compare(baseline, candidate):
    validate(baseline)
    validate(candidate)
    require(baseline['role'] == 'baseline' and candidate['role'] == 'candidate', 'expected baseline then candidate')
    for field in ('schema', 'method', 'session', 'environment', 'fixture', 'warmups', 'runs'):
        require(baseline[field] == candidate[field], 'incomparable ' + field)
    def signature(item):
        return item['mode'], item['argv'], item['expected_exit']
    require([signature(item) for item in baseline['commands']] == [signature(item) for item in candidate['commands']],
            'incomparable command lists')
    rows = ['mode / command | n | baseline median [min,max] ms | candidate median [min,max] ms | delta ms | delta %']
    for before, after in zip(baseline['commands'], candidate['commands']):
        left = [sample['milliseconds'] for sample in before['samples']]
        right = [sample['milliseconds'] for sample in after['samples']]
        bm, cm = statistics.median(left), statistics.median(right)
        rows.append(f"{before['mode']} / {' '.join(before['argv'])} | {len(left)} | "
                    f'{bm:.3f} [{min(left):.3f},{max(left):.3f}] | '
                    f'{cm:.3f} [{min(right):.3f},{max(right):.3f}] | {cm-bm:+.3f} | {(cm/bm-1)*100:+.2f}%')
    return '\n'.join(rows)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument('--record', metavar='NEW_DIRECTORY')
    mode.add_argument('--compare', nargs=2, metavar=('BASELINE_JSON', 'CANDIDATE_JSON'))
    parser.add_argument('--baseline')
    parser.add_argument('--candidate')
    parser.add_argument('--baseline-revision')
    parser.add_argument('--candidate-revision')
    parser.add_argument('--machine')
    parser.add_argument('--baseline-dirty', choices=('clean', 'dirty', 'unknown'), default='unknown')
    parser.add_argument('--candidate-dirty', choices=('clean', 'dirty', 'unknown'), default='unknown')
    parser.add_argument('--specs', type=int, default=500)
    parser.add_argument('--runs', type=int, default=6)
    args = parser.parse_args()
    try:
        if args.compare:
            def object_pairs(pairs):
                result = {}
                for key, value in pairs:
                    require(key not in result, 'duplicate JSON key: ' + key)
                    result[key] = value
                return result

            def invalid_constant(value):
                raise ValueError('invalid JSON constant: ' + value)

            def load(path):
                return json.loads(Path(path).read_text(), object_pairs_hook=object_pairs,
                                  parse_constant=invalid_constant)
            print(compare(*(load(path) for path in args.compare)))
        else:
            if not all((args.baseline, args.candidate, args.baseline_revision, args.candidate_revision, args.machine)):
                parser.error('--record requires --baseline, --candidate, both --*-revision flags, and --machine')
            record(args)
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print(f'error: {error}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
