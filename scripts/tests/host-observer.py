"""Windows window observer for a running, owned host tree; no visible fixture is created.

Usage: py host-observer.py ROOT_PID DURATION_SECONDS
PID is used only for read-only attribution, never for termination. Absence of
observed windows is not conformance: snapshot sampling can miss short-lived
children, and a pre-existing visible window only calibrates enumeration.
"""
import ctypes as c
from ctypes import wintypes as w
import json
import os
import sys
import time

if os.name != 'nt':
    raise SystemExit('Windows only')
u = c.WinDLL('user32', use_last_error=True)
k = c.WinDLL('kernel32', use_last_error=True)
P = c.c_void_p
u.EnumWindows.argtypes = [P, w.LPARAM]; u.EnumWindows.restype = w.BOOL
u.IsWindowVisible.argtypes = [w.HWND]; u.IsWindowVisible.restype = w.BOOL
u.GetWindowThreadProcessId.argtypes = [w.HWND, c.POINTER(w.DWORD)]
u.SetWinEventHook.argtypes = [w.DWORD,w.DWORD,P,P,w.DWORD,w.DWORD,w.DWORD]; u.SetWinEventHook.restype = P
u.UnhookWinEvent.argtypes = [P]; u.UnhookWinEvent.restype = w.BOOL
u.PeekMessageW.argtypes = [P,w.HWND,w.UINT,w.UINT,w.UINT]; u.PeekMessageW.restype = w.BOOL
u.TranslateMessage.argtypes = [P]; u.DispatchMessageW.argtypes = [P]
k.CreateToolhelp32Snapshot.argtypes = [w.DWORD,w.DWORD]; k.CreateToolhelp32Snapshot.restype = P
k.CloseHandle.argtypes = [P]
k.OpenProcess.argtypes = [w.DWORD,w.BOOL,w.DWORD]; k.OpenProcess.restype = P
k.GetProcessTimes.argtypes = [P,P,P,P,P]; k.GetProcessTimes.restype = w.BOOL
class PROCESSENTRY(c.Structure):
    _fields_ = [('size',w.DWORD),('usage',w.DWORD),('pid',w.DWORD),('heap',c.c_size_t),('module',w.DWORD),('threads',w.DWORD),('parent',w.DWORD),('priority',c.c_long),('flags',w.DWORD),('exe',w.WCHAR*260)]
k.Process32FirstW.argtypes = [P,c.POINTER(PROCESSENTRY)]; k.Process32FirstW.restype = w.BOOL
k.Process32NextW.argtypes = [P,c.POINTER(PROCESSENTRY)]; k.Process32NextW.restype = w.BOOL
class MSG(c.Structure):
    _fields_ = [('hwnd',w.HWND),('message',w.UINT),('wParam',w.WPARAM),('lParam',w.LPARAM),('time',w.DWORD),('pt',w.POINT),('private',w.DWORD)]
ENUM = c.WINFUNCTYPE(w.BOOL,w.HWND,w.LPARAM)
EVENT = c.WINFUNCTYPE(None,P,w.DWORD,w.HWND,w.LONG,w.LONG,w.DWORD,w.DWORD)

def generation(pid):
    h = k.OpenProcess(0x1000, False, pid)  # query-limited only
    if not h: return None
    try:
        values = [w.FILETIME() for _ in range(4)]
        if not k.GetProcessTimes(h,*[c.byref(x) for x in values]): return None
        return (values[0].dwHighDateTime << 32) | values[0].dwLowDateTime
    finally: k.CloseHandle(h)

def snapshot():
    h = k.CreateToolhelp32Snapshot(2,0)
    if h == P(-1).value: raise c.WinError(c.get_last_error())
    try:
        entry = PROCESSENTRY(); entry.size = c.sizeof(entry); rows=[]
        ok = k.Process32FirstW(h,c.byref(entry))
        while ok:
            rows.append((entry.pid,entry.parent)); ok=k.Process32NextW(h,c.byref(entry))
        return rows
    finally: k.CloseHandle(h)

def main():
    root=int(sys.argv[1]); duration=float(sys.argv[2])
    if not 0 < duration <= 300: raise ValueError('duration must be in (0,300]')
    gen=generation(root)
    if gen is None: raise RuntimeError('root generation unavailable')
    owned={root:gen}; events=[]; visible_existing=[]; uncertainty=[]
    @ENUM
    def calibrate(hwnd, _):
        if u.IsWindowVisible(hwnd): visible_existing.append(int(hwnd))
        return True
    if not u.EnumWindows(calibrate,0): raise c.WinError(c.get_last_error())
    @EVENT
    def on_event(hook,event,hwnd,obj,child,thread,event_time):
        if not hwnd or obj != 0 or child != 0: return
        pid=w.DWORD(); u.GetWindowThreadProcessId(hwnd,c.byref(pid))
        if pid.value in owned and generation(pid.value)==owned[pid.value]:
            events.append({'event':int(event),'pid':pid.value,'generation':owned[pid.value],'visible':bool(u.IsWindowVisible(hwnd))})
    hook=u.SetWinEventHook(0x8002,0x8002,None,on_event,0,0,0)
    if not hook: raise c.WinError(c.get_last_error())
    @ENUM
    def observe(hwnd, _):
        if u.IsWindowVisible(hwnd):
            pid=w.DWORD(); u.GetWindowThreadProcessId(hwnd,c.byref(pid))
            if pid.value in owned and generation(pid.value)==owned[pid.value]:
                events.append({'event':'visible_snapshot','pid':pid.value,'generation':owned[pid.value]})
        return True
    try:
        end=time.monotonic()+duration; msg=MSG()
        while time.monotonic()<end:
            rows=snapshot()
            for pid,parent in rows:
                if parent in owned and generation(parent)==owned[parent] and pid not in owned:
                    g=generation(pid)
                    if g is None: uncertainty.append('child generation inaccessible')
                    else: owned[pid]=g
            while u.PeekMessageW(c.byref(msg),None,0,0,1):
                u.TranslateMessage(c.byref(msg)); u.DispatchMessageW(c.byref(msg))
            if not u.EnumWindows(observe,0): raise c.WinError(c.get_last_error())
            # Wait for OS events rather than an agent or shell polling loop.
            u.MsgWaitForMultipleObjects(0,None,False,20,0x04FF)
    finally: u.UnhookWinEvent(hook)
    print(json.dumps({'existing_visible_windows':len(visible_existing),'owned_generations':owned,'events':events,'uncertainty':uncertainty,'coverage':'sampled, not conformance'}))

if __name__ == '__main__': main()
