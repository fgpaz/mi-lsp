"""Portable process/resource probes; unavailable counters remain null."""
import os,platform,time
try:
 import resource
except ImportError:
 resource=None
from pathlib import Path
def _rss_bytes():
    if platform.system()=="Linux":
        try:
            for line in Path("/proc/self/status").read_text().splitlines():
                if line.startswith("VmRSS:"): return int(line.split()[1])*1024
        except (OSError,ValueError): pass
    if resource is None: return None
    try:
        value=resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        return int(value if platform.system()=="Darwin" else value*1024)
    except (AttributeError,OSError): return None
def snapshot(): return {"os":platform.system().lower(),"arch":platform.machine().lower(),"python":platform.python_version(),"pid":os.getpid(),"rss_bytes":_rss_bytes(),"monotonic_ns":time.monotonic_ns(),"unit_version":"victory-process-v1"}
def directory_bytes(path):
    total=0
    for item in Path(path).rglob("*") if Path(path).exists() else []:
        if item.is_file():
            try: total+=item.stat().st_size
            except OSError: pass
    return total
