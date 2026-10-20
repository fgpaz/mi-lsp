#!/usr/bin/env python3
"""One authorized Pi model attempt; no product build/test and no provider retries."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import selectors
import shutil
import signal
import subprocess
import time

HERE = Path(__file__).resolve().parent
REPO = HERE.parent.parent
MODELS = {'minimax': 'opencode-go/minimax-m3.1-flash-free', 'longcat': 'opencode-go/longcat-2.5-preview-free'}
DATA = REPO / 'results/harness-measurement'

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def safe_error(text):
    """Do not persist provider bodies, request headers, or credential values."""
    low = str(text).lower()
    status = re.search(r'\b(400|401|403|404|408|429|5\d\d)\b', low)
    return {'http_status': int(status[1]) if status else None,
            'category': next((name for needle, name in [('endpoint is unavailable','upstream_unavailable'), ('rate limit','rate_limit'), ('quota','quota'), ('unauthorized','auth'), ('aborted','aborted')] if needle in low), 'provider_or_harness_error'),
            'body': 'omitted_by_redaction_policy'}

def fixture(root):
    if root.exists():
        raise SystemExit('fixture_exists: preserve prior attempt; use fresh checkout for another campaign')
    (root / 'src').mkdir(parents=True)
    (root / '.home').mkdir()
    (root / '.cache').mkdir()
    (root / '.pi').mkdir()
    (root / '.pi/settings.json').write_text(json.dumps({'retry': {'enabled': False}, 'compaction': {'enabled': False}}))
    (root / 'go.mod').write_text('module example.invalid/paired-pilot\n\ngo 1.26.0\n')
    (root / 'src/greeter.go').write_bytes((HERE / 'seed/greeter.go').read_bytes())
    (root / 'src/handler.go').write_text('package greeter\n\nfunc Handle() string { return Greet(42) }\n')
    (root / 'src/legacy_greeter.go').write_text('package greeter\n\nfunc LegacyGreeting() string { return "Hello, historical" }\n')
    (root / '.gitignore').write_text('.cache/\n.home/\n.pi/\n.mi-lsp/\n')
    subprocess.run(['git','init','-q',str(root)], check=True)
    return {str(p.relative_to(root)): digest(p) for p in sorted(root.glob('src/*.go'))}

def oracle(root, events):
    """Deterministic task facts only, not implementation FINAL_VERIFY or a test suite."""
    text = (root / 'src/greeter.go').read_text()
    caller = (root / 'src/handler.go').read_text()
    final = '\n'.join(c.get('text','') for e in events if e.get('type')=='message_end' and e.get('message',{}).get('role')=='assistant' for c in e['message'].get('content',[]) if c.get('type')=='text')
    observed = False
    semantic_tools = []
    for e in events:
        if e.get('type') == 'tool_execution_start' and e.get('toolName') == 'bench_command':
            semantic_tools.append(e.get('args',{}).get('command',''))
        if e.get('type') == 'tool_execution_end' and e.get('toolName') == 'bench_command':
            result = json.dumps(e.get('result',{}))
            if '42' in result and ('cannot use' in result or 'IncompatibleAssign' in result):
                observed = True
    # An edit must complete before the first diagnostic request, not in a sibling batch.
    edit_end = next((i for i,e in enumerate(events) if e.get('type')=='tool_execution_end' and e.get('toolName') in ('edit','write') and not e.get('isError')), None)
    diag_start = next((i for i,e in enumerate(events) if e.get('type')=='tool_execution_start' and ('diagnostics' in e.get('args',{}).get('command','') or 'gopls check' in e.get('args',{}).get('command',''))), None)
    scores = {
        'locate': 'src/greeter.go' in final,
        'edit': bool(re.search(r'return\s+"Hola, "\s*\+\s*name',text)) and 'func Greet(name string) string' in text,
        'semantic': observed and bool(re.search(r'Greet\(\s*"world"\s*\)',caller)),
    }
    return {'tasks':scores, 'successes':sum(scores.values()), 'total':3,
            'detected_before_test':int(observed), 'seeded_semantic_errors':1,
            'edit_before_diagnostics': edit_end is not None and diag_start is not None and edit_end < diag_start,
            'tests_executed':0, 'diagnostic_commands':semantic_tools}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('phase', choices=['baseline','after'])
    parser.add_argument('model', choices=MODELS)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--integrated-sha', default='')
    a = parser.parse_args()
    if a.phase == 'after' and not a.integrated_sha:
        parser.error('after requires parent FINAL_VERIFY integrated SHA, never lane code')
    binary = a.binary.resolve(strict=True)
    output = DATA / a.phase / a.model
    output.mkdir(parents=True, exist_ok=False)  # no overwrites/repeat-until-success
    root = DATA / 'work' / a.model  # identical absolute cwd across the pair
    if a.phase == 'after':
        if not (DATA / 'baseline' / a.model / 'metrics.json').exists():
            raise SystemExit('missing baseline; not comparable')
        # Only our campaign-owned disposable fixture is reset; raw baseline remains intact.
        if root.exists():
            shutil.rmtree(root)
    hashes = fixture(root)
    env = os.environ.copy()
    env.update(BENCH_MI_LSP=str(binary), BENCH_AUDIT=str(output/'guard.jsonl'),
               PI_TELEMETRY='0', PI_OFFLINE='1', TMPDIR=str(root/'.cache'))
    argv = ['pi','-p','--no-session','--model', MODELS[a.model], '--mode','json',
            '--no-extensions','--no-mcp','--no-skills','--no-context-files','--no-prompt-templates',
            '--no-themes','--approve','--offline','--thinking','low',
            '--tools','read,edit,write,bench_command','-e',str(HERE/'guard.ts'),
            (HERE/'prompt.md').read_text()]
    meta = {'schema':'harness-measurement/v1','phase':a.phase,'model_requested':MODELS[a.model],
            'model_version':'alias_only; provider does not publish immutable revision here',
            'harness':subprocess.check_output(['pi','--version'],text=True).strip(),
            'repo_head':subprocess.check_output(['git','rev-parse','HEAD'],cwd=REPO,text=True).strip(),
            'integrated_sha':a.integrated_sha or None,'binary_sha256':digest(binary),
            'prompt_sha256':digest(HERE/'prompt.md'),'guard_sha256':digest(HERE/'guard.ts'),
            'runner_sha256':digest(Path(__file__)),'fixture_sha256':hashes,
            'limits':{'turns':12,'max_output_per_request':2048,'wall_seconds':240,
                      'observed_total_tokens_stop_threshold':100000,'provider_retries':False},
            'argv':argv,'cwd':str(root),'started_utc':time.strftime('%Y-%m-%dT%H:%M:%SZ',time.gmtime())}
    (output/'invocation.json').write_text(json.dumps(meta,indent=2)+'\n')
    start=time.monotonic()
    proc=subprocess.Popen(argv,cwd=root,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,start_new_session=True)
    selector=selectors.DefaultSelector()
    selector.register(proc.stdout,selectors.EVENT_READ,'out'); selector.register(proc.stderr,selectors.EVENT_READ,'err')
    events=[]; buffers={'out':b'','err':b''}; errors=[]; reason=None; total=0
    with (output/'events.jsonl').open('w') as saved:
        while selector.get_map():
            if time.monotonic()-start>240 and reason is None:
                reason='wall_timeout'; os.killpg(proc.pid,signal.SIGKILL)
            for key,_ in selector.select(timeout=0.5):
                chunk=os.read(key.fileobj.fileno(),65536)
                stream=key.data
                if not chunk:
                    selector.unregister(key.fileobj); continue
                buffers[stream]+=chunk
                while b'\n' in buffers[stream]:
                    line,buffers[stream]=buffers[stream].split(b'\n',1)
                    if stream=='err':
                        if line.strip(): errors.append(safe_error(line.decode(errors='replace')))
                        continue
                    try: e=json.loads(line)
                    except ValueError:
                        errors.append({'category':'non_json_stdout','body':'omitted_by_redaction_policy'}); continue
                    # Keep authoritative native events, never cumulative deltas or duplicated agent_end messages.
                    if e.get('type') not in ('session','turn_start','message_end','tool_execution_start','tool_execution_end','agent_settled','auto_retry_start','auto_retry_end'):
                        continue
                    m=e.get('message',{})
                    if m.get('errorMessage'): m['errorMessage']=safe_error(m['errorMessage'])
                    for k in ('errorMessage','finalError'):
                        if k in e: e[k]=safe_error(e[k])
                    events.append(e); saved.write(json.dumps(e)+'\n'); saved.flush()
                    if e.get('type')=='message_end' and m.get('role')=='assistant':
                        total+=m.get('usage',{}).get('totalTokens',0)
                        if total>=100000 and reason is None:
                            reason='token_threshold'; os.killpg(proc.pid,signal.SIGKILL)
    code=proc.wait()
    elapsed=time.monotonic()-start
    if buffers['err'].strip(): errors.append(safe_error(buffers['err'].decode(errors='replace')))
    messages=[e['message'] for e in events if e.get('type')=='message_end' and e.get('message',{}).get('role')=='assistant']
    usages=[m.get('usage') for m in messages if m.get('stopReason') not in ('error','aborted')]
    usage={}
    for key in ('input','output','cacheRead','cacheWrite','totalTokens'):
        vals=[u.get(key) if u else None for u in usages]
        usage[key]={'value':sum(vals) if vals and all(isinstance(v,(int,float)) for v in vals) else None,
                    'source':'pi.message_end.message.usage',
                    'reason':None if vals and all(isinstance(v,(int,float)) for v in vals) else 'no successful provider usage for all completed responses'}
    tool_starts=[e for e in events if e.get('type')=='tool_execution_start']
    tool_ends=[e for e in events if e.get('type')=='tool_execution_end']
    reads=sorted({e['args']['path'] for e in tool_starts if e.get('toolName')=='read'})
    facts=oracle(root,events)
    audit=[json.loads(s) for s in (output/'guard.jsonl').read_text().splitlines()] if (output/'guard.jsonl').exists() else []
    provider_errors=[m['errorMessage'] for m in messages if m.get('errorMessage')]
    status=reason or ('provider_error' if provider_errors else 'harness_error' if code or errors else 'task_incomplete' if facts['successes']!=3 or not facts['edit_before_diagnostics'] else 'completed')
    metrics={**meta,'status':status,'exit_code':code,'seconds':round(elapsed,3),'usage':usage,
             'model_observed':sorted({m.get('provider','')+'/'+m.get('model','') for m in messages}),
             'assistant_responses':len(messages),'request_payloads':sum(e['type']=='request' for e in audit),
             'http_statuses':[e['status'] for e in audit if e['type']=='http'],
             'api_attempts_note':'request_payloads is observed pre-provider hook count, not a claim about hidden transport retries',
             'tool_calls':len(tool_starts),'tool_errors':sum(bool(e.get('isError')) for e in tool_ends),
             'explicit_files_read':reads,'explicit_files_read_count':len(reads),
             'implicit_backend_reads':'unknown: gopls/mi-lsp do not report full file-I/O inventory',
             'provider_errors':provider_errors,'harness_errors':errors,'oracle':facts}
    (output/'metrics.json').write_text(json.dumps(metrics,indent=2)+'\n')
    shutil.copytree(root/'src',output/'final-src')
    print(json.dumps({'phase':a.phase,'model':a.model,'status':status,'metrics':str(output/'metrics.json')}))

if __name__=='__main__': main()
