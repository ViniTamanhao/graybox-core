#!/usr/bin/env python3
"""Small Linux campaign runner. Private evidence; no simulated agent decisions."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import random
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
sys.path.insert(0, str(ROOT / 'evaluations/agent-skill'))
from fixture import checked, process, read_start, run
from grade import grade, request
TASKS = json.loads((HERE / 'tasks.json').read_text())


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def save(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')


def environment():
    # Synthetic, ephemeral credentials, never included in prompt/provenance.
    return {**os.environ, 'API_AUTH': 'Bearer ' + os.urandom(20).hex(),
            'OTHER_AUTH': 'Bearer ' + os.urandom(20).hex()}


def source(work):
    work.mkdir(mode=0o700)
    for name in ('main.go', 'main_test.go'):
        shutil.copyfile(HERE / 'app' / name, work / name)
    for name in ('go.mod', 'go.sum'):
        shutil.copyfile(ROOT / name, work / name)
    (work / 'go.mod').write_text((work / 'go.mod').read_text().replace('github.com/ViniTamanhao/graybox-core', 'example.test/commerce'))
    (work / 'bin').mkdir()
    (work / '.gitignore').write_text('/bin/\n*.sqlite*\n*.graybox\n*.jsonl\n')
    checked(['git', 'init', '-q'], work)


def build(work):
    checked(['go', 'build', '-o', work / 'bin/api', '.'], work)


def reset(work, env):
    result = run([work / 'bin/api', '--db', work / 'commerce.sqlite', '--reset'], work, env)
    if result.returncode:
        raise RuntimeError(result.stderr)


def capture(work, env, binary):
    reset(work, env)
    sets = {
        'reads': [('GET','/products/1',None),('GET','/orders/40',None),('GET','/orders/41',None)],
        'quotes': [('POST','/discounts/apply',dict(product_id=p,quantity=q,code=c)) for p,q,c in [(1,1,'SAVE10'),(2,1,'SAVE10'),(2,3,'SAVE10'),(3,1,'LOYAL15'),(1,0,'')]],
        'writes': [('POST','/orders',dict(product_id=1,quantity=2,code='SAVE10')),('GET','/orders/43',None),('POST','/orders/40/cancel',None),('POST','/orders/40/cancel',None),('POST','/orders/42/cancel',None)],
        'unauth': [('GET','/products/1',None)]
    }
    baseline = work / 'evidence'
    baseline.mkdir()
    manifest = dict(source_revision=checked(['git','rev-parse','HEAD'],ROOT).strip(), source_sha256=digest(work/'main.go'), binary_sha256=digest(binary), policy='mandatory headers; default Date/Content-Length ignores only', recordings={})
    with process([work/'bin/api','--db',work/'commerce.sqlite'],work,env) as app:
        target=read_start(app)
        for name, cases in sets.items():
            with socket.socket() as reserve:
                reserve.bind(('127.0.0.1',0)); listen=f'127.0.0.1:{reserve.getsockname()[1]}'
            path=baseline/(name+'.graybox')
            with process([binary,'record','--listen',listen,'--target',target,'--output',path,'--json'],work) as recorder:
                read_start(recorder,True)
                observed=[]
                for method,route,body in cases:
                    observed.append(request('http://'+listen,method,route,body,None if name=='unauth' else env['API_AUTH']))
                recorder.send_signal(signal.SIGTERM)
                assert recorder.wait(timeout=15)==0
            listing=json.loads(checked([binary,'ls',path,'--json'],work))
            assert len(listing['exchanges'])==len(cases)
            for exchange in listing['exchanges']:
                obj=json.loads(checked([binary,'show',path,str(exchange['id']),'--json'],work))
                assert not obj.get('proxy_error')
                for side in ('request','response'):
                    assert obj[side]['body']['complete'] and not obj[side]['body']['truncated']
            assert env['API_AUTH'].encode() not in path.read_bytes()
            manifest['recordings'][name]=dict(sha256=digest(path),exchanges=listing['exchanges'],responses=observed)
    save(baseline/'provenance.json',manifest)
    reset(work,env)
    return manifest


def evaluate(work, task, env):
    build(work)
    reset(work,env)
    with process([work/'bin/api','--db',work/'commerce.sqlite'],work,env) as child:
        result=grade(read_start(child),task,env['API_AUTH'],env['OTHER_AUTH'])
    # Reopen the same database; persistence must survive process exit.
    with process([work/'bin/api','--db',work/'commerce.sqlite'],work,env) as child:
        actual=request(read_start(child),'GET','/orders/43',None,env['API_AUTH'])
        ok=actual[0]==200 and actual[1]['state']=='cancelled'
        result['results'].append(dict(name='restart-persistence',passed=ok,critical=True))
        result['total']+=1;result['passed']+=int(ok);result['critical_failures']+=int(not ok)
    return result


def prepare(root):
    root.mkdir(mode=0o700,exist_ok=False)
    env=environment();work=root/'reference';source(work);build(work)
    binary=root/'graybox'
    revision=checked(['git','rev-parse','HEAD'],ROOT).strip()
    checked(['go','build','-ldflags','-X main.version=eval-'+revision,'-o',binary,'./cmd/graybox'],ROOT)
    result=evaluate(work,'refactor',env)
    assert result['passed']==result['total'], result
    manifest=capture(work,env,binary)
    save(root/'reference-grade.json',result)
    save(root/'credentials.json',{k:env[k] for k in ('API_AUTH','OTHER_AUTH')})
    order=[(t,c) for t in TASKS for c in 'ABC'];random.Random(403).shuffle(order)
    save(root/'order.json',order)
    for task,condition in order:
        trial=root/(task+'-'+condition);trial.mkdir(mode=0o700)
        app=trial/'app';source(app)
        if task=='recovery':
            s=(app/'main.go').read_text();assert 'state != "pending"' in s
            (app/'main.go').write_text(s.replace('state != "pending"','state == "cancelled"'))
        build(app)
        note='''# Development environment
This is an isolated disposable commerce API with no external integrations.
Runtime API_AUTH and OTHER_AUTH are provisioned for Alice and Bob; do not print
credentials. Use them only for this fixture. Read source for API contracts.
Run go test ./... and go build -o bin/api . after edits.
Run bin/api --db commerce.sqlite --reset only while your server is stopped;
then bin/api --db commerce.sqlite prints an OS-selected loopback URL.
No service is currently running. Start only your own current build; retain its
PID/session and stop only that process. No unrelated process cleanup is allowed.
SQLite state can be reset only in this workspace; repeated writes require reset.
Reports must use fresh private temporary directories outside the checkout.
Do not commit binaries, databases, recordings, credentials or reports.
'''
        if condition!='A':
            shutil.copyfile(binary,app/'bin/graybox');(app/'bin/graybox').chmod(0o700)
            shutil.copytree(work/'evidence',app/'evidence')
            note+='''\nAvailable optional tool: bin/graybox. Historical evidence and provenance are in
evidence/ relative to this project. Captured from accepted source
before changes. Keep recording bytes and ignore policy unchanged. The original
captured service is stopped; never use its stale port. Inspect selection/state.
reads and quotes do not mutate state; writes must run in full original order
from reset seed (IDs are deterministic). unauth must run without credential
injection. Other sets require API_AUTH injection. Do not inject credentials into
unauth. Never repeat writes without stopping and resetting your owned database.
'''
        if condition=='C':shutil.copytree(ROOT/'skills/graybox',app/'.agents/skills/graybox')
        (app/'ENVIRONMENT.md').write_text(note)
        (app/'TASK.txt').write_text(TASKS[task]+'\n\nRead ENVIRONMENT.md for tooling and constraints.\n')
        save(trial/'inputs.json',{p.name:digest(p) for p in app.iterdir() if p.is_file()})
    save(root/'manifest.json',manifest)
    return root


def check(root):
    env={**os.environ,**json.loads((root/'credentials.json').read_text())}
    ref=root/'reference';binary=root/'graybox'
    # Each set starts reset; a write recording is compared exactly once.
    for name in ('reads','quotes','writes','unauth'):
        reset(ref,env)
        with process([ref/'bin/api','--db',ref/'commerce.sqlite'],ref,env) as child:
            args=[binary,'diff',ref/'evidence'/(name+'.graybox'),'--target',read_start(child),'--json']
            if name!='unauth':args+=['--secret-header','Authorization=API_AUTH']
            output=run(args,ref,env); report=json.loads(output.stdout)
            assert output.returncode==0 and report['summary']['total']>0, output
    seeded=root/'recovery-A'/'app'
    result=evaluate(seeded,'recovery',env)
    assert {r['name'] for r in result['results'] if not r['passed']}=={'shipped-cancel','shipped-preserved'}
    # Confirm a real diff sees the seeded state defect, separately from agents.
    reset(seeded,env)
    with process([seeded/'bin/api','--db',seeded/'commerce.sqlite'],seeded,env) as child:
        output=run([binary,'diff',ref/'evidence/writes.graybox','--id','5','--target',read_start(child),'--secret-header','Authorization=API_AUTH','--json'],seeded,env)
        assert output.returncode==1 and json.loads(output.stdout)['summary']['changed']==1
    for name,record in json.loads((root/'manifest.json').read_text())['recordings'].items():
        assert digest(ref/'evidence'/(name+'.graybox'))==record['sha256']
    feature=root/'feature-A'/'app'
    text=(feature/'main.go').read_text()
    text=text.replace('case "SAVE10":', 'case "LOYAL15":\n if subtotal < 10000 { return price{}, "invalid_discount" }; discount=(subtotal*15+50)/100\n case "SAVE10":')
    (feature/'main.go').write_text(text)
    result=evaluate(feature,'feature',env)
    assert result['passed']==result['total'],result
    print('reference grading, four baseline sets, seeded grading/diff, integrity: PASS')


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode',choices=['prepare','check'])
    parser.add_argument('--root',type=Path)
    args=parser.parse_args();os.umask(0o077)
    if args.mode=='prepare':
        if not args.root:parser.error('--root must be a new private path')
        print(prepare(args.root.resolve()))
    else:
        with tempfile.TemporaryDirectory(prefix='commerce-check-') as tmp:
            check(prepare(Path(tmp)/'campaign'))
if __name__=='__main__':main()
