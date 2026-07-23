"""Local command wrapper with bounded timeout and resource metadata."""
import subprocess,time
from .process_metrics import snapshot
def run(argv,cwd=None,timeout_s=30,env=None):
    if not isinstance(argv,(list,tuple)) or not argv or any(not isinstance(x,str) for x in argv): raise ValueError("argv must be a non-empty argument list")
    before=snapshot(); start=time.monotonic_ns()
    try:
        proc=subprocess.run(list(argv),cwd=cwd,env=env,capture_output=True,text=True,timeout=timeout_s,check=False); timed_out=False
    except subprocess.TimeoutExpired as exc:
        proc=subprocess.CompletedProcess(argv,124,exc.stdout or "",exc.stderr or ""); timed_out=True
    after=snapshot()
    return {"argv":list(argv),"returncode":proc.returncode,"stdout":proc.stdout,"stderr":proc.stderr,"timed_out":timed_out,"duration_ms":(time.monotonic_ns()-start)/1_000_000,"process":{"before":before,"after":after}}
