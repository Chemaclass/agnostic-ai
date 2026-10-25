#!/usr/bin/env python3
import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location('recordings', Path(__file__).with_name('bench-recordings.py'))
bench = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(bench)


def fixture(role):
    result = {'schema': 1, 'method': bench.METHOD, 'session': 'paired', 'role': role,
            'environment': {'machine': 'test', 'os': 'test', 'architecture': 'test', 'go': 'go1.test'},
            'binary': {'sha256': 'a' * 64, 'revision': 'b' * 40,
                       'revision_source': 'operator', 'dirty': 'clean'},
            'fixture': {'specs': 500, 'identities': {'synced': 'c' * 64, 'no-ledger': 'd' * 64}},
            'warmups': 1, 'runs': 6, 'status': 'complete',
            'commands': [dict(command, warmups=[{'milliseconds': 1, 'actual_exit': command['expected_exit']}],
                              samples=[{'milliseconds': value, 'actual_exit': command['expected_exit']}
                                       for value in (1, 2, 3, 4, 5, 6)])
                         for command in bench.command_plan()]}
    for command in result['commands']:
        if command['argv'] == ['doctor', '--json']:
            for sample in command['samples'] + command['warmups']:
                sample['diagnostic'] = bench.expected_doctor_summary(command['mode'])
    return result


class RecordingTests(unittest.TestCase):
    def setUp(self):
        self.baseline = fixture('baseline')
        self.candidate = fixture('candidate')

    def test_median_range_and_deltas_use_all_samples(self):
        for command in self.candidate['commands']:
            for sample in command['samples']:
                sample['milliseconds'] *= 2
        report = bench.compare(self.baseline, self.candidate)
        self.assertIn('6 | 3.500 [1.000,6.000] | 7.000 [2.000,12.000] | +3.500 | +100.00%', report)
        self.assertEqual(6, len(self.candidate['commands'][0]['samples']))

    def test_known_doctor_failure_is_valid_but_unexpected_exit_is_rejected(self):
        bench.compare(self.baseline, self.candidate)
        doctor = next(command for command in self.candidate['commands'] if command['argv'] == ['doctor', '--json'])
        doctor['samples'][0]['actual_exit'] = 2
        with self.assertRaisesRegex(ValueError, 'unexpected command exit'):
            bench.compare(self.baseline, self.candidate)

    def test_missing_command_is_not_a_complete_recording(self):
        self.baseline['commands'].pop()
        self.candidate['commands'].pop()
        with self.assertRaisesRegex(ValueError, 'command list'):
            bench.compare(self.baseline, self.candidate)

    def test_incomparable_metadata_is_rejected(self):
        for field in ('schema', 'method', 'session', 'environment', 'fixture', 'warmups', 'runs'):
            with self.subTest(field=field):
                candidate = copy.deepcopy(self.candidate)
                if field == 'environment':
                    candidate[field]['machine'] = 'different'
                elif field == 'fixture':
                    candidate[field]['identities']['synced'] = 'e' * 64
                elif type(candidate[field]) is int:
                    candidate[field] += 1
                else:
                    candidate[field] += '-different'
                with self.assertRaises(ValueError):
                    bench.compare(self.baseline, candidate)

    def test_malformed_samples_and_missing_metadata_are_rejected(self):
        for value in (None, True, -1, 0, float('nan'), float('inf'), '1'):
            with self.subTest(value=value):
                candidate = copy.deepcopy(self.candidate)
                candidate['commands'][0]['samples'][0]['milliseconds'] = value
                with self.assertRaises(ValueError):
                    bench.compare(self.baseline, candidate)
        for key in self.candidate:
            candidate = copy.deepcopy(self.candidate)
            del candidate[key]
            with self.subTest(missing=key), self.assertRaises(ValueError):
                bench.compare(self.baseline, candidate)

    def test_comparison_process_fails_for_bad_json_and_missing_files(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'record.json'
            path.write_text('{')
            for other in (path, path.with_name('missing.json')):
                result = subprocess.run([sys.executable, str(SPEC.origin), '--compare', str(path), str(other)], capture_output=True)
                self.assertEqual(1, result.returncode)

    def test_setup_commands_accept_only_the_declared_exit_status(self):
        command = [sys.executable, '-c', 'raise SystemExit(1)']
        self.assertEqual(1, bench.execute(command, expected=1).returncode)
        with self.assertRaisesRegex(ValueError, 'expected exit 0, got 1'):
            bench.execute(command)

    def test_setup_failure_reports_the_tail_of_stderr(self):
        script = 'import sys\nfor n in range(30): print("line", n, file=sys.stderr)\nraise SystemExit(1)'
        with self.assertRaises(ValueError) as caught:
            bench.execute([sys.executable, '-c', script])
        message = str(caught.exception)
        self.assertIn('line 29', message)
        self.assertNotIn('line 1\n', message + '\n')
        self.assertLess(len(message), 2000)

    def test_timed_failure_reports_output_and_names_candidate_drift(self):
        result = subprocess.CompletedProcess([], 1, stdout=b'drift\n  - CLAUDE.md\n', stderr=b'cause here\n')
        plain = bench.timed_failure({'mode': 'synced', 'argv': ['status'], 'expected_exit': 0}, 'baseline', result)
        self.assertIn('expected 0, got 1', plain)
        self.assertIn('cause here', plain)
        check = {'mode': 'synced', 'argv': ['sync', '--check'], 'expected_exit': 0}
        self.assertNotIn('differs from the baseline', bench.timed_failure(check, 'baseline', result))
        message = bench.timed_failure(check, 'candidate', result)
        self.assertIn("candidate output differs from the baseline's", message)
        self.assertIn('CLAUDE.md', message)

    def test_isolated_environment_covers_windows_profile_locations(self):
        with tempfile.TemporaryDirectory() as directory:
            env = bench.isolated_environment(Path(directory))
            for name in ('USERPROFILE', 'LOCALAPPDATA', 'APPDATA', 'HOME'):
                self.assertTrue(env[name].startswith(directory), name)

    def test_comparison_rejects_valid_recordings_with_different_run_counts(self):
        candidate = fixture('candidate')
        for command in candidate['commands']:
            command['samples'].append(dict(command['samples'][0]))
        candidate['runs'] = 7
        bench.validate(self.baseline)
        bench.validate(candidate)
        with self.assertRaisesRegex(ValueError, 'incomparable runs'):
            bench.compare(self.baseline, candidate)

    def test_doctor_operational_failure_cannot_be_an_expected_sample(self):
        for output in (b'', b'config read failed', b'{"version":"1","command":"doctor","errors":[{"message":"read failed"}]}'):
            with self.subTest(output=output), self.assertRaises(ValueError):
                bench.doctor_diagnostic(output)

    def test_expected_doctor_report_is_validated_and_unknown_trust_is_rejected(self):
        report = {'version': '1', 'command': 'doctor', 'errors': [], 'writes': [], 'lint': [],
                  'hook_trust': [{'status': 'untrusted', 'event': 'PostToolUse', 'group': 0, 'handler': index, 'hook': f'export AGNOSTIC_AI_TARGET=codex; echo hook-{index:02d}'}
                                 for index in range(5)]}
        self.assertEqual(bench.expected_doctor_summary('synced'), bench.doctor_diagnostic(json.dumps(report)))
        report['hook_trust'][0]['status'] = 'unknown'
        report['hook_trust'][0]['problem'] = 'cannot read configuration'
        with self.assertRaisesRegex(ValueError, 'operational hook trust failure'):
            bench.doctor_diagnostic(json.dumps(report))

    def test_no_ledger_accepts_only_the_known_leftover_and_rejects_extra_drift(self):
        report = {'version': '1', 'command': 'doctor', 'errors': [],
                  'writes': [dict(bench.NO_LEDGER_WRITE)], 'lint': [],
                  'hook_trust': [{'status': 'untrusted', 'event': 'PostToolUse', 'group': 0,
                                  'handler': index, 'hook': f'echo hook-{index:02d}'}
                                 for index in range(5)]}
        self.assertEqual(bench.expected_doctor_summary('no-ledger'),
                         bench.doctor_diagnostic(json.dumps(report), 'no-ledger'))
        with self.assertRaisesRegex(ValueError, 'unexpected drift'):
            bench.doctor_diagnostic(json.dumps(report), 'synced')
        report['writes'].append(dict(bench.NO_LEDGER_WRITE, path='unexpected.json'))
        with self.assertRaisesRegex(ValueError, 'unexpected drift'):
            bench.doctor_diagnostic(json.dumps(report), 'no-ledger')

    def test_comparison_rejects_doctor_summary_from_the_other_scenario(self):
        doctor = next(command for command in self.candidate['commands']
                      if command['mode'] == 'no-ledger' and command['argv'] == ['doctor', '--json'])
        doctor['samples'][0]['diagnostic'] = bench.expected_doctor_summary('synced')
        with self.assertRaisesRegex(ValueError, 'unexpected doctor diagnostic'):
            bench.compare(self.baseline, self.candidate)

    def test_binary_changed_during_run_cannot_complete(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'binary'
            path.write_bytes(b'baseline')
            identity = {'sha256': bench.sha256(path)}
            path.write_bytes(b'modified')
            with self.assertRaisesRegex(ValueError, 'binary changed'):
                bench.verify_binaries([path], [{'binary': identity}])

    def test_cached_snapshot_uses_initial_bytes_and_restores_git_and_symlinks(self):
        import os
        with tempfile.TemporaryDirectory() as directory:
            source = Path(directory) / 'snapshot'
            active = Path(directory) / 'active'
            source.mkdir()
            (source / '.git').mkdir()
            (source / '.git/index').write_bytes(b'index-before')
            (source / 'body').write_bytes(b'initial')
            (source / 'link').symlink_to('body')
            snapshot = bench.Snapshot(source)
            self.assertEqual(snapshot.identity, bench.restore_tree(snapshot, active))
            (source / 'body').write_bytes(b'changed-source')
            (active / 'body').write_bytes(b'changed')
            (active / '.git/index').write_bytes(b'index-after')
            (active / 'link').unlink()
            (active / 'link').symlink_to('missing')
            self.assertEqual(snapshot.identity, bench.restore_tree(snapshot, active))
            self.assertEqual(b'initial', (active / 'body').read_bytes())
            self.assertEqual(b'index-before', (active / '.git/index').read_bytes())
            self.assertEqual('body', os.readlink(active / 'link'))
            self.assertEqual(snapshot.identity, bench.tree_identity(active))

    def test_restore_same_stat_mutation_and_directory_permissions(self):
        import os
        with tempfile.TemporaryDirectory() as directory:
            snapshot = Path(directory) / 'snapshot'
            active = Path(directory) / 'active'
            snapshot.mkdir()
            folder = snapshot / 'folder'
            folder.mkdir(mode=0o755)
            (folder / 'body').write_text('original')
            bench.restore_tree(snapshot, active)
            path = active / 'folder/body'
            (active / 'folder').chmod(0o700)
            self.assertNotEqual(bench.tree_identity(snapshot), bench.tree_identity(active))
            before = path.stat()
            path.write_text('mutation')
            os.utime(path, ns=(before.st_atime_ns, before.st_mtime_ns))
            (active / 'folder').chmod(0o700)
            self.assertNotEqual(bench.tree_identity(snapshot), bench.tree_identity(active))
            bench.restore_tree(snapshot, active)
            self.assertEqual('original', path.read_text())
            self.assertEqual(before.st_mtime_ns, path.stat().st_mtime_ns)
            self.assertEqual(0o755, (active / 'folder').stat().st_mode & 0o777)
            self.assertEqual(bench.tree_identity(snapshot), bench.tree_identity(active))

    def test_restore_removes_added_files_and_restores_changed_and_deleted_files(self):
        with tempfile.TemporaryDirectory() as directory:
            snapshot = Path(directory) / 'snapshot'
            active = Path(directory) / 'active'
            snapshot.mkdir()
            (snapshot / 'body').write_text('original')
            (snapshot / 'deleted').write_text('keep')
            bench.restore_tree(snapshot, active)
            (active / 'body').write_text('changed')
            (active / 'deleted').unlink()
            (active / 'extra').mkdir()
            (active / 'extra/file').write_text('remove')
            bench.restore_tree(snapshot, active)
            self.assertEqual(bench.tree_identity(snapshot), bench.tree_identity(active))


if __name__ == '__main__':
    unittest.main()
