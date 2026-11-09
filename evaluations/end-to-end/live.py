#!/usr/bin/env python3
"""Linux mount/PID isolation for real Codex trials; no host evaluator files mounted."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import datetime
import json
import os
from pathlib import Path
import shutil
import subprocess
import time
from runner import ROOT, digest, evaluate, save


def launch_args(trial, env):
    scratch=trial/'scratch';scratch.mkdir(mode=0o700,exist_ok=True)
    home=trial/'home';home.mkdir(mode=0o700,exist_ok=True)
    state=home/'.codex';state.mkdir(mode=0o700,exist_ok=True)
    auth=Path.home()/'.codex/auth.json'
    if not (state/'auth.json').exists():shutil.copyfile(auth,state/'auth.json');(state/'auth.json').chmod(0o600)
    (state/'config.toml').write_text('model = "gpt-6-astra"\nmodel_reasoning_effort = "medium"\n')
    go=Path(shutil.which('go')).resolve().parents[1]
    codex=Path(shutil.which('codex')).resolve()
    modules=subprocess.check_output(['go','env','GOMODCACHE'],text=True).strip()
    args=['bwrap','--die-with-parent','--unshare-pid','--ro-bind','/','/',
          '--tmpfs','/home','--bind',str(scratch),'/tmp','--tmpfs','/run','--ro-bind','/etc/resolv.conf',str(Path('/etc/resolv.conf').resolve()),'--proc','/proc','--dev','/dev',
          '--bind',str(home),'/home/eval','--bind',str(trial/'app'),'/tmp/work',
          '--ro-bind',str(codex.parent),'/tmp/codex-bin','--ro-bind',str(go),'/tmp/go',
          '--ro-bind',modules,'/tmp/modules','--chdir','/tmp/work','--clearenv']
    values={'HOME':'/home/eval','CODEX_HOME':'/home/eval/.codex','PATH':'/tmp/work/bin:/tmp/go/bin:/usr/bin:/bin',
            'GOMODCACHE':'/tmp/modules','GOCACHE':'/home/eval/go-cache','GOPATH':'/home/eval/go',
            'SHELL':'/bin/bash','LANG':'C.UTF-8','API_AUTH':env['API_AUTH'],'OTHER_AUTH':env['OTHER_AUTH']}
    for k,v in values.items():args+=['--setenv',k,v]
    return args


def isolation(trial, env):
    args=launch_args(trial,env)
    # A child cannot read host repo, evaluator grade, sibling trials, or host /proc.
    code='import os; from pathlib import Path; assert not Path('+repr(str(ROOT))+').exists(); assert not Path('+repr(str(trial.parent))+').exists(); assert not Path("/tmp/work/grade.py").exists(); assert not Path("/home/vini/.codex").exists(); print("isolated")'
    result=subprocess.run(args+['python','-c',code],capture_output=True,text=True,timeout=30)
    if result.returncode:raise RuntimeError(result.stderr)
    return result.stdout.strip()


def execute(root, name, probe=False):
    trial=root/name
    if not probe and (trial/'events-metadata.json').exists():
        raise RuntimeError('refusing to overwrite completed trial evidence')
    env={**os.environ,**json.loads((root/'credentials.json').read_text())}
    assert isolation(trial,env)=='isolated'
    command=launch_args(trial,env)+['/tmp/codex-bin/codex','exec','--ephemeral','--sandbox','workspace-write','-c','sandbox_workspace_write.network_access=true','--cd','/tmp/work','--json','--output-last-message','/home/eval/final.txt']
    prompt='Run pwd and go version; report results. Do not modify files.' if probe else (trial/'app/TASK.txt').read_text()
    start=time.monotonic();meta=dict(trial=name,probe=probe,start=datetime.datetime.now(datetime.timezone.utc).isoformat(),limit_seconds=900,model='gpt-6-astra',reasoning='medium',sandbox='workspace-write',network_access=True,isolation='bubblewrap mount/PID',codex_version=subprocess.check_output(['codex','--version'],text=True).strip())
    prefix='probe' if probe else 'events'
    with (trial/(prefix+'.jsonl')).open('w') as out,(trial/(prefix+'.stderr')).open('w') as err:
        child=subprocess.Popen(command,stdin=subprocess.PIPE,stdout=out,stderr=err,text=True,start_new_session=True)
        try:
            child.communicate(prompt,timeout=90 if probe else 900)
            meta['exit']=child.returncode
        except subprocess.TimeoutExpired:
            child.terminate()
            try:child.wait(timeout=15)
            except subprocess.TimeoutExpired:child.kill();child.wait()
            meta['exit']=child.returncode;meta['timeout']=True
    meta['elapsed_seconds']=round(time.monotonic()-start,2)
    meta['trace_sha256']=digest(trial/(prefix+'.jsonl'))
    save(trial/(prefix+'-metadata.json'),meta)
    if not probe:
        evidence=trial/'app/evidence'
        if evidence.exists():
            manifest=json.loads((root/'manifest.json').read_text())
            save(trial/'integrity.json',{name:digest(evidence/(name+'.graybox'))==record['sha256'] for name,record in manifest['recordings'].items()})
        # Namespace exited: its owned descendants are gone before grading.
        try:save(trial/'grade.json',evaluate(trial/'app',name.rsplit('-',1)[0],env))
        except Exception as error:save(trial/'grade.json',dict(blocked=str(error)))
        subprocess.run(['git','diff','--no-index',str(root/'reference/main.go'),str(trial/'app/main.go')],stdout=(trial/'source.diff').open('w'),check=False)
    print(json.dumps(meta),flush=True)


def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('mode',choices=['probe','run']);parser.add_argument('--root',type=Path,required=True);parser.add_argument('--trial');parser.add_argument('--jobs',type=int,choices=[1,2,3],default=1);a=parser.parse_args();os.umask(0o077)
    if a.mode=='probe':execute(a.root,a.trial or 'refactor-A',True)
    elif a.trial:execute(a.root,a.trial)
    else:
        names=[task+'-'+condition for task,condition in json.loads((a.root/'order.json').read_text())]
        with ThreadPoolExecutor(max_workers=a.jobs) as pool:
            list(pool.map(lambda name: execute(a.root,name),names))
if __name__=='__main__':main()
