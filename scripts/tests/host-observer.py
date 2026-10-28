"""Global conservative Windows observer, imported by host-acceptance.py.

No visible window is created. A pre-existing visible window calibrates the
snapshot; a hidden STATIC window calibrates CREATE delivery only. Any global
SHOW or newly visible window is inconclusive/failing, even if unrelated.
"""
import ctypes as c
from ctypes import wintypes as w
import json
import os
import threading
import time

if os.name != 'nt':
    raise RuntimeError('Windows only')
u = c.WinDLL('user32', use_last_error=True)
k = c.WinDLL('kernel32', use_last_error=True)
P = c.c_void_p
ENUM = c.WINFUNCTYPE(w.BOOL, w.HWND, w.LPARAM)
EVENT = c.WINFUNCTYPE(None, P, w.DWORD, w.HWND, w.LONG, w.LONG, w.DWORD, w.DWORD)
u.EnumWindows.argtypes = [ENUM, w.LPARAM]; u.EnumWindows.restype = w.BOOL
u.IsWindowVisible.argtypes = [w.HWND]; u.IsWindowVisible.restype = w.BOOL
u.SetWinEventHook.argtypes = [w.DWORD, w.DWORD, P, EVENT, w.DWORD, w.DWORD, w.DWORD]; u.SetWinEventHook.restype = P
u.UnhookWinEvent.argtypes = [P]; u.UnhookWinEvent.restype = w.BOOL
u.CreateWindowExW.argtypes = [w.DWORD,w.LPCWSTR,w.LPCWSTR,w.DWORD,c.c_int,c.c_int,c.c_int,c.c_int,w.HWND,P,P,P]; u.CreateWindowExW.restype = w.HWND
u.DestroyWindow.argtypes = [w.HWND]; u.DestroyWindow.restype = w.BOOL
u.PeekMessageW.argtypes = [P,w.HWND,w.UINT,w.UINT,w.UINT]; u.PeekMessageW.restype = w.BOOL
u.TranslateMessage.argtypes = [P]; u.DispatchMessageW.argtypes = [P]
u.MsgWaitForMultipleObjects.argtypes = [w.DWORD,P,w.BOOL,w.DWORD,w.DWORD]; u.MsgWaitForMultipleObjects.restype = w.DWORD
k.GetModuleHandleW.argtypes = [w.LPCWSTR]; k.GetModuleHandleW.restype = P

class MSG(c.Structure):
    _fields_ = [('hwnd',w.HWND),('message',w.UINT),('wParam',w.WPARAM),('lParam',w.LPARAM),('time',w.DWORD),('pt',w.POINT),('private',w.DWORD)]

class Observer:
    def __init__(self):
        self.ready = threading.Event(); self.stop = threading.Event(); self.done = threading.Event()
        self.failure = None; self.shows = []; self.new_visible = set(); self.baseline = set()
        self.hidden_create = False; self.hidden = None
        self.started_at = None; self.ended_at = None
        self.thread = threading.Thread(target=self._run, name='host-window-observer')

    def _snapshot(self):
        found = set()
        @ENUM
        def visit(hwnd, _):
            try:
                if u.IsWindowVisible(hwnd): found.add(int(hwnd))
                return True
            except BaseException as exc:
                self.failure = 'EnumWindows callback: ' + type(exc).__name__
                return False
        if not u.EnumWindows(visit, 0):
            if self.failure: raise RuntimeError(self.failure)
            raise c.WinError(c.get_last_error())
        return found

    def start(self):
        self.thread.start()
        if not self.ready.wait(10): raise RuntimeError('observer startup timeout')
        if self.failure: raise RuntimeError(self.failure)

    def finish(self):
        self.stop.set()
        if not self.done.wait(10): raise RuntimeError('observer shutdown timeout')
        self.thread.join()
        if self.failure: raise RuntimeError(self.failure)
        if not self.hidden_create: raise RuntimeError('hidden CREATE callback not observed')
        if not self.baseline: raise RuntimeError('no pre-existing visible window; calibration inconclusive')
        if self.shows or self.new_visible: raise RuntimeError('global SHOW/new visible window: inconclusive or failure')

    def _run(self):
        hook = None; hwnd = None; msg = MSG(); created = []
        @EVENT
        def on_event(_hook, event, window, obj, child, _thread, _when):
            try:
                if obj != 0 or child != 0: return
                if event == 0x8000: created.append(int(window or 0))
                elif event == 0x8002: self.shows.append(int(window or 0))
            except BaseException as exc:
                self.failure = 'WinEvent callback: ' + type(exc).__name__
        def pump():
            while u.PeekMessageW(c.byref(msg), None, 0, 0, 1):
                u.TranslateMessage(c.byref(msg)); u.DispatchMessageW(c.byref(msg))
                if self.failure: raise RuntimeError(self.failure)
            self.new_visible.update(self._snapshot() - self.baseline)
        def wait_message():
            if u.MsgWaitForMultipleObjects(0,None,False,20,0x04ff) == 0xffffffff:
                raise c.WinError(c.get_last_error())
        try:
            hook = u.SetWinEventHook(0x8000,0x8002,None,on_event,0,0,0)
            if not hook: raise c.WinError(c.get_last_error())
            self.started_at = time.monotonic()
            self.baseline = self._snapshot()
            if not self.baseline: raise RuntimeError('no pre-existing visible window; calibration inconclusive')
            hwnd = u.CreateWindowExW(0,'STATIC','',0,0,0,1,1,None,None,k.GetModuleHandleW(None),None)
            if not hwnd: raise c.WinError(c.get_last_error())
            self.hidden = int(hwnd)
            deadline = time.monotonic()+5
            while self.hidden not in created and time.monotonic()<deadline:
                pump(); wait_message()
            self.hidden_create = self.hidden in created
            if not self.hidden_create: raise RuntimeError('hidden CREATE callback not delivered')
            self.ready.set()
            while not self.stop.is_set():
                pump(); wait_message()
            # Final drain barrier keeps the hook and snapshots active for one second.
            deadline = time.monotonic()+1
            while time.monotonic()<deadline:
                pump(); wait_message()
            pump()
        except BaseException as exc:
            self.failure = '%s: %s' % (type(exc).__name__, exc)
        finally:
            if hwnd and not u.DestroyWindow(hwnd) and not self.failure: self.failure = 'DestroyWindow failed'
            if hook and not u.UnhookWinEvent(hook) and not self.failure: self.failure = 'UnhookWinEvent failed'
            self.ended_at = time.monotonic()
            receipt = {'type': 'host_observer_receipt', 'scope': 'global_conservative',
                       'calibration': 'hidden_CREATE_and_existing_visible_not_positive_SHOW',
                       'baseline_visible_count': len(self.baseline),
                       'hidden_create_observed': self.hidden_create,
                       'global_show_count': len(self.shows), 'new_visible_count': len(self.new_visible),
                       'start_monotonic': self.started_at, 'end_monotonic': self.ended_at,
                       'healthy': self.failure is None}
            print(json.dumps(receipt, sort_keys=True), flush=True)
            self.ready.set(); self.done.set()
