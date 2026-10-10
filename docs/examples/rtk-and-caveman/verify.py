#!/usr/bin/env python3
"""Check local pack configuration and hook protocols without launching Claude."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

HERE = Path(__file__).resolve().parent
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--agnostic', type=Path, required=True)
parser.add_argument('--rtk', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True, help='New evidence directory')
args = parser.parse_args()
binary, rtk = args.agnostic.resolve(), args.rtk.resolve()
args.output.mkdir(parents=True, exist_ok=False)
root = args.output.resolve()
home = root / 'agnostic-home'
home.mkdir()
env = dict(os.environ, AGNOSTIC_AI_HOME=str(home))
env.pop('AGNOSTIC_AI_PROFILE', None)
env.pop('RTK_HOOK_AUDIT', None)
empty_path = root / 'empty-path'
empty_path.mkdir()
present_env = dict(env, PATH=str(rtk.parent) + os.pathsep + env.get('PATH', ''))
missing_env = dict(env, PATH=str(empty_path))
evidence = {'checks': [], 'runs': [], 'repeat_changes': []}
sentinel = {'matcher': 'Read', 'hooks': [{'type': 'command', 'command': 'printf sentinel'}]}


def check(name, condition):
    evidence['checks'].append({'name': name, 'passed': bool(condition)})
    if not condition:
        raise AssertionError(name)


def run(argv, project, process_env=env, payload=None, expected_exit=0):
    result = subprocess.run([str(x) for x in argv], cwd=project, env=process_env,
                            input=json.dumps(payload) if payload is not None else None,
                            text=True, capture_output=True)
    evidence['runs'].append({'argv': [str(x) for x in argv], 'project': project.name,
                             'exit': result.returncode, 'stdout': result.stdout,
                             'stderr': result.stderr})
    if result.returncode != expected_exit:
        raise RuntimeError(f'{argv}: {result.stderr or result.stdout}')
    return result


def project(name, extra_hook=None):
    p = root / name
    (p / '.agnostic-ai' / 'rules').mkdir(parents=True)
    (p / 'agnostic-ai.yaml').write_text('version: 1\ntargets: [claude]\nbuiltins: []\n')
    (p / '.agnostic-ai' / 'rules' / 'fixture.md').write_text(
        '---\nname: fixture\ndescription: Keep one source rule.\n---\n\nUse source specs.\n')
    (p / '.claude').mkdir()
    groups = [sentinel] + ([extra_hook] if extra_hook else [])
    (p / '.claude' / 'settings.json').write_text(json.dumps(
        {'env': {'EXAMPLE_SENTINEL': 'preserve'}, 'hooks': {'PreToolUse': groups}}, indent=2) + '\n')
    return p


def settings(p):
    return json.loads((p / '.claude' / 'settings.json').read_text())


def handlers(p):
    return [h for groups in settings(p).get('hooks', {}).values()
            for group in groups for h in group.get('hooks', [])]


def add(p, name):
    run([binary, 'packs', 'add', HERE / 'packs' / name, '--name', name], p)
    sync(p)


def remove(p, name):
    run([binary, 'packs', 'remove', name], p)
    sync(p)


def sync(p, process_env=env):
    run([binary, 'sync', '--all'], p, process_env)
    doc = settings(p)
    check(f'{p.name}: handwritten hook and environment survive sync',
          sentinel in doc['hooks']['PreToolUse'] and doc['env']['EXAMPLE_SENTINEL'] == 'preserve')


def snapshot(p, bookkeeping=True):
    return {str(f.relative_to(p)): hashlib.sha256(f.read_bytes()).hexdigest()
            for f in p.rglob('*') if f.is_file()
            and (bookkeeping or f.name not in ('.command-lock', '.sync-state'))}


def skill(p):
    return p / '.claude' / 'skills' / 'caveman'


try:
    p = project('independent-components')
    run([binary, '--version'], p)
    run([rtk, '--version'], p)
    sources = {name: (p / name).read_bytes() for name in
               ['agnostic-ai.yaml', '.agnostic-ai/rules/fixture.md']}
    upstream = json.loads((HERE / 'packs/caveman/UPSTREAM.json').read_text())
    for name, record in upstream['files'].items():
        data = (HERE / 'packs/caveman/skills/caveman' / name).read_bytes()
        check(f'Pinned upstream {name} digest matches', hashlib.sha256(data).hexdigest() == record['sha256'])
    add(p, 'rtk')
    check('RTK can be installed without Caveman', not skill(p).exists())
    command = next(h['command'] for h in handlers(p) if 'rtk hook claude' in h.get('command', ''))
    check('Exactly one RTK handler emitted', sum('rtk hook claude' in h.get('command', '') for h in handlers(p)) == 1)
    payload = {'hook_event_name': 'PreToolUse', 'tool_name': 'Bash', 'permission_mode': 'default',
               'tool_input': {'command': 'git status', 'description': 'preserve', 'timeout': 1000}}
    reply = run(['/bin/sh', '-c', command], p, present_env, payload)
    evidence['host_request'] = payload
    evidence['hook_reply'] = json.loads(reply.stdout)
    updated = json.loads(reply.stdout)['hookSpecificOutput']['updatedInput']
    check('Exact emitted command returns rewrite and preserves unrelated input',
          updated == dict(payload['tool_input'], command='rtk git status'))
    for text in ['rtk git status', 'printf unsupported']:
        probe = dict(payload, tool_input=dict(payload['tool_input'], command=text))
        reply = run(['/bin/sh', '-c', command], p, present_env, probe)
        check(f'No replacement for {text}', reply.stdout == '')
    marker = p / 'payload-executed'
    probe = dict(payload, tool_input={'command': f'touch {marker}'})
    reply = run(['/bin/sh', '-c', command], p, missing_env, probe)
    check('Missing RTK returns no reply and never executes payload', reply.stdout == '' and not marker.exists())
    add(p, 'caveman')
    evidence['native_settings'] = settings(p)
    source_skill = HERE / 'packs/caveman/skills/caveman'
    check('Default Caveman skill emitted without companion modes',
          (skill(p) / 'SKILL.md').exists() and not (skill(p).parent / 'ultracave').exists()
          and not (skill(p).parent / 'megacave').exists())
    source_body = (source_skill / 'SKILL.md').read_text().split('---', 2)[2].strip()
    check('Default skill instruction body preserved', source_body in (skill(p) / 'SKILL.md').read_text())
    for name in upstream['files']:
        if name != 'SKILL.md':
            check(f'Emitted {name} bytes preserved', (skill(p) / name).read_bytes() == (source_skill / name).read_bytes())
    before_all, before = snapshot(p), snapshot(p, False)
    sync(p)
    after_all = snapshot(p)
    evidence['repeat_changes'] = [{'path': name, 'before': before_all.get(name), 'after': after_all.get(name)}
                                  for name in sorted(set(before_all) | set(after_all))
                                  if before_all.get(name) != after_all.get(name)]
    check('Repeat sync preserves generated output and source bytes', before == snapshot(p, False))
    run([binary, 'sync', '--check'], p)
    sync(p, missing_env)
    check('Emitted output is independent of RTK on PATH', before == snapshot(p, False))
    run([binary, 'sync', '--check'], p, missing_env)
    remove(p, 'rtk')
    check('RTK removal leaves Caveman available', skill(p).exists() and not any('rtk hook' in h.get('command', '') for h in handlers(p)))
    add(p, 'rtk')
    remove(p, 'caveman')
    check('Caveman removal leaves RTK available', not skill(p).exists() and any('rtk hook' in h.get('command', '') for h in handlers(p)))
    remove(p, 'rtk')
    check('Canonical project source unchanged', all((p / name).read_bytes() == data for name, data in sources.items()))
    run([binary, 'sync', '--check'], p)

    old_command = 'bash .claude/hooks/rtk-rewrite.sh'
    old = {'matcher': 'Bash', 'hooks': [{'type': 'command', 'command': old_command, 'timeout': 10}]}
    p = project('existing-upstream-hook', old)
    add(p, 'caveman')
    check('Keep upstream RTK owner by selecting only Caveman pack', old in settings(p)['hooks']['PreToolUse'])
    backup = root / 'upstream-hook.json'
    run([sys.executable, HERE / 'transfer-hook.py', 'take', '--project', p,
         '--backup', backup, '--command', old_command], p)
    add(p, 'rtk')
    check('Explicit transfer replaces upstream command without a duplicate',
          not any(h.get('command') == old_command for h in handlers(p))
          and sum('rtk hook claude' in h.get('command', '') for h in handlers(p)) == 1)
    remove(p, 'rtk')
    run([sys.executable, HERE / 'transfer-hook.py', 'restore', '--project', p, '--backup', backup], p)
    remove(p, 'caveman')
    check('Restore upstream RTK ownership preserves original handler', old in settings(p)['hooks']['PreToolUse'])
    run([binary, 'sync', '--check'], p)
    unchanged = (p / '.claude/settings.json').read_bytes()
    run([sys.executable, HERE / 'transfer-hook.py', 'restore', '--project', p,
         '--backup', backup], p, expected_exit=2)
    check('Duplicate restore refuses without changing settings', (p / '.claude/settings.json').read_bytes() == unchanged)
    run([sys.executable, HERE / 'transfer-hook.py', 'take', '--project', p,
         '--backup', root / 'no-match.json', '--command', 'not the selected hook'], p, expected_exit=2)
    check('Unmatched transfer refuses without changing settings', (p / '.claude/settings.json').read_bytes() == unchanged)

    p = project('restore-with-no-hooks', old)
    empty_backup = root / 'empty-hooks-backup.json'
    run([sys.executable, HERE / 'transfer-hook.py', 'take', '--project', p,
         '--backup', empty_backup, '--command', old_command], p)
    (p / '.claude/settings.json').write_text(json.dumps({'env': {'EXAMPLE_SENTINEL': 'preserve'}}) + '\n')
    run([sys.executable, HERE / 'transfer-hook.py', 'restore', '--project', p,
         '--backup', empty_backup], p)
    check('Restore recreates missing hooks object and retains environment',
          settings(p)['hooks']['PreToolUse'] == [old] and settings(p)['env']['EXAMPLE_SENTINEL'] == 'preserve')

    p = project('existing-upstream-skill')
    skill(p).mkdir(parents=True)
    original = b'---\nname: caveman\ndescription: Upstream installed skill.\n---\n\nKeep this user-installed skill.\n'
    (skill(p) / 'SKILL.md').write_bytes(original)
    add(p, 'rtk')
    check('Keep upstream skill owner by selecting only RTK pack', (skill(p) / 'SKILL.md').read_bytes() == original)
    backup_skill = root / 'upstream-caveman'
    skill(p).rename(backup_skill)
    add(p, 'caveman')
    remove(p, 'caveman')
    skill(p).parent.mkdir(parents=True, exist_ok=True)
    backup_skill.rename(skill(p))
    remove(p, 'rtk')
    check('Explicit skill transfer and restore retain upstream bytes', (skill(p) / 'SKILL.md').read_bytes() == original)
    evidence['status'] = 'PASS'
except Exception as error:
    evidence['status'] = 'FAIL'
    evidence['error'] = str(error)
finally:
    (root / 'evidence.json').write_text(json.dumps(evidence, indent=2) + '\n')
    print(json.dumps({'status': evidence['status'], 'checks': len(evidence['checks']), 'evidence': str(root / 'evidence.json')}))
if evidence['status'] != 'PASS':
    raise SystemExit(1)
