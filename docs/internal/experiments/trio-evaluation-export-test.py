#!/usr/bin/env python3
"""Focused local exporter controls, with synthetic data only."""

import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location('trio_export', Path(__file__).with_name('trio-evaluation-export.py'))
exporter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(exporter)


class ExportTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='trio-export-control-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.private = self.root/'private'
        self.private.mkdir()
        self.public = self.root/'public'
        self.cases = ['payment-both-0', 'short-concise-0']
        manifest = {'model': 'test-model', 'seed': 1957, 'repetitions': 1,
                    'skill_source': str(self.root/'selected-skill'), 'runner_sha256': 'a'*64,
                    'authentication_method': 'inherited environment; credentials are not inspected or recorded'}
        self.write('manifest.json', manifest)
        rows = []
        for case in self.cases:
            directory = self.private/case
            directory.mkdir()
            identifier = 'toolu_private_relation_12345'
            session = '11111111-2222-4333-8444-555555555555'
            fixture = 'tests/payment.rs:42 | expected 200, got 503\n'
            row = {'case_id': case, 'correct': case == self.cases[0], 'usage': {'input_tokens': 12, 'output_tokens': 3} if case == self.cases[0] else None,
                   'estimated_cost_usd': 0.01 if case == self.cases[0] else None,
                   'token_accounting': {'measurement_complete': case == self.cases[0]},
                   'tool_calls': [{'id': identifier, 'name': 'Bash', 'input': {'command': 'cargo test --test payment'}}],
                   'answer': fixture, 'session_id': session, 'fixture_sha256': hashlib.sha256(fixture.encode()).hexdigest()}
            rows.append(row)
            request = {'tool_use_id': identifier, 'session_id': session, 'cwd': str(directory), 'tool_input': {'command': 'cargo test --test payment'}}
            self.write(case+'/.claude/settings.json', {
                'hooks': {'PreToolUse': [{'matcher': 'Bash', 'hooks': [{'command': str(directory/'observe.py')}]}]},
                'skillOverrides': {'PrivateCompany-review': 'off', 'caveman': 'off'},
                'enabledPlugins': {'EmployerTools@private': False}})
            self.write(case+'/hook-events.jsonl', [{'request': request, 'reply': json.dumps({'hookSpecificOutput': {'updatedInput': {'command': 'rtk cargo test --test payment'}}})}], jsonl=True)
            raw = json.dumps(request).encode()
            (directory/'hook-requests.bin').write_bytes(len(raw).to_bytes(8, 'big')+raw)
            self.write(case+'/stdout.jsonl', [
                {'type': 'system', 'subtype': 'init', 'session_id': session, 'skills': ['caveman', 'doctor', 'PrivateCompany-review'],
                 'plugins': [{'name': 'public-builtin', 'source': 'public-builtin@builtin', 'path': 'bunfs/builtin/public-builtin'},
                             {'name': 'EmployerTools', 'source': 'EmployerTools@private', 'path': str(self.root/'private-plugin')}],
                 'mcp_servers': []},
                {'type': 'assistant', 'message': {'id': 'msg_private_relation_12345', 'content': row['tool_calls'], 'usage': row['usage']}},
                {'type': 'user', 'message': {'content': [{'type': 'tool_result', 'tool_use_id': identifier, 'content': 'Exit code 7\n'+fixture}]}},
                {'type': 'assistant', 'message': {'content': [{'type': 'text', 'text': 'PrivateCompany disabled; '+fixture}]}},
                {'type': 'result', 'session_id': session, 'usage': row['usage'], 'total_cost_usd': row['estimated_cost_usd'], 'result': fixture}], jsonl=True)
            (directory/'transcript.txt').write_text(fixture)
            (directory/'stderr.txt').write_text('')
            self.write(case+'/claude-profile/credentials.json', {'api_key': 'sk-ant-'+('z'*40)})
            (directory/'.env').write_text('PRIVATE_SECRET=never-read')
        self.write('results.json', rows)

    def write(self, relative, value, jsonl=False):
        target = self.private/relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(''.join(json.dumps(row)+'\n' for row in value) if jsonl else json.dumps(value))

    def test_preserves_raw_pairs_failures_unknown_usage_and_relations(self):
        reads = []
        original = Path.read_bytes
        def observed(path):
            reads.append(path)
            return original(path)
        with patch.object(Path, 'read_bytes', observed):
            result = exporter.export(self.private, self.public)
        self.assertEqual(result['rows'], 2)
        self.assertFalse(any('credentials' in path.name or path.name == '.env' for path in reads))
        rows = json.loads((self.public/'results.json').read_text())
        self.assertFalse(rows[1]['correct'])
        self.assertIsNone(rows[1]['usage'])
        self.assertIsNone(rows[1]['estimated_cost_usd'])
        self.assertFalse(rows[1]['token_accounting']['measurement_complete'])
        self.assertEqual(rows[0]['usage'], {'input_tokens': 12, 'output_tokens': 3})
        case = self.cases[0]
        events = [json.loads(line) for line in (self.public/case/'stdout.jsonl').read_text().splitlines()]
        hook = json.loads((self.public/case/'hook-events.jsonl').read_text().splitlines()[0])
        raw_hook = json.loads((self.public/case/'hook-requests.jsonl').read_text().splitlines()[0])
        identifier = rows[0]['tool_calls'][0]['id']
        self.assertEqual(identifier, events[1]['message']['content'][0]['id'])
        self.assertEqual(identifier, events[2]['message']['content'][0]['tool_use_id'])
        self.assertEqual(identifier, hook['request']['tool_use_id'])
        self.assertEqual(identifier, raw_hook['request']['tool_use_id'])
        self.assertEqual(rows[0]['session_id'], events[0]['session_id'])
        self.assertEqual(hook['request']['cwd'], '{case}')
        self.assertEqual(len(events[0]['skills']), 3)
        self.assertEqual(len(events[0]['plugins']), 2)
        self.assertIn('caveman', events[0]['skills'])
        self.assertEqual(events[0]['plugins'][0]['source'], 'public-builtin@builtin')
        combined = '\n'.join(path.read_text() for path in self.public.rglob('*') if path.is_file())
        for private in ['PrivateCompany', 'EmployerTools', str(self.private), 'toolu_private_relation', '11111111-2222-4333-8444-555555555555']:
            self.assertNotIn(private, combined)
        self.assertIn('tests/payment.rs:42 | expected 200, got 503', combined)
        settings = json.loads((self.public/case/'.claude/settings.json').read_text())
        self.assertEqual(len(settings['skillOverrides']), 2)
        self.assertEqual(len(settings['enabledPlugins']), 1)
        index = json.loads((self.public/'export-manifest.json').read_text())
        file = index['files'][case+'/stdout.jsonl']
        self.assertEqual(file['private_original_sha256'], hashlib.sha256((self.private/case/'stdout.jsonl').read_bytes()).hexdigest())
        self.assertEqual(file['sanitized_sha256'], hashlib.sha256((self.public/case/'stdout.jsonl').read_bytes()).hexdigest())
        second = self.root/'public-second'
        exporter.export(self.private, second)
        for path in self.public.rglob('*'):
            if path.is_file():
                self.assertEqual(path.read_bytes(), (second/path.relative_to(self.public)).read_bytes())

    def test_refuses_secrets_before_creating_output(self):
        for value in ['sk-ant-'+('z'*40), 'Bearer '+('q'*32), 'ANTHROPIC_API_KEY=private-value',
                      {'env': {'TOKEN': 'private-value'}}, {'api_key': 'private-value'}, {'token': 'private-value'}, {'auth': 'abc'},
                      '{"api_key": "private-value"}', '{"api_key": "\\u0070rivate-value"}']:
            with self.subTest(kind=type(value).__name__):
                self.write(self.cases[0]+'/stdout.jsonl', [{'type': 'assistant', 'message': {'content': value}}], jsonl=True)
                with self.assertRaises(ValueError):
                    exporter.export(self.private, self.public)
                self.assertFalse(self.public.exists())

    def test_refuses_overwrite_symlinks_and_legacy_debug_rows(self):
        self.public.mkdir()
        (self.public/'sentinel').write_text('keep')
        with self.assertRaises(ValueError):
            exporter.export(self.private, self.public)
        self.assertEqual((self.public/'sentinel').read_text(), 'keep')
        fresh = self.root/'fresh'
        selected = self.private/self.cases[0]/'stderr.txt'
        selected.unlink()
        selected.symlink_to(self.private/self.cases[0]/'claude-profile/credentials.json')
        with self.assertRaises(ValueError):
            exporter.export(self.private, fresh)
        self.assertFalse(fresh.exists())
        selected.unlink()
        selected.write_text('')
        rows = json.loads((self.private/'results.json').read_text())
        del rows[0]['case_id']
        self.write('results.json', rows)
        with self.assertRaises(ValueError):
            exporter.export(self.private, fresh)

    def test_rejects_truncated_frames_and_nested_output(self):
        frame = self.private/self.cases[0]/'hook-requests.bin'
        frame.write_bytes((100).to_bytes(8, 'big')+b'{}')
        with self.assertRaises(ValueError):
            exporter.export(self.private, self.public)
        self.assertFalse(self.public.exists())
        with self.assertRaises(ValueError):
            exporter.export(self.private, self.private/'nested-export')

    def test_preserves_distinct_private_inventory_keys(self):
        settings = {'skillOverrides': {'PrivateCompany-review': 'off', 'privatecompany-review': 'off', 'caveman': 'off'}}
        self.write(self.cases[0]+'/.claude/settings.json', settings)
        exporter.export(self.private, self.public)
        public = json.loads((self.public/self.cases[0]/'.claude/settings.json').read_text())
        self.assertEqual(len(public['skillOverrides']), 3)
        self.assertEqual(list(public['skillOverrides'].values()), ['off', 'off', 'off'])

    def test_private_inventory_words_do_not_change_diagnostic_facts(self):
        self.write(self.cases[0]+'/.claude/settings.json', {'skillOverrides': {'payment': 'off', 'Bash': 'off', 'command': 'off', 'timeout': 'off'}})
        exporter.export(self.private, self.public)
        rows = json.loads((self.public/'results.json').read_text())
        self.assertEqual(rows[0]['tool_calls'][0]['name'], 'Bash')
        self.assertEqual(rows[0]['tool_calls'][0]['input']['command'], 'cargo test --test payment')
        self.assertEqual(rows[0]['answer'], 'tests/payment.rs:42 | expected 200, got 503\n')
        settings = json.loads((self.public/self.cases[0]/'.claude/settings.json').read_text())
        self.assertNotIn('payment', settings['skillOverrides'])
        self.assertNotIn('Bash', settings['skillOverrides'])

    def test_failure_without_a_provider_stream_is_retained_and_labeled(self):
        failed = self.cases[1]
        (self.private/failed/'stdout.jsonl').unlink()
        exporter.export(self.private, self.public)
        rows = json.loads((self.public/'results.json').read_text())
        self.assertEqual(len(rows), 2)
        self.assertFalse(rows[1]['correct'])
        self.assertIsNone(rows[1]['usage'])
        metadata = json.loads((self.public/'export-manifest.json').read_text())
        self.assertFalse(metadata['redactions'][failed]['raw_stream_available'])

    def test_model_json_text_keeps_duplicate_facts_and_spacing(self):
        text = '{ "failed": 1, "failed": 2, "path": "tests/café.rs:17", "note": "PrivateCompany" }'
        self.write(self.cases[0]+'/stdout.jsonl', [{'type': 'assistant', 'message': {'content': [{'type': 'text', 'text': text}]}}], jsonl=True)
        exporter.export(self.private, self.public)
        event = json.loads((self.public/self.cases[0]/'stdout.jsonl').read_text())
        actual = event['message']['content'][0]['text']
        self.assertIn('"failed": 1, "failed": 2', actual)
        self.assertTrue(actual.startswith('{ "failed"'))
        self.assertIn('tests/café.rs:17', actual)
        self.assertNotIn('PrivateCompany', actual)

    def test_private_paths_with_spaces_are_redacted_in_json_and_unparsed_lines(self):
        case = self.cases[0]
        paths = ['/Users/example/Library/Application Support/PrivateClient/report.json',
                 '/home/example/Client Work/PrivateClient/report.json',
                 '/tmp/Client Work/PrivateClient/report.json',
                 '~/Library/Application Support/PrivateClient/report.json']
        diagnostic = 'tests/café.rs:17 | expected 12, got 18'
        events = [{'type': 'assistant', 'message': {'content': [
            {'type': 'text', 'text': json.dumps({'path': path, 'error': diagnostic}, ensure_ascii=False)}]}}
                  for path in paths]
        self.write(case+'/stdout.jsonl', events, jsonl=True)
        stream = self.private/case/'stdout.jsonl'
        with stream.open('a') as output:
            for path in paths:
                output.write('Cannot open '+path+'; '+diagnostic+'\n')
        original_hash = hashlib.sha256(stream.read_bytes()).hexdigest()
        exporter.export(self.private, self.public)
        public = self.public/case/'stdout.jsonl'
        exported = [json.loads(line) for line in public.read_text().splitlines()]
        for index, path in enumerate(paths):
            with self.subTest(path=path):
                text = json.loads(exported[index]['message']['content'][0]['text'])
                self.assertRegex(text['path'], r'^\{private-path-[0-9]+\}$')
                self.assertEqual(text['error'], diagnostic)
                self.assertEqual(exported[len(paths)+index]['text'], 'Cannot open '+text['path']+'; '+diagnostic)
        combined = '\n'.join(path.read_text() for path in self.public.rglob('*') if path.is_file())
        self.assertNotIn('PrivateClient', combined)
        self.assertNotIn('Application Support', combined)
        metadata = json.loads((self.public/'export-manifest.json').read_text())
        record = metadata['files'][case+'/stdout.jsonl']
        self.assertEqual(record['private_original_sha256'], original_hash)
        self.assertEqual(record['sanitized_sha256'], hashlib.sha256(public.read_bytes()).hexdigest())

    def test_duplicate_escaped_credential_keys_refuse_export_before_any_write(self):
        for field in ['password', 'api_key']:
            for padding in ['', '  ']:
                with self.subTest(field=field, padding=len(padding)):
                    destination = self.root/('public-'+field+'-'+str(len(padding)))
                    escaped = '\\u'+format(ord(field[0]), '04x')+field[1:]
                    text = padding+'{"'+escaped+'":"abc","'+field+'":""}'
                    self.write(self.cases[0]+'/stdout.jsonl', [
                        {'type': 'assistant', 'message': {'content': [{'type': 'text', 'text': text}]}}], jsonl=True)
                    with self.assertRaisesRegex(ValueError, 'Credential or environment field'):
                        exporter.export(self.private, destination)
                    self.assertFalse(destination.exists())

    def test_duplicate_credentials_in_raw_json_and_hook_frames_refuse_export(self):
        text = '{"\\u0070assword":"abc","password":""}'
        case = self.private/self.cases[0]
        original_stream = (case/'stdout.jsonl').read_bytes()
        for artifact in ['stdout.jsonl', 'probe-evidence.json', 'hook-requests.bin']:
            with self.subTest(artifact=artifact):
                target = case/artifact
                original = target.read_bytes() if target.exists() else None
                data = text.encode()
                target.write_bytes(len(data).to_bytes(8, 'big')+data if artifact.endswith('.bin') else data)
                destination = self.root/('public-'+artifact)
                with self.assertRaisesRegex(ValueError, 'Credential or environment field'):
                    exporter.export(self.private, destination)
                self.assertFalse(destination.exists())
                if original is None:
                    target.unlink()
                else:
                    target.write_bytes(original)
        self.assertEqual((case/'stdout.jsonl').read_bytes(), original_stream)

    def test_success_without_raw_stream_is_retained_with_incomplete_evidence(self):
        successful = self.cases[0]
        (self.private/successful/'stdout.jsonl').unlink()
        exporter.export(self.private, self.public)
        rows = json.loads((self.public/'results.json').read_text())
        self.assertTrue(rows[0]['correct'])
        metadata = json.loads((self.public/'export-manifest.json').read_text())
        self.assertTrue(metadata['redactions'][successful]['incomplete_evidence'])

    def test_generated_target_environment_is_preserved_in_known_settings_only(self):
        target = {'AGNOSTIC_AI_TARGET': 'claude'}
        settings = {'env': target, 'hooks': {'PreToolUse': [{'matcher': 'Bash', 'hooks': [
            {'type': 'command', 'command': 'command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude'}]}]}}
        case = self.cases[0]
        for relative in ['.claude/settings.json', 'generated-settings.json']:
            self.write(case+'/'+relative, settings)
        exporter.export(self.private, self.public)
        for relative in ['.claude/settings.json', 'generated-settings.json']:
            self.assertEqual(json.loads((self.public/case/relative).read_text()), settings)
        self.write(case+'/probe-evidence.json', {'env': target})
        destination = self.root/'rejected-public'
        with self.assertRaisesRegex(ValueError, 'Credential or environment field'):
            exporter.export(self.private, destination)
        self.assertFalse(destination.exists())

    def test_generated_settings_refuse_other_environment_auth_and_duplicate_poison(self):
        for index, settings in enumerate([
                {'env': {'AGNOSTIC_AI_TARGET': 'other'}},
                {'env': {'AGNOSTIC_AI_TARGET': 'claude', 'TOKEN': 'abc'}},
                {'env': {'AGNOSTIC_AI_TARGET': 'claude'}, 'auth': 'abc'},
                {'hooks': {'env': {'AGNOSTIC_AI_TARGET': 'claude'}}}]):
            with self.subTest(index=index):
                self.write(self.cases[0]+'/.claude/settings.json', settings)
                destination = self.root/('rejected-'+str(index))
                with self.assertRaisesRegex(ValueError, 'Credential or environment field'):
                    exporter.export(self.private, destination)
                self.assertFalse(destination.exists())
        for index, text in enumerate([
                '{"env":{"TOKEN":"abc"},"env":{"AGNOSTIC_AI_TARGET":"claude"}}',
                '{"env":{"AGNOSTIC_AI_TARGET":"other","AGNOSTIC_AI_TARGET":"claude"}}']):
            with self.subTest(duplicate=index):
                (self.private/self.cases[0]/'.claude/settings.json').write_text(text)
                destination = self.root/('rejected-duplicate-'+str(index))
                with self.assertRaisesRegex(ValueError, 'Credential or environment field'):
                    exporter.export(self.private, destination)
                self.assertFalse(destination.exists())


if __name__ == '__main__':
    unittest.main()
