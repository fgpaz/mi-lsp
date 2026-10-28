"""Black-box job/control fixtures; execute only in parent's Windows verification wave.
Usage: py host-job.py ABS_HOST_EXE ABS_PYTHON_EXE
All termination is by retained Popen handle, never by PID. PID opens below are
query/synchronize only, to observe the owned descendants' exit.
"""
import ctypes as c
from ctypes import wintypes as w
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

if os.name != 'nt': raise SystemExit('Windows only')
k=c.WinDLL('kernel32',use_last_error=True)
k.OpenProcess.argtypes=[w.DWORD,w.BOOL,w.DWORD]; k.OpenProcess.restype=c.c_void_p
k.WaitForSingleObject.argtypes=[c.c_void_p,w.DWORD]; k.WaitForSingleObject.restype=w.DWORD
k.CloseHandle.argtypes=[c.c_void_p]

def wait_file(path, timeout=10):
    until=time.monotonic()+timeout
    while time.monotonic()<until:
        if path.exists(): return
        time.sleep(.02)
    raise TimeoutError(str(path))

def payload(mode, directory, code='125'):
    d=Path(directory)
    if mode=='child':
        (d/'child.pid').write_text(str(os.getpid()))
        wait_file(d/'release',30)
    elif mode=='root':
        p=subprocess.Popen([sys.executable, __file__, '--payload','child',str(d)],creationflags=subprocess.CREATE_NO_WINDOW)
        wait_file(d/'child.pid')
        (d/'root.ready').write_text(str(os.getpid()))
        # Retain the process handle until root exits; the child continues in the job.
        c.windll.kernel32.ExitProcess(int(code))
    elif mode=='hold':
        p=subprocess.Popen([sys.executable, __file__, '--payload','child',str(d)],creationflags=subprocess.CREATE_NO_WINDOW)
        wait_file(d/'child.pid'); (d/'root.ready').write_text(str(os.getpid()))
        wait_file(d/'release',30); p.wait(timeout=5)
    elif mode=='foreign':
        (d/'foreign.ready').write_text(str(os.getpid())); wait_file(d/'foreign.release',30)

def open_observer(pid):
    h=k.OpenProcess(0x00100000 | 0x1000,False,pid)
    if not h: raise c.WinError(c.get_last_error())
    return h

def record(path):
    raw=path.read_bytes(); lines=raw.splitlines(keepends=True)
    assert len(lines)==2 and all(x.endswith(b'\n') for x in lines),raw
    r=json.loads(lines[1]); assert lines[0]==('execsafe-host/v1 %s %d\n'%(r['kind'],r['exit_code'])).encode()
    assert r['protocol']=='execsafe-host/v1'; return r

def main():
    host,python=map(Path,sys.argv[1:]); assert host.is_absolute() and host.is_file() and python.is_absolute() and python.is_file()
    script=str(Path(__file__).resolve())
    for scenario in ('wait','INT','TERM','invalid','timeout','hardkill','exclusive'):
        with tempfile.TemporaryDirectory() as tmp:
            d=Path(tmp); control=d/'control'; control.touch(); result=d/'control.result'
            if scenario=='exclusive':
                result.write_bytes(b'preserve')
                p=subprocess.run([str(host),'--control-file',str(control),'--',str(python),'-c','pass'],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,timeout=10,creationflags=0)
                assert p.returncode==125 and result.read_bytes()==b'preserve'; continue
            foreign=subprocess.Popen([str(python),script,'--payload','foreign',str(d)],creationflags=subprocess.CREATE_NO_WINDOW)
            p=None; child_handle=None
            try:
                wait_file(d/'foreign.ready')
                mode='root' if scenario=='wait' else 'hold'
                timeout='1500' if scenario=='timeout' else '15000'
                p=subprocess.Popen([str(host),'--timeout-ms',timeout,'--control-file',str(control),'--',str(python),script,'--payload',mode,str(d),'125'],stdin=subprocess.DEVNULL,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL,creationflags=0)
                wait_file(d/'root.ready'); child_handle=open_observer(int((d/'child.pid').read_text()))
                if scenario=='wait':
                    assert p.poll() is None and k.WaitForSingleObject(child_handle,0)==258
                    (d/'release').touch(); assert p.wait(timeout=10)==0
                    r=record(result); assert r['kind']=='root_exit' and r['exit_code']==125
                elif scenario in ('INT','TERM'):
                    prefix='I' if scenario=='INT' else 'TE'
                    control.write_text(prefix,encoding='ascii'); time.sleep(.15)
                    assert p.poll() is None and k.WaitForSingleObject(child_handle,0)==258
                    control.write_text(scenario+'\n',encoding='ascii'); assert p.wait(timeout=10)==0
                    r=record(result); assert r['kind']=='cancelled' and r['signal']==scenario and r['exit_code']==(130 if scenario=='INT' else 143)
                elif scenario=='invalid':
                    control.write_text('INVALID\n',encoding='ascii'); assert p.wait(timeout=10)==0
                    assert record(result)['kind']=='infra_error'
                elif scenario=='timeout':
                    assert p.wait(timeout=10)==0; assert record(result)['kind']=='infra_error'
                else:
                    # Popen.terminate calls TerminateProcess on its original retained handle.
                    p.terminate(); p.wait(timeout=10)
                assert k.WaitForSingleObject(child_handle,5000)==0,scenario
                assert foreign.poll() is None,scenario
            finally:
                if p and p.poll() is None: p.terminate(); p.wait(timeout=10)
                if child_handle: k.CloseHandle(child_handle)
                (d/'foreign.release').touch(); foreign.wait(timeout=10)
    print('job/control fixtures completed; not window conformance')

if __name__=='__main__':
    if len(sys.argv)>1 and sys.argv[1]=='--payload': payload(*sys.argv[2:])
    else: main()
