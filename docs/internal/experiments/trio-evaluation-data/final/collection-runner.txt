#!/usr/bin/env python3
"""Run a bounded, four-arm Claude evaluation against fixed local transcripts."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import random
import re
import shutil
import shlex
import math
import signal
import subprocess
import tempfile
import time

ARMS = ('concise', 'rtk', 'caveman', 'both')
ARM_BUILTINS = {'concise': (), 'rtk': ('rtk',), 'caveman': ('caveman',), 'both': ('rtk', 'caveman')}
COMMON_NATIVE_SKILLS = frozenset({'design', 'doctor', 'plugin-authoring'})
COMMON_NATIVE_TOOLS = frozenset({'Bash', 'Skill'})
EXPECTED_BUNDLED_PLUGIN_COUNT = 3
TASKS = {
    'payment': {'passed': 700, 'failed': 1, 'errors': [('tests::payment', 'tests/payment.rs:42', '200', '503')]},
    'unicode': {'passed': 300, 'failed': 2, 'errors': [('tests::cafe', 'tests/café.rs:17', '12', '18'), ('tests::timeout', 'tests/timeout.rs:88', '100', '250')]},
    'short': {'passed': 0, 'failed': 1, 'errors': [('tests::short', 'tests/short.rs:9', '1', '2')],
              'command': 'fixture-report short', 'rtk_rewrite': False},
}

def task_command(task_name):
    return TASKS[task_name].get('command', f'cargo test --test {task_name}')

def task_argv(task_name):
    return shlex.split(task_command(task_name))[1:]

def task_program(task_name):
    return shlex.split(task_command(task_name))[0]

def selected_tasks(selections):
    names = [name for group in selections for name in group] if selections is not None else list(TASKS)
    if not names or len(set(names)) != len(names):
        raise ValueError('Task selection must be nonempty and contain no duplicates')
    return names

def transcript(task):
    lines = [f'test tests::module_{i:03d} ... ok' for i in range(task['passed'])]
    lines += [f'test {name} ... FAILED' for name, _, _, _ in task['errors']]
    lines += ['', 'failures:']
    for name, path, expected, actual in task['errors']:
        lines += [f'---- {name} stdout ----', f'thread panicked at {path}: expected {expected}, got {actual}', '']
    lines += ['failures:'] + [f'    {item[0]}' for item in task['errors']]
    lines += [f"test result: FAILED. {task['passed']} passed; {task['failed']} failed; 0 ignored", '']
    return '\n'.join(lines).encode()

def oracle(answer, task):
    required = [f"{task['passed']} passed; {task['failed']} failed"]
    required += [value for error in task['errors'] for value in error[:2]]
    required += [f'expected {expected}, got {actual}' for _, _, expected, actual in task['errors']]
    failure_stated = bool(re.search(r'\bcommand failed\b', answer, flags=re.IGNORECASE))
    missing = [value for value in required if value not in answer]
    if not failure_stated:
        missing.append('explicit command failed statement')
    contradictions = []
    if any(int(count) != task['passed'] for count in re.findall(r'\b(\d+)\s+passed\b', answer, re.IGNORECASE)):
        contradictions.append('conflicting passed count')
    if any(int(count) != task['failed'] for count in re.findall(r'\b(\d+)\s+failed\b', answer, re.IGNORECASE)):
        contradictions.append('conflicting failed count')
    expected_pairs = {(expected, actual) for _, _, expected, actual in task['errors']}
    pairs = re.findall(r'\bexpected\s+(\d+)\s*,\s*got\s+(\d+)\b', answer, re.IGNORECASE)
    if any(pair not in expected_pairs for pair in pairs):
        contradictions.append('conflicting expected/actual pair')
    observed = []
    for number, line in enumerate(answer.splitlines(), 1):
        identifiers = re.findall(r'\btests::[\w:]+', line)
        paths = re.findall(r'\btests/[^\s`]+?\.rs:\d+', line)
        comparisons = re.findall(r'\bexpected\s+(\d+)\s*,\s*got\s+(\d+)\b', line, re.IGNORECASE)
        if not (identifiers or paths or comparisons):
            continue
        if len(identifiers) != 1 or len(paths) != 1 or len(comparisons) != 1:
            contradictions.append(f'ambiguous failure detail line {number}')
            continue
        detail = (identifiers[0], paths[0], *comparisons[0])
        if detail not in task['errors']:
            contradictions.append(f'incorrect failure detail line {number}')
        observed.append(detail)
    for error in task['errors']:
        if observed.count(error) != 1:
            missing.append(f'one linked failure line for {error[0]}')
    if re.search(r'\b(?:command (?:succeeded|passed|completed successfully|was successful)|exit(?:ed)?(?: with)?(?: code| status)?\s*[:=]?\s*0)\b|(?<!not )\ball tests passed\b', answer, re.IGNORECASE):
        contradictions.append('command success claim')
    return {'passed': not missing and not contradictions, 'missing': missing, 'contradictions': contradictions}

def digest_tree(root):
    digest = hashlib.sha256()
    for path in sorted(p for p in root.rglob('*') if p.is_file()):
        digest.update(str(path.relative_to(root)).encode()+b'\0')
        digest.update(path.read_bytes())
    return digest.hexdigest()

def sha256(data):
    return hashlib.sha256(data).hexdigest()

def file_manifest(root):
    return {str(path.relative_to(root)): {'sha256': sha256(path.read_bytes()), 'bytes': path.stat().st_size}
            for path in sorted(root.rglob('*')) if path.is_file()}

def skill_assets(root):
    return {str(path.relative_to(root)): sha256(path.read_bytes()) for path in root.rglob('*')
            if path.is_file() and path.name != 'SKILL.md'}

def generated_skill_body(path):
    body = skill_body(path)
    header = '<!-- Generated by agnostic-ai. Do not edit this file directly; edit specs under .agnostic-ai/ and run `agnostic-ai sync`. -->\n\n'
    if not body.startswith(header):
        raise ValueError(f'{path}: generated skill header missing')
    return body[len(header):]

def native_rtk_handler(settings):
    hooks = settings.get('hooks', {})
    if set(hooks) != {'PreToolUse'}:
        raise ValueError(f'Unexpected generated hook events: {sorted(hooks)}')
    groups = hooks['PreToolUse']
    if not isinstance(groups, list) or len(groups) != 1 or groups[0].get('matcher') != 'Bash':
        raise ValueError('Expected one generated Bash PreToolUse group')
    handlers = groups[0].get('hooks')
    if not isinstance(handlers, list) or len(handlers) != 1 or handlers[0].get('type') != 'command':
        raise ValueError('Expected one generated native command handler')
    command = handlers[0].get('command')
    if not isinstance(command, str) or not command.strip():
        raise ValueError('Generated native command is empty')
    return handlers[0]

def version(argv):
    result = subprocess.run(argv, capture_output=True, text=True, timeout=10)
    return result.stdout.strip() or result.stderr.strip()

def resolve_executable(candidate):
    selected = shutil.which(candidate) if candidate else None
    if not selected:
        raise ValueError(f'Executable unavailable: {candidate}')
    path = Path(selected).resolve(strict=True)
    if not path.is_file() or not os.access(path, os.X_OK):
        raise ValueError(f'Executable is not a file: {path}')
    return str(path)

def accounting(usage, estimated_cost):
    usage = usage if isinstance(usage, dict) else {}
    uncached = usage.get('input_tokens')
    created = usage.get('cache_creation_input_tokens')
    read = usage.get('cache_read_input_tokens')
    output = usage.get('output_tokens')
    counts = {'uncached_input_tokens': uncached, 'cache_creation_input_tokens': created,
              'cache_read_input_tokens': read, 'output_tokens': output}
    counter_presence = {key: isinstance(value, int) and not isinstance(value, bool) and value >= 0
                        for key, value in counts.items()}
    known_cost = isinstance(estimated_cost, (int, float)) and not isinstance(estimated_cost, bool) and math.isfinite(estimated_cost) and estimated_cost >= 0
    details = usage.get('output_tokens_details')
    thinking = details.get('thinking_tokens') if isinstance(details, dict) else None
    return {'uncached_input_tokens': uncached, 'cache_creation_input_tokens': created,
            'cache_read_input_tokens': read,
            'unweighted_total_input_tokens': uncached+created+read if all(counter_presence[key] for key in ('uncached_input_tokens', 'cache_creation_input_tokens', 'cache_read_input_tokens')) else None,
            'output_tokens': output, 'thinking_tokens_within_output': thinking,
            'counter_presence': counter_presence, 'cost_present': known_cost,
            'measurement_complete': all(counter_presence.values()) and known_cost,
            'cli_list_price_estimate_usd': estimated_cost if known_cost else None}

def bundled_plugin(plugin):
    location = str(plugin.get('path', ''))
    return (bool(location) and not location.startswith('/')
            and ('builtin' in location or 'bunfs' in location)
            and str(plugin.get('source', '')).endswith('@builtin'))

def host_inventory_evidence(init, caveman_enabled):
    skills = init.get('skills') or []
    tools = init.get('tools') or []
    plugins = init.get('plugins') or []
    identities = sorted((str(item.get('name', '')), str(item.get('source', '')),
                         str(item.get('path', ''))) for item in plugins if isinstance(item, dict))
    expected_skills = COMMON_NATIVE_SKILLS | ({'caveman'} if caveman_enabled else set())
    common_skills = sorted(set(value for value in skills if isinstance(value, str)) - {'caveman'})
    profile = {'tools': sorted(str(value) for value in tools), 'common_skills': common_skills,
               'bundled_plugins': [list(identity) for identity in identities]}
    good = (bool(init) and all(isinstance(value, str) for value in skills)
            and len(skills) == len(expected_skills) and set(skills) == expected_skills
            and all(isinstance(value, str) for value in tools)
            and len(tools) == len(COMMON_NATIVE_TOOLS) and set(tools) == COMMON_NATIVE_TOOLS
            and len(plugins) == len(identities) == EXPECTED_BUNDLED_PLUGIN_COUNT
            and len(set(identities)) == EXPECTED_BUNDLED_PLUGIN_COUNT
            and all(isinstance(plugin, dict) and plugin.get('name') and bundled_plugin(plugin)
                    for plugin in plugins)
            and not init.get('mcp_servers'))
    return good, profile

def comparable_host_profiles(rows):
    return bool(rows) and all(row.get('host_inventory_ok') and row.get('host_profile_sha256') for row in rows) and len({
        row.get('host_profile_sha256') for row in rows}) == 1

def account_live_row(spent, row, comparison_valid):
    cost = row.get('estimated_cost_usd')
    if cost is not None:
        spent += cost
    reasons = []
    if not comparison_valid:
        reasons.append('effective host inventory differs or is unexpected')
    if cost is None:
        reasons.append('no cost returned')
    return spent, reasons, cost is None

def skill_body(path):
    content = path.read_text()
    if content.startswith('---\n'):
        end = content.find('\n---\n', 4)
        if end < 0:
            raise ValueError(f'Unclosed skill frontmatter: {path}')
        return content[end+5:].lstrip('\n')
    return content

def loaded_skill_source(events, expected_directory):
    marker = 'Base directory for this skill: '
    matches = []
    for event in events:
        if event.get('type') != 'user' or not event.get('isSynthetic'):
            continue
        for block in event.get('message', {}).get('content', []):
            if not isinstance(block, dict):
                continue
            content = block.get('text', '')
            if content.startswith(marker) and '\n\n' in content:
                location, body = content.split('\n\n', 1)
                matches.append((Path(location[len(marker):]).resolve(), body))
    return len(matches) == 1 and matches[0] == (expected_directory.resolve(), skill_body(expected_directory/'SKILL.md'))

def execution_evidence(case, task_name):
    runs_path = case/'fixture-runs.txt'
    count = len(runs_path.read_text().splitlines()) if runs_path.exists() else 0
    args_path = case/'fixture-args.txt'
    observed = args_path.read_text().splitlines() if args_path.exists() else []
    return count, observed, count == 1 and observed == [task_program(task_name), *task_argv(task_name)]

def make_fake_program(case, bindir, task_name):
    program = task_program(task_name)
    expected_argv = task_argv(task_name)
    guard = ' || '.join([f'[ "${index}" != {shlex.quote(value)} ]'
                         for index, value in enumerate(expected_argv, 1)])
    fake_program = bindir/program
    fake_program.write_text('#!/bin/sh\nprintf "run\\n" >> ' + shlex.quote(str(case/'fixture-runs.txt')) + '\n'
                            'printf "%s\\n" ' + shlex.quote(program) + ' "$@" > ' + shlex.quote(str(case/'fixture-args.txt')) + '\n'
                            f'if [ "$#" -ne {len(expected_argv)} ] || {guard}; then exit 64; fi\n'
                            'cat ' + shlex.quote(str(case/'transcript.txt')) + '\nexit 7\n')
    fake_program.chmod(0o755)
    return fake_program

def make_rtk_shim(bindir, selected):
    shim = bindir/'rtk'
    root = bindir.parent
    shim.write_text('#!/usr/bin/env python3\nimport json, os, sys\n'
                    f'with open({str(root/"rtk-argv.jsonl")!r}, "a") as log:\n'
                    '    log.write(json.dumps(sys.argv[1:]) + "\\n")\n'
                    f'os.execv({str(selected)!r}, [{str(selected)!r}, *sys.argv[1:]])\n')
    shim.chmod(0o755)
    return shim

def rtk_call_evidence(case, task_name, enabled, rewritten):
    path = case/'rtk-argv.jsonl'
    calls = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
    processor = [call for call in calls if call == ['hook', 'claude']]
    host = [call for call in calls if call == ['cargo', 'test', '--test', task_name]]
    valid = (len(processor) == (1 if enabled else 0) and len(host) == (1 if rewritten else 0)
             and len(calls) == len(processor)+len(host))
    return calls, len(processor), len(host), valid

def task_call_counts(bash_uses, command):
    commands = [str(block.get('input', {}).get('command', '')) for block in bash_uses]
    test_calls = sum(value in (command, f'rtk {command}') for value in commands)
    recovery_reads = sum(value.startswith('rtk recall ') for value in commands)
    return test_calls, max(0, test_calls-1), recovery_reads

def completion_ok(exit_code, result):
    return exit_code == 0 and result.get('subtype') == 'success' and not result.get('permission_denials')

def self_test():
    assert selected_tasks(None) == list(TASKS)
    assert selected_tasks([['short']]) == ['short']
    for invalid in ([], [['short', 'short']], [['short'], ['short']]):
        try:
            selected_tasks(invalid)
        except ValueError:
            pass
        else:
            raise AssertionError(f'invalid task selection accepted: {invalid}')
    task = TASKS['payment']
    answer = '700 passed; 1 failed. Command failed. tests::payment tests/payment.rs:42 expected 200, got 503'
    assert oracle(answer, task)['passed']
    assert not oracle(answer.replace('700 passed', '701 passed'), task)['passed']
    assert not oracle(answer.replace('Command failed.', ''), task)['passed']
    assert 'conflicting passed count' in oracle(answer + ' 701 passed; 1 failed.', task)['contradictions']
    assert 'conflicting failed count' in oracle(answer + ' 700 passed; 0 failed.', task)['contradictions']
    assert 'conflicting expected/actual pair' in oracle(answer + ' expected 200, got 200.', task)['contradictions']
    assert 'command success claim' in oracle(answer + ' Command succeeded (exit code 0).', task)['contradictions']
    assert oracle(answer + ' Not all tests passed.', task)['passed']
    unicode_answer = ('300 passed; 2 failed. Command failed.\n'
                      '- tests::cafe | tests/café.rs:17 | expected 12, got 18\n'
                      '- tests::timeout | tests/timeout.rs:88 | expected 100, got 250')
    assert oracle(unicode_answer, TASKS['unicode'])['passed']
    swapped = unicode_answer.replace('expected 12, got 18', 'expected 100, got 250', 1).replace(
        'tests/timeout.rs:88 | expected 100, got 250', 'tests/timeout.rs:88 | expected 12, got 18', 1)
    assert not oracle(swapped, TASKS['unicode'])['passed']
    assert 'incorrect failure detail line 2' in oracle(swapped, TASKS['unicode'])['contradictions']
    combined = unicode_answer.replace('\n- tests::timeout', ' - tests::timeout')
    assert not oracle(combined, TASKS['unicode'])['passed']
    assert 'ambiguous failure detail line 2' in oracle(combined, TASKS['unicode'])['contradictions']
    original_swapped = ('300 passed; 2 failed. Command failed. '
                        'tests::cafe tests/café.rs:17 expected 100, got 250. '
                        'tests::timeout tests/timeout.rs:88 expected 12, got 18.')
    assert not oracle(original_swapped, TASKS['unicode'])['passed']
    short_answer = '0 passed; 1 failed. Command failed. tests::short | tests/short.rs:9 | expected 1, got 2'
    assert oracle(short_answer, TASKS['short'])['passed']
    assert not oracle(short_answer.replace('expected 1, got 2', 'expected 2, got 1'), TASKS['short'])['passed']
    unknown = accounting(None, None)
    assert not unknown['measurement_complete'] and unknown['unweighted_total_input_tokens'] is None
    assert all(not present for present in unknown['counter_presence'].values())
    partial = accounting({'input_tokens': 0}, None)
    assert partial['uncached_input_tokens'] == 0 and partial['cache_read_input_tokens'] is None
    assert not partial['measurement_complete'] and partial['counter_presence']['uncached_input_tokens']
    complete_usage = {'input_tokens': 0, 'cache_creation_input_tokens': 2,
                      'cache_read_input_tokens': 3, 'output_tokens': 4,
                      'output_tokens_details': {'thinking_tokens': 1}}
    measured = accounting(complete_usage, 0.01)
    assert measured['measurement_complete'] and measured['unweighted_total_input_tokens'] == 5
    assert measured['thinking_tokens_within_output'] == 1
    common_plugins = [{'name': name, 'source': name+'@builtin', 'path': 'builtin'}
                      for name in ('bundled-a', 'bundled-b', 'bundled-c')]
    base_init = {'skills': sorted(COMMON_NATIVE_SKILLS), 'tools': ['Bash', 'Skill'],
                 'plugins': common_plugins, 'mcp_servers': []}
    assert host_inventory_evidence(base_init, False)[0]
    cave_init = json.loads(json.dumps(base_init))
    cave_init['skills'].append('caveman')
    assert host_inventory_evidence(cave_init, True)[0]
    assert host_inventory_evidence(base_init, False)[1] == host_inventory_evidence(cave_init, True)[1]
    bad_init = json.loads(json.dumps(base_init))
    bad_init['tools'] = ['Bash']
    assert not host_inventory_evidence(bad_init, False)[0]
    bad_init['tools'] = ['Bash', 'Skill']
    bad_init['skills'] = []
    assert not host_inventory_evidence(bad_init, False)[0]
    bad_init['skills'] = sorted(COMMON_NATIVE_SKILLS | {'unexpected'})
    assert not host_inventory_evidence(bad_init, False)[0]
    bad_init = json.loads(json.dumps(base_init))
    bad_init['plugins'][0]['path'] = '/personal/plugin'
    assert not host_inventory_evidence(bad_init, False)[0]
    bad_init = json.loads(json.dumps(base_init))
    bad_init['plugins'].pop()
    assert not host_inventory_evidence(bad_init, False)[0]
    bad_init = json.loads(json.dumps(base_init))
    bad_init['plugins'][0]['name'] = 'different-bundled'
    assert host_inventory_evidence(bad_init, False)[0]
    assert host_inventory_evidence(bad_init, False)[1] != host_inventory_evidence(base_init, False)[1]
    profile = host_inventory_evidence(base_init, False)[1]
    other_profile = host_inventory_evidence(bad_init, False)[1]
    profile_hash = lambda value: sha256(json.dumps(value, sort_keys=True).encode())
    assert comparable_host_profiles([{'host_inventory_ok': True, 'host_profile_sha256': profile_hash(profile)},
                                     {'host_inventory_ok': True, 'host_profile_sha256': profile_hash(profile)}])
    assert not comparable_host_profiles([{'host_inventory_ok': True, 'host_profile_sha256': profile_hash(profile)},
                                         {'host_inventory_ok': True, 'host_profile_sha256': profile_hash(other_profile)}])
    spent, reasons, unknown = account_live_row(0, {'estimated_cost_usd': 0.05}, False)
    assert spent == 0.05 and reasons == ['effective host inventory differs or is unexpected'] and not unknown
    spent, reasons, unknown = account_live_row(spent, {'estimated_cost_usd': None}, True)
    assert spent == 0.05 and reasons == ['no cost returned'] and unknown
    spent, reasons, unknown = account_live_row(0, {'estimated_cost_usd': None}, False)
    assert spent == 0 and len(reasons) == 2 and unknown
    with tempfile.TemporaryDirectory(prefix='trio-self-test-') as directory:
        case = Path(directory)
        (case/'fixture-runs.txt').write_text('run\n')
        (case/'fixture-args.txt').write_text('cargo\ntest\n--test\npayment\n')
        assert execution_evidence(case, 'payment')[2]
        (case/'fixture-runs.txt').write_text('run\nrun\n')
        assert not execution_evidence(case, 'payment')[2]
        (case/'fixture-runs.txt').write_text('run\n')
        (case/'fixture-args.txt').write_text('cargo\ntest\n--test\nwrong\n')
        assert not execution_evidence(case, 'payment')[2]
        (case/'fixture-args.txt').write_text('fixture-report\nshort\n')
        assert execution_evidence(case, 'short')[2]
        (case/'fixture-runs.txt').write_text('run\nrun\n')
        assert not execution_evidence(case, 'short')[2]
        (case/'fixture-runs.txt').unlink()
        (case/'fixture-args.txt').unlink()
        (case/'transcript.txt').write_bytes(transcript(TASKS['short']))
        bindir = case/'bin'
        bindir.mkdir()
        fixture = make_fake_program(case, bindir, 'short')
        launched_fixture = subprocess.run([str(fixture), 'short'], capture_output=True)
        assert launched_fixture.returncode == 7 and launched_fixture.stdout == transcript(TASKS['short'])
        assert launched_fixture.stderr == b'' and execution_evidence(case, 'short')[2]
        wrong_fixture = subprocess.run([str(fixture), 'wrong'], capture_output=True)
        assert wrong_fixture.returncode == 64
        assert not execution_evidence(case, 'short')[2]
        selected = case/'selected-rtk'
        selected.write_text('#!/bin/sh\nprintf "selected:%s\\n" "$1"\n')
        selected.chmod(0o755)
        relative_selected = os.path.relpath(selected, Path.cwd())
        assert resolve_executable(relative_selected) == str(selected.resolve())
        fake_claude = case/'fake-claude'
        fake_claude.write_text('#!/bin/sh\nexit 0\n')
        fake_claude.chmod(0o755)
        relative_claude = os.path.relpath(fake_claude, Path.cwd())
        assert resolve_executable(relative_claude) == str(fake_claude.resolve())
        shim = make_rtk_shim(bindir, selected)
        env = dict(os.environ, PATH=str(bindir)+os.pathsep+'/usr/bin:/bin')
        assert shutil.which('selected-rtk', path=env['PATH']) is None
        launched = subprocess.run(['rtk', 'cargo', 'test', '--test', 'payment'],
                                  env=env, capture_output=True, text=True, check=True)
        assert launched.stdout.strip() == 'selected:cargo'
        assert not rtk_call_evidence(case, 'payment', True, True)[3]
        assert shim.is_file()
        subprocess.run(['rtk', 'hook', 'claude'], env=env, capture_output=True, check=True)
        assert rtk_call_evidence(case, 'payment', True, True)[3]
        subprocess.run(['rtk', 'cargo', 'test', '--test', 'payment'], env=env, capture_output=True, check=True)
        assert not rtk_call_evidence(case, 'payment', True, True)[3]
        (case/'rtk-argv.jsonl').write_text(json.dumps(['hook', 'claude'])+'\n')
        assert rtk_call_evidence(case, 'short', True, False)[3]
        assert not rtk_call_evidence(case, 'short', False, False)[3]
        (case/'rtk-argv.jsonl').write_text('')
        assert not rtk_call_evidence(case, 'short', True, False)[3]
        (case/'rtk-argv.jsonl').write_text(json.dumps(['hook', 'claude'])+'\n'+json.dumps(['cargo', 'test', '--test', 'short'])+'\n')
        assert not rtk_call_evidence(case, 'short', True, False)[3]
        skill = case/'skill'
        skill.mkdir()
        (skill/'SKILL.md').write_text('---\nname: caveman\n---\n\n# caveman\n')
        loaded = [{'type': 'user', 'isSynthetic': True,
                   'message': {'content': [{'type': 'text', 'text': f'Base directory for this skill: {skill}\n\n# caveman\n'}]}}]
        assert loaded_skill_source(loaded, skill)
        loaded[0]['message']['content'][0]['text'] += 'changed'
        assert not loaded_skill_source(loaded, skill)
    command = 'cargo test --test payment'
    tool_id = 'test-tool'
    pre = {'request': {'hook_event_name': 'PreToolUse', 'tool_use_id': tool_id,
                       'tool_input': {'command': command}},
           'reply': json.dumps({'hookSpecificOutput': {'updatedInput': {'command': f'rtk {command}'}}})}
    post = {'request': {'hook_event_name': 'PostToolUseFailure', 'tool_use_id': tool_id,
                        'tool_input': {'command': f'rtk {command}'}}, 'reply': ''}
    assert hook_evidence([pre, post], command, tool_id)
    assert hook_observed([pre, post], command, tool_id, True)
    assert not hook_evidence([pre, post], command, 'wrong-tool')
    assert not hook_evidence([pre, post, post], command, tool_id)
    assert not hook_observed([pre, post, post], command, tool_id, True)
    wrong = json.loads(json.dumps(pre))
    wrong['reply'] = json.dumps({'hookSpecificOutput': {'updatedInput': {'command': f'rtk rtk {command}'}}})
    assert not hook_evidence([wrong, post], command, tool_id)
    no_op_pre = {'request': {'hook_event_name': 'PreToolUse', 'tool_use_id': tool_id,
                             'tool_input': {'command': 'fixture-report short', 'timeout': 1000}},
                 'reply': '', 'exit': 0, 'stderr_sha256': sha256(b'')}
    no_op_post = {'request': {'hook_event_name': 'PostToolUseFailure', 'tool_use_id': tool_id,
                              'tool_input': {'command': 'fixture-report short', 'timeout': 1000}}, 'reply': ''}
    assert hook_noop_evidence([no_op_pre, no_op_post], 'fixture-report short', tool_id)
    assert not hook_noop_evidence([no_op_pre, post], 'fixture-report short', tool_id)
    assert not hook_noop_evidence([no_op_pre, no_op_post, no_op_post], 'fixture-report short', tool_id)
    bad_no_op = json.loads(json.dumps(no_op_pre))
    bad_no_op['reply'] = '{}'
    assert not hook_noop_evidence([bad_no_op, no_op_post], 'fixture-report short', tool_id)
    bad_no_op['reply'] = ''
    bad_no_op['exit'] = 7
    assert not hook_noop_evidence([bad_no_op, no_op_post], 'fixture-report short', tool_id)
    bad_no_op['exit'] = 0
    bad_no_op['request']['tool_input']['timeout'] = 2000
    assert not hook_noop_evidence([bad_no_op, no_op_post], 'fixture-report short', tool_id)
    assert completion_ok(0, {'subtype': 'success', 'permission_denials': []})
    assert not completion_ok(0, {'subtype': 'success', 'permission_denials': ['Bash denied']})
    assert not completion_ok(1, {'subtype': 'error', 'permission_denials': []})
    observed_calls = [{'input': {'command': command}}, {'input': {'command': f'rtk {command}'}},
                      {'input': {'command': 'rtk recall deadbeef'}}]
    assert task_call_counts(observed_calls, command) == (2, 1, 1)
    native_hook = {'hooks': {'PreToolUse': [{'matcher': 'Bash', 'hooks': [{'type': 'command', 'command': 'sentinel'}]}]}}
    assert native_rtk_handler(native_hook)['command'] == 'sentinel'
    native_hook['hooks']['PreToolUse'][0]['hooks'].append({'type': 'command', 'command': 'duplicate'})
    try:
        native_rtk_handler(native_hook)
    except ValueError:
        pass
    else:
        raise AssertionError('duplicate generated native handler was accepted')

def tool_results(events):
    return {block.get('tool_use_id'): block
            for event in events if event.get('type') == 'user'
            for block in event.get('message', {}).get('content', []) if isinstance(block, dict)
            and block.get('type') == 'tool_result'}

def hook_evidence(hook_events, command, tool_use_id):
    pre = [event for event in hook_events if event['request'].get('hook_event_name') == 'PreToolUse']
    post = [event for event in hook_events if event['request'].get('hook_event_name') in ('PostToolUse', 'PostToolUseFailure')]
    rewritten = []
    for event in pre:
        try:
            reply = json.loads(event['reply'])
        except ValueError:
            continue
        changed = reply.get('hookSpecificOutput', {}).get('updatedInput', {}).get('command')
        if event['request'].get('tool_input', {}).get('command') == command and changed == f'rtk {command}':
            rewritten.append(event)
    failures = [event for event in post if event['request'].get('hook_event_name') == 'PostToolUseFailure'
                and event['request'].get('tool_input', {}).get('command') == f'rtk {command}']
    return (len(rewritten) == 1 and len(failures) == 1 and len(post) == 1
            and rewritten[0]['request'].get('tool_use_id') == tool_use_id
            and failures[0]['request'].get('tool_use_id') == tool_use_id)

def hook_noop_evidence(hook_events, command, tool_use_id):
    if len(hook_events) != 2:
        return False
    pre, post = hook_events
    return (pre['request'].get('hook_event_name') == 'PreToolUse' and pre['reply'] == ''
            and pre.get('exit') == 0 and pre.get('stderr_sha256') == sha256(b'')
            and pre['request'].get('tool_input', {}).get('command') == command
            and post['request'].get('hook_event_name') == 'PostToolUseFailure' and post['reply'] == ''
            and post['request'].get('tool_input', {}).get('command') == command
            and pre['request'].get('tool_input') == post['request'].get('tool_input')
            and pre['request'].get('tool_use_id') == post['request'].get('tool_use_id') == tool_use_id)

def hook_observed(hook_events, command, tool_use_id, rewritten):
    if len(hook_events) != 2:
        return False
    pre, post = (event['request'] for event in hook_events)
    return (pre.get('hook_event_name') == 'PreToolUse'
            and post.get('hook_event_name') == 'PostToolUseFailure'
            and pre.get('tool_use_id') == post.get('tool_use_id') == tool_use_id
            and pre.get('tool_input', {}).get('command') == command
            and post.get('tool_input', {}).get('command') == (f'rtk {command}' if rewritten else command))

def prepare_native_setup(case, args, source, arm, bindir):
    builtins = ARM_BUILTINS[arm]
    yaml_bytes = ('version: 1\ntargets: [claude]\nbuiltins: [' + ', '.join(builtins) + ']\n').encode()
    (case/'agnostic-ai.yaml').write_bytes(yaml_bytes)
    (case/'.agnostic-ai').mkdir()
    agnostic_home = case.parent/('agnostic-home-'+case.name)
    agnostic_home.mkdir(mode=0o700)
    env = dict(os.environ, AGNOSTIC_AI_HOME=str(agnostic_home),
               PATH=str(bindir)+os.pathsep+os.environ['PATH'])
    sync_records = []
    for label, command in [('sync', [args.agnostic, 'sync', '--gitignore=off']),
                           ('check', [args.agnostic, 'sync', '--gitignore=off', '--check'])]:
        started = time.monotonic()
        result = subprocess.run(command, cwd=case, env=env, stdin=subprocess.DEVNULL,
                                capture_output=True, timeout=60)
        record = {'exit': result.returncode, 'elapsed_seconds': time.monotonic()-started,
                  'stdout_sha256': sha256(result.stdout), 'stderr_sha256': sha256(result.stderr)}
        (case/f'{label}-stdout.txt').write_bytes(result.stdout)
        (case/f'{label}-stderr.txt').write_bytes(result.stderr)
        sync_records.append(record)
        if result.returncode:
            raise RuntimeError(f'{case}: agnostic-ai {label} failed with exit {result.returncode}; see {label}-stderr.txt')
    if (case/'rtk-argv.jsonl').exists():
        raise RuntimeError(f'{case}: sync/check executed RTK')
    native = case/'.claude'
    settings_path = native/'settings.json'
    generated_settings_bytes = settings_path.read_bytes() if settings_path.exists() else b''
    generated = file_manifest(case)
    generated = {path: value for path, value in generated.items()
                 if path not in {'agnostic-ai.yaml', 'transcript.txt'} and not path.startswith('bin/')
                 and not path.startswith(('sync-', 'check-'))}
    if generated_settings_bytes:
        (case/'generated-settings.json').write_bytes(generated_settings_bytes)
    emitted = {path: value for path, value in generated.items()
               if path.startswith('.claude/') or path in {'.agnostic-ai/AGNOSTIC_AI.md', 'CLAUDE.md'}}
    expected_context = {'.agnostic-ai/AGNOSTIC_AI.md', 'CLAUDE.md'}
    allowed = expected_context | {'.claude/settings.json'}
    if 'caveman' in builtins:
        allowed |= {'.claude/skills/caveman/'+path for path in file_manifest(source)}
    allowed_project_files = allowed | {'.gitignore', '.agnostic-ai/.command-lock', '.agnostic-ai/.sync-state'}
    unexpected = set(generated) - allowed_project_files
    missing = expected_context - set(emitted)
    if unexpected or missing:
        raise ValueError(f'{case}: unexpected or missing generated context: {sorted(unexpected | missing)}')
    skill_directory = native/'skills/caveman'
    if 'caveman' in builtins:
        if not (skill_directory/'SKILL.md').is_file():
            raise ValueError(f'{case}: generated Caveman skill missing')
        if generated_skill_body(skill_directory/'SKILL.md') != skill_body(source/'SKILL.md'):
            raise ValueError(f'{case}: generated Caveman skill body differs from pin')
        if skill_assets(skill_directory) != skill_assets(source):
            raise ValueError(f'{case}: generated Caveman skill assets differ from pin')
    elif skill_directory.exists():
        raise ValueError(f'{case}: unexpected Caveman skill')
    settings = json.loads(generated_settings_bytes) if generated_settings_bytes else {}
    original_handler = None
    native_command = None
    if 'rtk' in builtins:
        original_handler = dict(native_rtk_handler(settings))
        native_command = original_handler['command']
        (case/'generated-native-handler.json').write_text(json.dumps(original_handler, sort_keys=True))
    elif settings.get('hooks'):
        raise ValueError(f'{case}: unexpected generated hooks')
    return settings, {'builtins': list(builtins), 'yaml_utf8_bytes': len(yaml_bytes),
                      'yaml_sha256': sha256(yaml_bytes), 'yaml_text': yaml_bytes.decode(),
                      'agnostic_home': str(agnostic_home), 'sync': sync_records[0], 'check': sync_records[1],
                      'generated_outputs': emitted, 'generated_project_files': generated,
                      'generated_settings_sha256': sha256(generated_settings_bytes) if generated_settings_bytes else None,
                      'generated_settings_utf8_bytes': len(generated_settings_bytes),
                      'original_native_handler': original_handler,
                      'original_native_handler_sha256': sha256(json.dumps(original_handler, sort_keys=True).encode()) if original_handler else None,
                      'original_native_command': native_command,
                      'generated_skill_tree_sha256': digest_tree(skill_directory) if 'caveman' in builtins else None,
                      'generated_skill_body_sha256': sha256(generated_skill_body(skill_directory/'SKILL.md').encode()) if 'caveman' in builtins else None,
                      'generated_skill_assets_verified': 'caveman' in builtins}

def probe_native_rtk_noop(root, args, source):
    case = root/'noop-preflight'
    case.mkdir(mode=0o700)
    bindir = case/'bin'
    bindir.mkdir()
    make_rtk_shim(bindir, args.rtk)
    _, setup = prepare_native_setup(case, args, source, 'rtk', bindir)
    profile = case/'rtk-profile'
    profile.mkdir()
    tee = case/'rtk-tee'
    tee.mkdir()
    request = json.dumps({'hook_event_name': 'PreToolUse', 'tool_name': 'Bash',
                          'tool_input': {'command': task_command('short')},
                          'tool_use_id': 'noop-preflight', 'session_id': 'noop-preflight',
                          'cwd': str(case)}).encode()
    env = dict(os.environ, PATH=str(bindir)+os.pathsep+os.environ['PATH'], CLAUDE_CONFIG_DIR=str(profile),
               RTK_DB_PATH=str(case/'rtk-usage.db'), RTK_RECALL_DB=str(case/'rtk-recall.db'),
               RTK_TEE_DIR=str(tee))
    result = subprocess.run(['/bin/sh', '-c', setup['original_native_command']], input=request,
                            env=env, cwd=case, capture_output=True, timeout=15)
    calls, processor_count, host_count, exact_calls = rtk_call_evidence(case, 'short', True, False)
    evidence = {'native_command': setup['original_native_command'], 'request_sha256': sha256(request),
                'exit': result.returncode, 'stdout_sha256': sha256(result.stdout),
                'stderr_sha256': sha256(result.stderr), 'rtk_calls': calls,
                'processor_count': processor_count, 'host_count': host_count, 'exact_calls': exact_calls,
                'generated_settings_sha256': setup['generated_settings_sha256']}
    (case/'probe-evidence.json').write_text(json.dumps(evidence, indent=2)+'\n')
    if result.returncode != 0 or result.stdout or result.stderr or not exact_calls:
        raise RuntimeError(f'{case}: selected RTK did not leave {task_command("short")} unchanged')
    return evidence

def run_case(case, args, source):
    case.mkdir(mode=0o700)
    task_name = case.name.split('-')[0]
    task = TASKS[task_name]
    arm = case.name.split('-')[1]
    rtk_enabled = arm in ('rtk', 'both')
    expected_rewrite = rtk_enabled and task.get('rtk_rewrite', True)
    raw = transcript(task)
    (case / 'transcript.txt').write_bytes(raw)
    bindir = case / 'bin'
    bindir.mkdir()
    make_fake_program(case, bindir, task_name)
    host_rtk = make_rtk_shim(bindir, args.rtk)
    settings_data, setup = prepare_native_setup(case, args, source, arm, bindir)
    native = case / '.claude'
    native.mkdir(exist_ok=True)
    hook = case / 'observe.py'
    hook.write_text('''import json, os, subprocess, sys
from pathlib import Path
root=Path(__file__).resolve().parent
raw=sys.stdin.buffer.read()
p=json.loads(raw)
out=b''
err=b''
code=0
if p.get('hook_event_name') == 'PreToolUse':
 with (root/'hook-requests.bin').open('ab') as request_log:
  request_log.write(len(raw).to_bytes(8,'big')+raw)
 command=json.loads((root/'native-hook-command.json').read_text())
 if command is not None:
  env=dict(os.environ, CLAUDE_CONFIG_DIR=str(root/'rtk-profile'))
  r=subprocess.run(['/bin/sh','-c',command],input=raw,capture_output=True,env=env)
  code=r.returncode
  out=r.stdout
  err=r.stderr
with (root/'hook-events.jsonl').open('a') as f:
 f.write(json.dumps({'request':p,'request_sha256':__import__('hashlib').sha256(raw).hexdigest(),
                     'reply':out.decode(errors='replace'),'stderr_sha256':__import__('hashlib').sha256(err).hexdigest(),
                     'exit':code})+'\\n')
sys.stdout.buffer.write(out)
sys.stderr.buffer.write(err)
sys.exit(code)
''')
    (case/'native-hook-command.json').write_text(json.dumps(setup['original_native_command']))
    settings = native / 'settings.json'
    observer_command = shlex.join([shutil.which('python3'), str(hook)])
    handler = [{'matcher': 'Bash', 'hooks': [{'type': 'command', 'command': observer_command}]}]
    if rtk_enabled:
        native_rtk_handler(settings_data)['command'] = observer_command
    else:
        settings_data.setdefault('hooks', {})['PreToolUse'] = handler
    for event_name in ('PostToolUse', 'PostToolUseFailure'):
        settings_data['hooks'][event_name] = handler
    if any(key in settings_data for key in ('enabledPlugins', 'disableBundledSkills', 'skillOverrides')):
        raise ValueError(f'{case}: generated settings already own host isolation fields')
    overrides = {'caveman': 'off'} if arm not in ('caveman', 'both') else {}
    for folder in (Path.home()/'.claude/skills', Path.home()/'.agents/skills'):
        if folder.is_dir():
            overrides.update({child.name: 'off' for child in folder.iterdir() if child.is_dir() and child.name != 'caveman'})
    user_settings = Path.home()/'.claude/settings.json'
    plugin_flags = json.loads(user_settings.read_text()).get('enabledPlugins', {}) if user_settings.exists() else {}
    settings_data.update({'enabledPlugins': {name: False for name in plugin_flags},
                          'disableBundledSkills': True, 'skillOverrides': overrides})
    settings.write_text(json.dumps(settings_data, sort_keys=True))
    command = task_command(task_name)
    prompt = (f'Run exactly `{command}` once. Do not edit files. Report counts in the form N passed; M failed. '
              'For each failing test, write exactly one plain-text line: TEST_IDENTIFIER | ERROR_PATH | expected N, got M. '
              'Put no other test identifier, error path, or comparison on that line. State that the command failed. Keep the answer concise.')
    if arm in ('caveman', 'both'):
        prompt = 'First invoke /caveman using the Skill tool. Then perform this task: ' + prompt
    cli = [args.claude, '-p', '--model', args.model, '--max-budget-usd', str(args.per_run_budget),
           '--max-turns', '6', '--setting-sources', 'project', '--strict-mcp-config', '--mcp-config', '{"mcpServers":{}}',
           '--settings', str(settings), '--no-session-persistence', '--permission-mode', 'manual', '--permission-prompts', 'none',
           '--system-prompt', 'Perform the requested local diagnostic task. Preserve identifiers, paths, numbers, and error details.',
           '--tools', 'Bash,Skill',
           '--allowedTools', f'Bash({command})', f'Bash(rtk {command})', 'Skill(caveman)',
           '--output-format', 'stream-json', '--verbose', '--include-hook-events']
    cli += ['--', prompt]
    metadata = {'case_id': case.name, 'repetition': int(case.name.split('-')[2]),
                'arm': arm, 'task': task_name, 'command': command,
                'fixture_sha256': hashlib.sha256(raw).hexdigest(), 'settings_sha256': hashlib.sha256(settings.read_bytes()).hexdigest(), 'argv': cli,
                'rtk_processor_executable': args.rtk, 'rtk_host_executable': str(host_rtk),
                'rtk_host_target': args.rtk, 'rtk_binary_sha256': args.rtk_sha256,
                'rtk_host_shim_sha256': hashlib.sha256(host_rtk.read_bytes()).hexdigest(),
                'prompt_utf8_bytes': len(prompt.encode('utf-8')),
                'skill_body_utf8_bytes': len(skill_body(native/'skills/caveman/SKILL.md').encode('utf-8')) if arm in ('caveman', 'both') else 0,
                'rtk_enabled': rtk_enabled, 'rewrite_expected': expected_rewrite,
                'fixture_utf8_bytes': len(raw),
                'scope': 'visible-summary-only; accepted protocol requires one task command and no recovery calls',
                'recovery_reads_definition': 'requested rtk recall Bash calls; extra tool calls fail the strict sequence'} | setup
    if not args.live:
        return metadata | {'prepared': True}
    profile = case / 'claude-profile'
    profile.mkdir()
    (case/'rtk-profile').mkdir()
    tee = case / 'rtk-tee'
    tee.mkdir()
    env = dict(os.environ, PATH=str(bindir)+os.pathsep+os.environ['PATH'], TRIO_RTK=args.rtk,
               RTK_DB_PATH=str(case/'rtk-usage.db'), RTK_RECALL_DB=str(case/'rtk-recall.db'),
               RTK_TEE_DIR=str(tee), CLAUDE_CONFIG_DIR=str(profile))
    started = time.monotonic()
    proc = subprocess.Popen(cli, cwd=case, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            stdin=subprocess.DEVNULL, start_new_session=True)
    try:
        stdout, stderr = proc.communicate(timeout=180)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(proc.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        stdout, stderr = proc.communicate()
        (case/'stdout.jsonl').write_bytes(stdout)
        (case/'stderr.txt').write_bytes(stderr)
        return metadata | {'timeout': True, 'correct': False, 'elapsed_seconds': time.monotonic()-started,
                           'execution_count': len((case/'fixture-runs.txt').read_text().splitlines()) if (case/'fixture-runs.txt').exists() else 0,
                           'retry_calls': None, 'recovery_reads': None,
                           'usage': None, 'estimated_cost_usd': None, 'token_accounting': accounting(None, None)}
    (case/'stdout.jsonl').write_bytes(stdout)
    (case/'stderr.txt').write_bytes(stderr)
    events = []
    for line in stdout.splitlines():
        try:
            events.append(json.loads(line))
        except ValueError:
            pass
    results = [event for event in events if event.get('type') == 'result']
    result = results[-1] if results else {}
    uses = [block for event in events if event.get('type') == 'assistant'
            for block in event.get('message', {}).get('content', []) if block.get('type') == 'tool_use']
    results_by_id = tool_results(events)
    expected_uses = ['Skill', 'Bash'] if arm in ('caveman', 'both') else ['Bash']
    tool_sequence_ok = [block.get('name') for block in uses] == expected_uses
    bash_uses = [block for block in uses if block.get('name') == 'Bash']
    test_command_calls, retry_calls, recovery_reads = task_call_counts(bash_uses, command)
    bash_request_ok = len(bash_uses) == 1 and bash_uses[0].get('input', {}).get('command') == command
    bash_result = results_by_id.get(bash_uses[0].get('id'), {}) if bash_uses else {}
    bash_content = str(bash_result.get('content', ''))
    bash_failed_as_expected = bash_result.get('is_error') is True and bash_content.startswith('Exit code 7')
    original_short_output = (bash_content == 'Exit code 7\n'+raw.decode().rstrip('\n')) if task_name == 'short' else None
    execution_count, fixture_args, exact_execution = execution_evidence(case, task_name)
    rtk_calls, processor_rtk_count, host_rtk_count, rtk_calls_exact = rtk_call_evidence(
        case, task_name, rtk_enabled, expected_rewrite)
    skill_ids = {block['id'] for block in uses if block.get('name') == 'Skill' and block.get('input', {}).get('skill') == 'caveman'}
    skill_used = any(block.get('type') == 'tool_result' and block.get('tool_use_id') in skill_ids and not block.get('is_error', False)
                     for event in events if event.get('type') == 'user'
                     for block in event.get('message', {}).get('content', []) if isinstance(block, dict))
    skill_source_verified = loaded_skill_source(events, native/'skills/caveman') if arm in ('caveman', 'both') else True
    hook_events = [json.loads(line) for line in (case/'hook-events.jsonl').read_text().splitlines()] if (case/'hook-events.jsonl').exists() else []
    tool_use_id = bash_uses[0].get('id') if bash_uses else None
    rtk_applied = hook_evidence(hook_events, command, tool_use_id)
    hook_recorded = hook_observed(hook_events, command, tool_use_id, expected_rewrite)
    no_unexpected_rewrite = hook_noop_evidence(hook_events, command, tool_use_id) if not expected_rewrite else rtk_applied
    answer = result.get('result', '')
    quality = oracle(answer, task)
    init = next((event for event in events if event.get('type') == 'system' and event.get('subtype') == 'init'), {})
    inventory = {key: init.get(key) for key in ('claude_code_version', 'model', 'skills', 'plugins', 'mcp_servers', 'tools')}
    expected_skill = arm in ('caveman', 'both')
    inventory_ok, host_profile = host_inventory_evidence(init, expected_skill)
    denials = result.get('permission_denials') or []
    complete = completion_ok(proc.returncode, result)
    token_accounting = accounting(result.get('usage'), result.get('total_cost_usd'))
    correct = all((quality['passed'], tool_sequence_ok, bash_request_ok, bash_failed_as_expected,
                   exact_execution, rtk_calls_exact, original_short_output is not False,
                   skill_used == expected_skill, skill_source_verified,
                   rtk_applied == expected_rewrite, hook_recorded,
                   no_unexpected_rewrite, inventory_ok, complete))
    return metadata | {'exit': proc.returncode, 'elapsed_seconds': time.monotonic()-started,
                       'correct_task': quality['passed'],
                       'setup_matches_arm': inventory_ok and (skill_used == expected_skill) and skill_source_verified and hook_recorded and (rtk_applied == expected_rewrite) and rtk_calls_exact,
                       'correct': correct, 'tool_sequence_ok': tool_sequence_ok, 'bash_request_ok': bash_request_ok,
                       'bash_failed_as_expected': bash_failed_as_expected, 'hook_recorded': hook_recorded,
                       'bash_hook_lifecycle': [(event.get('subtype'), event.get('hook_name')) for event in events
                                               if event.get('type') == 'system' and str(event.get('subtype', '')).startswith('hook_')],
                       'execution_count': execution_count,
                       'fixture_args': fixture_args, 'exact_execution': exact_execution, 'effective_inventory': inventory,
                       'host_profile': host_profile, 'host_profile_sha256': sha256(json.dumps(host_profile, sort_keys=True).encode()),
                       'host_inventory_ok': inventory_ok,
                       'original_short_output': original_short_output,
                       'test_command_calls': test_command_calls, 'retry_calls': retry_calls,
                       'recovery_reads': recovery_reads, 'recovery_in_accepted_protocol': False,
                       'rtk_calls': rtk_calls, 'processor_rtk_count': processor_rtk_count,
                       'host_rtk_count': host_rtk_count, 'rtk_calls_exact': rtk_calls_exact,
                       'skill_source_verified': skill_source_verified,
                       'oracle': quality, 'skill_used': skill_used, 'rtk_applied': rtk_applied, 'hook_events': hook_events, 'answer': answer,
                       'tool_calls': uses, 'usage': result.get('usage'), 'modelUsage': result.get('modelUsage'),
                       'token_accounting': token_accounting,
                       'estimated_cost_usd': token_accounting['cli_list_price_estimate_usd'], 'cost_label': 'CLI list-price estimate, not a bill',
                       'permission_denials': denials, 'result_subtype': result.get('subtype')}

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path)
    parser.add_argument('--skill', type=Path, help='Pinned Caveman skill directory, including notices')
    parser.add_argument('--agnostic', help='Pinned agnostic-ai CLI binary with tool builtins support')
    parser.add_argument('--agnostic-build-source', required=False, help='Source label for the selected CLI build')
    parser.add_argument('--agnostic-build-commit', required=False, help='Commit used to build the selected CLI')
    parser.add_argument('--self-test', action='store_true', help='Run local negative controls without a provider request')
    parser.add_argument('--claude', default=shutil.which('claude'))
    parser.add_argument('--rtk', default=shutil.which('rtk'))
    parser.add_argument('--model', default='claude-haiku-5-5')
    parser.add_argument('--repetitions', type=int, default=2)
    parser.add_argument('--tasks', nargs='+', action='append', choices=tuple(TASKS),
                        help='Task names to run; default is all three')
    parser.add_argument('--seed', type=int, default=1957)
    parser.add_argument('--live', action='store_true', help='Permit provider requests')
    parser.add_argument('--per-run-budget', type=float, default=0.12)
    parser.add_argument('--total-budget', type=float, default=2.0)
    args = parser.parse_args()
    if args.self_test:
        self_test()
        print('local negative controls passed')
        return
    if not args.output or not args.skill or not args.agnostic or not args.agnostic_build_source or not args.agnostic_build_commit:
        parser.error('--output, --skill, --agnostic, --agnostic-build-source, and --agnostic-build-commit are required except for --self-test')
    if (args.repetitions < 1 or not math.isfinite(args.per_run_budget) or not math.isfinite(args.total_budget)
            or args.per_run_budget <= 0 or args.total_budget <= 0):
        parser.error('Repetitions and budgets must be positive')
    try:
        args.selected_tasks = selected_tasks(args.tasks)
    except ValueError as error:
        parser.error(str(error))
    if not args.claude or not args.rtk or not (args.skill / 'SKILL.md').is_file():
        parser.error('Claude, RTK, and a pinned Caveman skill are required')
    try:
        args.claude = resolve_executable(args.claude)
        args.rtk = resolve_executable(args.rtk)
        args.agnostic = resolve_executable(args.agnostic)
    except ValueError as error:
        parser.error(str(error))
    args.claude_sha256 = hashlib.sha256(Path(args.claude).read_bytes()).hexdigest()
    args.rtk_sha256 = hashlib.sha256(Path(args.rtk).read_bytes()).hexdigest()
    args.agnostic_sha256 = hashlib.sha256(Path(args.agnostic).read_bytes()).hexdigest()
    args.output = args.output.resolve()
    args.output.mkdir(parents=True, exist_ok=False, mode=0o700)
    source = args.skill.resolve()
    repo = Path(__file__).resolve().parents[3]
    baseline = subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=repo, capture_output=True, text=True, check=True).stdout.strip()
    manifest = {'baseline_commit': baseline, 'runner_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                'skill_source': str(source), 'skill_tree_sha256': digest_tree(source),
                'agnostic_executable': args.agnostic, 'agnostic_binary_sha256': args.agnostic_sha256,
                'agnostic_version': version([args.agnostic, '--version']),
                'agnostic_build_source': args.agnostic_build_source,
                'agnostic_build_commit': args.agnostic_build_commit,
                'claude_executable': args.claude, 'claude_binary_sha256': args.claude_sha256,
                'claude_version': version([args.claude, '--version']), 'rtk_version': version([args.rtk, '--version']),
                'rtk_processor_executable': args.rtk, 'rtk_host_executable': '{case}/bin/rtk',
                'rtk_host_target': args.rtk, 'rtk_binary_sha256': args.rtk_sha256,
                'model': args.model, 'seed': args.seed, 'repetitions': args.repetitions,
                'selected_tasks': args.selected_tasks,
                'scope': 'visible-summary-only; accepted protocol requires one task command and no recovery calls',
                'cost_measure': 'CLI list-price estimate; input, cache creation, cache read, and output remain separate',
                'overhead_measure': 'Prompt and loaded skill body are UTF-8 byte counts, not provider tokens',
                'raw_artifacts': 'private; stdout and hook events may contain session IDs, local paths, and full tool input',
                'authentication_method': 'inherited environment; credentials are not inspected or recorded',
                'isolation': 'fresh AGNOSTIC_AI_HOME and CLAUDE_CONFIG_DIR per case; HOME unchanged',
                'builtin_cache': 'normal os.UserCacheDir cache is shared with the host and its warmth is uncontrolled; sync time is setup telemetry only'}
    manifest['expected_host'] = {'tools': sorted(COMMON_NATIVE_TOOLS),
                                 'common_skills': sorted(COMMON_NATIVE_SKILLS),
                                 'bundled_plugin_count': EXPECTED_BUNDLED_PLUGIN_COUNT,
                                 'caveman_delta': 'generated project skill only in caveman and both arms'}
    manifest['rtk_store'] = 'RTK_DB_PATH, RTK_RECALL_DB, and RTK_TEE_DIR point inside each private case directory'
    manifest['dataset'] = 'new cases only; earlier debug runs are excluded'
    manifest['task_matrix'] = {name: {'command': task_command(name), 'fixture_sha256': sha256(transcript(task)),
                                      'fixture_utf8_bytes': len(transcript(task)),
                                      'rtk_rewrite': task.get('rtk_rewrite', True)}
                               for name, task in TASKS.items() if name in args.selected_tasks}
    if args.live:
        manifest['noop_preflight'] = probe_native_rtk_noop(args.output, args, source)
    (args.output / 'manifest.json').write_text(json.dumps(manifest, indent=2)+'\n')
    cases = [f'{task}-{arm}-{rep}' for rep in range(args.repetitions)
             for task in args.selected_tasks for arm in ARMS]
    random.Random(args.seed).shuffle(cases)
    rows = []
    spent = 0
    unknown_cost = False
    for name in cases:
        if args.live and spent + args.per_run_budget > args.total_budget:
            break
        row = run_case(args.output / name, args, source)
        rows.append(row)
        if args.live:
            comparison_valid = comparable_host_profiles(rows)
            for item in rows:
                item['comparison_valid'] = comparison_valid
            spent, stop_reasons, missing_cost = account_live_row(spent, row, comparison_valid)
            unknown_cost = unknown_cost or missing_cost
        (args.output / 'results.json').write_text(json.dumps(rows, indent=2)+'\n')
        if args.live:
            if stop_reasons:
                print('Stopped: '+ '; '.join(stop_reasons) + '; retained incomplete comparison.', flush=True)
                break
        print(json.dumps({'case': name, 'correct': row.get('correct'), 'cost': row.get('estimated_cost_usd')}), flush=True)
    print(json.dumps({'completed': len(rows), 'planned': len(cases),
                      'estimated_cost_usd': None if unknown_cost else spent,
                      'known_estimated_cost_usd': spent}))

if __name__ == '__main__':
    main()
