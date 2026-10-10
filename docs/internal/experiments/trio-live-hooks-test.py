import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

RUNNER = Path(__file__).with_name('trio-live-hooks.py')


class LiveHooksRunnerTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="trio fixture's ")
        self.root = Path(self.directory.name)
        self.addCleanup(self.directory.cleanup)
        self.claude = self.root / "fake claude's CLI"
        self.rtk = self.root / "fake RTK's CLI"
        self.env = dict(os.environ, TRIO_CLAUDE_BIN=str(self.claude),
                        TRIO_RTK_BIN=str(self.rtk),
                        TRIO_LIVE_ROOT=str(self.root / 'runs'))
        for key in ('CLAUDE_CODE_OAUTH_TOKEN', 'ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN'):
            self.env.pop(key, None)
        self.script(self.rtk, "print('{}')")

    def script(self, path, body):
        path.write_text('#!' + sys.executable + '\n' + body + '\n')
        path.chmod(0o755)

    def run_case(self, name, *args):
        return subprocess.run([sys.executable, str(RUNNER), name, *args], env=self.env,
                              capture_output=True, text=True, timeout=15)

    def test_paths_and_storage_are_valid_in_generated_hooks(self):
        self.script(self.rtk, '''import json, os, pathlib
keys = ('RTK_DB_PATH', 'RTK_RECALL_DB', 'RTK_TEE_DIR')
pathlib.Path('observed-storage.json').write_text(json.dumps({key: os.environ[key] for key in keys}))
print('{}')''')
        self.script(self.claude, '''import json, os, pathlib, subprocess, sys
settings=json.loads(pathlib.Path(sys.argv[sys.argv.index('--settings')+1]).read_text())
payload=json.dumps({'tool_input':{'command':'git status'}})
for event in ('PreToolUse','PostToolUse'):
 command=settings['hooks'][event][0]['hooks'][0]['command']
 result=subprocess.run(command,shell=True,input=payload,text=True,capture_output=True)
 assert result.returncode==0,result.stderr
observed=json.loads(pathlib.Path('observed-storage.json').read_text())
for key,value in observed.items():
 assert value==settings['env'][key]==os.environ[key]
 assert pathlib.Path(value).resolve().is_relative_to(pathlib.Path.cwd())
print(json.dumps({'type':'result','total_cost_usd':0,'is_error':False}))''')
        result = self.run_case('quoted paths')
        self.assertEqual(result.returncode, 0, result.stderr)
        case = self.root / 'runs/quoted paths'
        self.assertEqual(json.loads((case / 'summary.json').read_text())['exit'], 0, (case / 'stderr.txt').read_text())
        self.assertTrue((case / 'post.jsonl').exists())
        self.assertEqual(json.loads((self.root / 'runs/budget.json').read_text())['calls'][0]['state'], 'settled')

    def test_cargo_fixture_records_exact_command_and_exits_seven(self):
        self.script(self.claude, "print('{\"type\":\"result\",\"total_cost_usd\":0}')")
        result = self.run_case('cargo-case', 'raw', 'allow', 'cargo test --test fixture')
        self.assertEqual(result.returncode, 0, result.stderr)
        case = self.root / 'runs/cargo-case'
        cargo = subprocess.run([str(case / 'bin/cargo'), 'test', '--test', 'fixture'],
                               capture_output=True, text=True, timeout=5)
        self.assertEqual(cargo.returncode, 7)
        self.assertIn('tests/payment.rs:42: expected 200, got 503', cargo.stdout)
        self.assertIn('2 passed; 1 failed', cargo.stdout)
        self.assertEqual(json.loads((case / 'cargo-runs.jsonl').read_text()),
                         ['test', '--test', 'fixture'])

    def test_missing_final_cost_blocks_another_call(self):
        self.script(self.claude, "print('{}')")
        self.assertEqual(self.run_case('no-final').returncode, 0)
        blocked = self.run_case('blocked')
        self.assertNotEqual(blocked.returncode, 0)
        ledger = json.loads((self.root / 'runs/budget.json').read_text())
        self.assertEqual(ledger['calls'], [{'label': 'no-final', 'state': 'pending', 'cost': 0.12}])

    def test_timeout_stops_child_and_keeps_reservation(self):
        self.env['TRIO_CALL_TIMEOUT'] = '0.15'
        sentinel = self.root / 'child-survived'
        child = 'import time,pathlib;time.sleep(1);pathlib.Path(' + repr(str(sentinel)) + ').touch()'
        self.script(self.claude, 'import subprocess,sys,time\nsubprocess.Popen([sys.executable,"-c",' + repr(child) + '])\ntime.sleep(30)')
        self.assertEqual(self.run_case('timeout').returncode, 0)
        time.sleep(1.1)
        self.assertFalse(sentinel.exists())
        self.assertNotEqual(self.run_case('blocked').returncode, 0)
        ledger = json.loads((self.root / 'runs/budget.json').read_text())
        self.assertEqual(ledger['calls'][0]['state'], 'pending')


if __name__ == '__main__':
    unittest.main()
