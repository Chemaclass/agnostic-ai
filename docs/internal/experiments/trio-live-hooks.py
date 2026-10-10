import json, os, pathlib, shlex, shutil, signal, subprocess, sys, time
ROOT=pathlib.Path(os.environ.get('TRIO_LIVE_ROOT','/tmp/agnostic-ai-1959-live')); ROOT.mkdir(exist_ok=True)
CLAUDE=os.environ.get('TRIO_CLAUDE_BIN') or shutil.which('claude')
RTK=os.environ.get('TRIO_RTK_BIN') or shutil.which('rtk')
if not CLAUDE or not RTK:raise SystemExit('claude and rtk must be installed')
MODEL='claude-haiku-5-5'
SYSTEM='You are running a controlled command fixture. Use Bash exactly once with the exact command given, timeout 10000 and description fixture-metadata-1959. Do not retry, edit, inspect files, or run other commands. Then report the observed result in one sentence. If denied, report denied and stop.'

def redact(s):
    s=s.replace(str(pathlib.Path.home()),'<USER_HOME>')
    for key in ('CLAUDE_CODE_OAUTH_TOKEN','ANTHROPIC_API_KEY','ANTHROPIC_AUTH_TOKEN'):
        secret=os.environ.get(key)
        if secret:s=s.replace(secret,'<REDACTED>')
    return s

def reserve_budget(label):
    path=ROOT/'budget.json'
    ledger=json.loads(path.read_text()) if path.exists() else {
        'prior_cost':sum((json.loads(f.read_text()).get('result') or {}).get('total_cost_usd',0) for f in ROOT.glob('*/*summary.json')),
        'calls':[],
    }
    if any(call['state']=='pending' for call in ledger['calls']):
        raise RuntimeError('A previous call has no final cost; stop and inspect its outcome')
    spent=ledger['prior_cost']+sum(call['cost'] for call in ledger['calls'])
    if spent+0.12>3:raise RuntimeError('Stop before the $3 provider budget')
    ledger['calls'].append({'label':label,'state':'pending','cost':0.12})
    path.write_text(json.dumps(ledger,indent=2)+'\n')
    return len(ledger['calls'])-1

def invoke(args, project, env, label):
    reservation=reserve_budget(label)
    process=subprocess.Popen(args,cwd=project,env=env,stdin=subprocess.DEVNULL,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,start_new_session=True)
    try:
        stdout,stderr=process.communicate(timeout=float(os.environ.get('TRIO_CALL_TIMEOUT','100')))
    except subprocess.TimeoutExpired:
        os.killpg(process.pid,signal.SIGTERM)
        try:stdout,stderr=process.communicate(timeout=2)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid,signal.SIGKILL)
            stdout,stderr=process.communicate()
    events=[]
    for line in stdout.splitlines():
        try:events.append(json.loads(line))
        except ValueError:pass
    final=next((event for event in reversed(events) if event.get('type')=='result'),None)
    if final is not None and isinstance(final.get('total_cost_usd'),(int,float)):
        path=ROOT/'budget.json';ledger=json.loads(path.read_text())
        ledger['calls'][reservation].update(state='settled',cost=final['total_cost_usd'])
        path.write_text(json.dumps(ledger,indent=2)+'\n')
    return subprocess.CompletedProcess(args,process.returncode,stdout,stderr)

def rtk_storage(project):
    return {'RTK_DB_PATH':str(project/'rtk-data/tracking.db'),
            'RTK_RECALL_DB':str(project/'rtk-data/recall.db'),
            'RTK_TEE_DIR':str(project/'rtk-tee')}

def run(name, mode='rtk', policy='allow', command='git status', extra=False):
    p=ROOT/name;p.mkdir(exist_ok=False)
    (p/'.claude').mkdir(exist_ok=True);(p/'bin').mkdir(exist_ok=True);(p/'rtk-profile').mkdir(exist_ok=True)
    subprocess.run(['/usr/bin/git','init','-q',str(p)],check=True)
    (p/'fixture.txt').write_text('fixture 1959\n')
    (p/'fail-fixture.sh').write_text("printf '%s\\n' 'FAIL tests/payment.rs:42 expected 200 got 503' >&2\nexit 7\n")
    (p/'rtk-data').mkdir();(p/'rtk-tee').mkdir()
    hook=p/'hook.py'
    hook.write_text('''import sys,subprocess,json,os,pathlib
p=pathlib.Path(__file__).parent
body=sys.stdin.buffer.read()
with (p/'hooks.jsonl').open('a') as f:f.write(json.dumps({'registration':sys.argv[1:] or ['project'],'payload':json.loads(body)})+'\\n')
env=dict(os.environ,CLAUDE_CONFIG_DIR=str(p/'rtk-profile'),RTK_DB_PATH=str(p/'rtk-data/tracking.db'),RTK_RECALL_DB=str(p/'rtk-data/recall.db'),RTK_TEE_DIR=str(p/'rtk-tee'))
mode='''+repr(mode)+'''
if mode=='raw':sys.exit(0)
if mode=='missing':
 env['PATH']='/usr/bin:/bin'
 r=subprocess.run(['/bin/sh','-c','command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude'],input=body,capture_output=True,env=env)
else:
 r=subprocess.run(['''+repr(RTK)+''','hook','claude'],input=body,capture_output=True,env=env)
with (p/'hook-replies.jsonl').open('a') as f:f.write(json.dumps({'exit':r.returncode,'stdout':r.stdout.decode(),'stderr':r.stderr.decode()})+'\\n')
sys.stdout.buffer.write(r.stdout);sys.stderr.buffer.write(r.stderr);sys.exit(r.returncode)
''')
    post=p/'post.py';post.write_text("import sys,json,pathlib\np=pathlib.Path(__file__).parent\nwith (p/'post.jsonl').open('a') as f:f.write(sys.stdin.read()+'\\n')\n")
    for binary,real in [('rtk',RTK),('git','/usr/bin/git')]:
        f=p/'bin'/binary
        f.write_text('#!/usr/bin/python3\nimport os,sys,json,pathlib\np=pathlib.Path(__file__).resolve().parent.parent\nwith (p/"executed.jsonl").open("a") as out:out.write(json.dumps({"binary":'+repr(binary)+',"argv":sys.argv[1:]})+"\\n")\nif '+repr(binary)+'=="git" and sys.argv[1:]==["status"]:(p/"executed-command.sentinel").write_text("git status executed\\n")\nos.execv('+repr(real)+',["'+binary+'"]+sys.argv[1:])\n');f.chmod(0o755)
    perms={'allow':[],'ask':[],'deny':[]}
    if policy in ('ask-wrapper-allow','deny-wrapper-allow'):
        perms[policy.split('-')[0]]=['Bash('+command+')'];perms['allow']=['Bash(rtk '+command+')']
    elif policy!='none':perms[policy]=['Bash('+command+')']
    if policy=='allow' and command.startswith('rtk '):perms['allow']=['Bash('+command+')']
    h={'matcher':'Bash','hooks':[{'type':'command','command':shlex.join(['/usr/bin/python3',str(hook),'project'])}]}
    settings={'permissions':perms,'env':{'PATH':str(p/'bin')+':'+os.environ['PATH'],**rtk_storage(p)},'hooks':{'PreToolUse':[h],'PostToolUse':[{'matcher':'Bash','hooks':[{'type':'command','command':shlex.join(['/usr/bin/python3',str(post)])}]}]},'enabledPlugins':{}}
    settings['hooks']['PostToolUseFailure']=settings['hooks']['PostToolUse']
    if os.environ.get('TRIO_DISABLE_PLUGINS'):
        user_settings=pathlib.Path.home()/'.claude/settings.json'
        flags=json.loads(user_settings.read_text()).get('enabledPlugins',{}) if user_settings.exists() else {}
        settings['enabledPlugins']={name:False for name in flags}
        settings['disableBundledSkills']=True
        settings['skillOverrides']={child.name:'off' for directory in [pathlib.Path.home()/'.claude/skills',pathlib.Path.home()/'.agents/skills'] if directory.is_dir() for child in directory.iterdir() if child.is_dir()}
    (p/'.claude/settings.json').write_text(json.dumps(settings,indent=2))
    args=[CLAUDE,'-p','--model',MODEL,'--effort','low','--max-budget-usd','0.12','--max-turns','3','--setting-sources','project','--strict-mcp-config','--mcp-config','{"mcpServers":{}}','--disable-slash-commands','--no-session-persistence','--tools','Bash','--permission-mode','manual','--permission-prompts','none','--system-prompt',SYSTEM,'--output-format','stream-json','--verbose','--include-hook-events']
    args+=['--settings',str(p/'.claude/settings.json')]
    if os.environ.get('TRIO_RESTRICTED'):args+=['--restricted']
    if policy=='allow':args+=['--allowedTools','Bash('+command+')']
    if policy in ('ask-wrapper-allow','deny-wrapper-allow'):args+=['--allowedTools','Bash(rtk '+command+')']
    if extra and extra!='actual-global':
        profile=p/'simulated-global.json';profile.write_text(json.dumps({'hooks':{'PreToolUse':[{'matcher':'Bash','hooks':[{'type':'command','command':shlex.join(['/usr/bin/python3',str(hook),'simulated-global'])}]}]}}))
        args+=['--settings',str(profile)]
    if os.environ.get('TRIO_ISOLATED_PROFILE') or extra=='actual-global':
        profile=p/'host-profile';profile.mkdir(exist_ok=True)
        args[args.index('--setting-sources')+1]='user,project'
        if extra=='actual-global':
            (profile/'settings.json').write_text(json.dumps({'hooks':{'PreToolUse':[{'matcher':'Bash','hooks':[{'type':'command','command':shlex.join(['/usr/bin/python3',str(hook),'user'])}]}]}}))
    args+=['--','Run exactly this shell command once: '+command]
    (p/'invocation.json').write_text(json.dumps(args,indent=2))
    started=time.time()
    env=dict(os.environ,PATH=str(p/'bin')+':'+os.environ['PATH'],**rtk_storage(p))
    if os.environ.get('TRIO_ISOLATED_PROFILE') or extra=='actual-global':env['CLAUDE_CONFIG_DIR']=str(p/'host-profile')
    r=invoke(args,p,env,name)
    (p/'stderr.txt').write_text(redact(r.stderr))
    events=[]
    for line in r.stdout.splitlines():
        try:events.append(json.loads(line))
        except ValueError:pass
    results=[e for e in events if e.get('type')=='result']
    init=next((e for e in events if e.get('subtype')=='init'),{})
    startup={'plugin_count':len(init.get('plugins',[])),'skill_count':len(init.get('skills',[])),'tools':init.get('tools',[])}
    for event in events:
        if event.get('subtype')=='init':event['plugins']=[{'redacted':True} for _ in event.get('plugins',[])]
    (p/'events.jsonl').write_text(redact(''.join(json.dumps(e)+'\n' for e in events)))
    summary={'startup':startup,'case':name,'exit':r.returncode,'elapsed':round(time.time()-started,2),'result':results[-1] if results else None}
    (p/'summary.json').write_text(redact(json.dumps(summary,indent=2)))
    print(json.dumps({'case':name,'startup':startup,'exit':r.returncode,'elapsed':summary['elapsed'],'cost':(summary['result'] or {}).get('total_cost_usd'),'denials':(summary['result'] or {}).get('permission_denials'),'text':(summary['result'] or {}).get('result')}),flush=True)

def isolated_suite():
    os.environ['TRIO_ISOLATED_PROFILE']='1'
    for mode in ('raw','rtk'):
        for decision in ('ask','deny'):
            run('isolated_'+decision+'_'+mode,mode,decision+'-wrapper-allow')
    run('actual_global','rtk','allow','git status','actual-global')
    remove_user_registration()

def remove_user_registration():
    p=ROOT/'actual_global'
    if (p/'after-removal-summary.json').exists():raise RuntimeError('Removal replay already recorded')
    summary=json.loads((p/'summary.json').read_text())
    if not summary.get('result') or summary['result'].get('is_error'):
        raise RuntimeError('The dedicated profile has not authenticated successfully')
    registrations=[json.loads(line)['registration'] for line in (p/'hooks.jsonl').read_text().splitlines()]
    if sorted(registrations)!=[['project'],['user']]:
        raise RuntimeError('Expected one actual user and one project hook before removal')
    (p/'host-profile/settings.json').write_text('{}\n')
    before=(p/'hooks.jsonl').read_text()
    args=json.loads((p/'invocation.json').read_text())
    env=dict(os.environ,CLAUDE_CONFIG_DIR=str(p/'host-profile'),PATH=str(p/'bin')+':'+os.environ['PATH'],**rtk_storage(p))
    result=invoke(args,p,env,'actual_global_after_user_removal')
    events=[json.loads(line) for line in result.stdout.splitlines() if line.strip().startswith('{')]
    init=next((e for e in events if e.get('subtype')=='init'),{})
    startup={'plugin_count':len(init.get('plugins',[])),'skill_count':len(init.get('skills',[])),'tools':init.get('tools',[])}
    for event in events:
        if event.get('subtype')=='init':event['plugins']=[{'redacted':True} for _ in event.get('plugins',[])]
    (p/'after-removal-events.jsonl').write_text(redact(''.join(json.dumps(e)+'\n' for e in events)))
    (p/'after-removal-stderr.txt').write_text(redact(result.stderr))
    result_event=next((e for e in reversed(events) if e.get('type')=='result'),None)
    after=(p/'hooks.jsonl').read_text()[len(before):]
    report={'startup':startup,'exit':result.returncode,'owners':[json.loads(line)['registration'] for line in after.splitlines()],'result':result_event}
    (p/'after-removal-summary.json').write_text(redact(json.dumps(report,indent=2)))
    print(json.dumps({'case':'actual_global_after_user_removal','owners':report['owners'],'cost':(result_event or {}).get('total_cost_usd')}),flush=True)

if __name__=='__main__':
    if not sys.argv[1:] or sys.argv[1:] in (['--help'],['-h']):
        print('Usage: trio-live-hooks.py NAME [raw|rtk|missing] [allow|ask|deny|none|ask-wrapper-allow|deny-wrapper-allow] [COMMAND]')
        print('       trio-live-hooks.py --isolated-suite | --complete-removal')
        print('Set TRIO_LIVE_ROOT to a fresh output directory. Live runs require existing Claude authentication.')
    elif sys.argv[1:]==['--isolated-suite']:isolated_suite()
    elif sys.argv[1:]==['--complete-removal']:remove_user_registration()
    else:run(*sys.argv[1:])
