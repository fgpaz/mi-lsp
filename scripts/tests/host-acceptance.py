"""Source-only acceptance driver. Requires Windows, explicit assets and Python.

Usage: py host-acceptance.py ABS_NATIVE_HOST ABS_PYTHON ABS_X64_HOST ABS_ARM64_HOST
Requires an already-visible window for enumeration calibration; never creates
one. Any global SHOW/new visible window makes this conservative run fail.
"""
import importlib.util
import os
from pathlib import Path
import struct
import subprocess
import sys


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(file))
    module = importlib.util.module_from_spec(spec); spec.loader.exec_module(module)
    return module


def pe_machine(path):
    with path.open('rb') as f:
        dos = f.read(64)
        if len(dos) != 64 or dos[:2] != b'MZ': raise ValueError('invalid DOS header')
        offset = struct.unpack_from('<I',dos,60)[0]
        if offset < 64 or offset > path.stat().st_size - 24: raise ValueError('invalid PE offset')
        f.seek(offset); header=f.read(24)
        if len(header)!=24 or header[:4]!=b'PE\0\0': raise ValueError('invalid PE signature')
        machine=struct.unpack_from('<H',header,4)[0]
        size=struct.unpack_from('<H',header,20)[0]
        if size<70: raise ValueError('short optional header')
        optional=f.read(size)
        if len(optional)!=size or struct.unpack_from('<H',optional,0)[0]!=0x20b: raise ValueError('not PE32+')
        if struct.unpack_from('<H',optional,68)[0]!=2: raise ValueError('not GUI subsystem')
        if machine not in (0x8664,0xaa64): raise ValueError('unexpected machine')
        return machine


def main():
    if os.name != 'nt' or len(sys.argv)!=5: raise SystemExit(__doc__)
    host,python,x64,arm64=map(Path,sys.argv[1:])
    if not all(p.is_absolute() and p.is_file() for p in (host,python,x64,arm64)): raise ValueError('absolute existing paths required')
    if pe_machine(x64)!=0x8664 or pe_machine(arm64)!=0xaa64: raise ValueError('asset machine mismatch')
    if pe_machine(host) not in (0x8664,0xaa64): raise ValueError('native host machine')
    load('host_result','host-result.py').fixture_results()
    observer=load('host_observer','host-observer.py').Observer()
    observer.start()
    try:
        for fixture in ('host-wire.py','host-job.py'):
            subprocess.run([str(python),str(Path(__file__).with_name(fixture)),str(host),str(python)],
                           check=True,timeout=180,creationflags=subprocess.CREATE_NO_WINDOW)
    finally:
        observer.finish()
    print('source acceptance fixtures completed; global observer is conservative, not a release certification')

if __name__=='__main__': main()
