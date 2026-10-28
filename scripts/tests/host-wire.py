"""Black-box wire fixtures; run only under the parent's Windows verification gate.

Usage: py scripts/tests/host-wire.py ABS_HOST_EXE ABS_PYTHON_EXE
Neither executable is located via PATH. This does not test window visibility or jobs.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile


def main():
    host, python = map(Path, sys.argv[1:])
    if os.name != "nt" or not all(p.is_absolute() and p.is_file() for p in (host, python)):
        raise SystemExit("requires Windows and two absolute existing executable paths")
    program = (
        "import ctypes,os,sys; "
        "data=sys.stdin.buffer.read(); "
        "os.write(1,data); os.write(2,b'\\x00\\xff'); "
        "os.write(1,('\\n'+repr(sys.argv[2:])).encode('utf-8')); "
        "ctypes.windll.kernel32.ExitProcess(int(sys.argv[1]))"
    )
    for code in (0, 1, 125, 130, 143, 0xFEDCBA98):
        with tempfile.TemporaryDirectory() as tmp:
            control = Path(tmp) / "control"
            control.touch(exist_ok=False)
            args = ["", "á雪", 'a"b', "trailing\\", 'x\\"y']
            payload = b"\x00\xff\r\nraw"
            p = subprocess.run([str(host), "--control-file", str(control), "--",
                                str(python), "-c", program, str(code), *args],
                               input=payload, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               timeout=30, creationflags=subprocess.CREATE_NO_WINDOW)
            assert p.returncode == 0, (code, p.returncode)
            assert p.stdout == payload + ("\n" + repr(args)).encode("utf-8"), code
            assert p.stderr == b"\x00\xff", code
            raw = Path(str(control) + ".result").read_bytes()
            lines = raw.splitlines(keepends=True)
            assert len(lines) == 2 and all(line.endswith(b"\n") for line in lines), raw
            record = json.loads(lines[1])
            assert lines[0] == f"execsafe-host/v1 root_exit {code}\n".encode("ascii"), raw
            assert record["protocol"] == "execsafe-host/v1"
            assert record["kind"] == "root_exit" and record["exit_code"] == code, record
    print("wire fixtures passed; not a native visibility/job conformance result")


if __name__ == "__main__":
    main()
