"""Truthful child-process metrics and process-tree cleanup for Victory Lab v2.

This module deliberately does not read the harness' RSS.  On Windows it samples
GetProcessMemoryInfo for the child and its currently observable descendants,
while a kill-on-close Job Object provides the cleanup boundary.  If the native
counter or containment primitive is unavailable, the result is explicitly
NOT_COMPARABLE rather than an estimate.
"""
from __future__ import annotations

from dataclasses import dataclass, field
import ctypes
from ctypes import wintypes
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import threading
import time
from typing import Callable, Mapping, Sequence

try:
    from .security_gate import RuntimeProofProbe, _observable_processes
except ImportError:  # pragma: no cover
    from security_gate import RuntimeProofProbe, _observable_processes


STATUS_NOT_COMPARABLE = "NOT_COMPARABLE"
_STATUS_CODES = frozenset({"PASS", STATUS_NOT_COMPARABLE})
_FAILURE_CLASSES = frozenset({"none", "timeout", "crash", "exit_nonzero", "spawn_error"})


@dataclass(frozen=True)
class ChildMetrics:
    """Sanitized process evidence; paths and native handles are never retained."""

    peak_rss_bytes: int | None
    status: str
    reason: str | None = None
    tree_peak_rss_bytes: int | None = None
    pid: int | None = None
    exit_code: int | None = None
    failure_class: str = "none"
    timed_out: bool = False
    crashed: bool = False
    cleanup_status: str = "not_required"
    samples: int = 0
    tree_supported: bool = False
    reason_codes: tuple[str, ...] = field(default_factory=tuple)
    observed_pids: tuple[int, ...] = field(default_factory=tuple)

    def __post_init__(self) -> None:
        if self.status not in _STATUS_CODES:
            raise ValueError(f"unsupported child metric status: {self.status}")
        if self.failure_class not in _FAILURE_CLASSES:
            raise ValueError(f"unsupported failure class: {self.failure_class}")
        if self.peak_rss_bytes is not None and self.peak_rss_bytes < 0:
            raise ValueError("peak_rss_bytes must be non-negative")
        if self.tree_peak_rss_bytes is not None and self.tree_peak_rss_bytes < 0:
            raise ValueError("tree_peak_rss_bytes must be non-negative")
        observed_pids = {
            pid for pid in self.observed_pids
            if isinstance(pid, int) and not isinstance(pid, bool) and pid > 0
        }
        singleton_tree = (
            len(observed_pids) == 1
            and self.peak_rss_bytes is not None
            and self.tree_peak_rss_bytes == self.peak_rss_bytes
        )
        tree_valid = (
            self.tree_supported
            and self.tree_peak_rss_bytes is not None
            and observed_pids
            and len(observed_pids) == len(self.observed_pids)
            and (len(observed_pids) > 1 or singleton_tree)
        )
        if not tree_valid:
            # A partial sample is not a process-tree measurement.  A singleton
            # root is valid only when its tree total is exactly its native peak;
            # never expose a fabricated descendant or root/tree mismatch.
            object.__setattr__(self, "status", STATUS_NOT_COMPARABLE)
            object.__setattr__(self, "tree_supported", False)
            object.__setattr__(self, "tree_peak_rss_bytes", None)
            if self.reason is None:
                object.__setattr__(self, "reason", "tree_not_observed")
                codes = set(self.reason_codes)
                codes.add("tree_not_observed")
                object.__setattr__(self, "reason_codes", tuple(sorted(codes)))

    def to_dict(self) -> dict[str, object]:
        return {
            "peak_rss_bytes": self.peak_rss_bytes,
            "tree_peak_rss_bytes": self.tree_peak_rss_bytes,
            "status": self.status,
            "reason_code": self.reason,
            "pid": self.pid,
            "exit_code": self.exit_code,
            "failure_class": self.failure_class,
            "timed_out": self.timed_out,
            "crashed": self.crashed,
            "cleanup_status": self.cleanup_status,
            "samples": self.samples,
            "tree_supported": self.tree_supported,
            "reason_codes": list(self.reason_codes),
            "observed_pids": list(self.observed_pids),
        }


@dataclass(frozen=True)
class ChildRunResult:
    """Subprocess output kept in memory for the comparator, never durable by default."""

    argv: list[str]
    cwd: str
    env_keys: list[str]
    returncode: int
    stdout: str = ""
    stderr: str = ""
    elapsed_ms: float = 0.0
    timed_out: bool = False
    crashed: bool = False
    metrics: ChildMetrics | None = None
    runtime_proof: dict[str, object] | None = None


if os.name == "nt":
    _kernel32 = ctypes.WinDLL("kernel32", use_last_error=True)
    _psapi = ctypes.WinDLL("psapi", use_last_error=True)

    class _PROCESS_MEMORY_COUNTERS(ctypes.Structure):
        _fields_ = [
            ("cb", wintypes.DWORD),
            ("PageFaultCount", wintypes.DWORD),
            ("PeakWorkingSetSize", ctypes.c_size_t),
            ("WorkingSetSize", ctypes.c_size_t),
            ("QuotaPeakPagedPoolUsage", ctypes.c_size_t),
            ("QuotaPagedPoolUsage", ctypes.c_size_t),
            ("QuotaPeakNonPagedPoolUsage", ctypes.c_size_t),
            ("QuotaNonPagedPoolUsage", ctypes.c_size_t),
            ("PagefileUsage", ctypes.c_size_t),
            ("PeakPagefileUsage", ctypes.c_size_t),
        ]

    class _IO_COUNTERS(ctypes.Structure):
        _fields_ = [(name, ctypes.c_uint64) for name in (
            "ReadOperationCount", "WriteOperationCount", "OtherOperationCount",
            "ReadTransferCount", "WriteTransferCount", "OtherTransferCount",
        )]

    class _JOBOBJECT_BASIC_LIMIT_INFORMATION(ctypes.Structure):
        _fields_ = [
            ("PerProcessUserTimeLimit", ctypes.c_int64),
            ("PerJobUserTimeLimit", ctypes.c_int64),
            ("LimitFlags", wintypes.DWORD),
            ("MinimumWorkingSetSize", ctypes.c_size_t),
            ("MaximumWorkingSetSize", ctypes.c_size_t),
            ("ActiveProcessLimit", wintypes.DWORD),
            ("Affinity", ctypes.c_size_t),
            ("PriorityClass", wintypes.DWORD),
            ("SchedulingClass", wintypes.DWORD),
        ]

    class _JOBOBJECT_EXTENDED_LIMIT_INFORMATION(ctypes.Structure):
        _fields_ = [
            ("BasicLimitInformation", _JOBOBJECT_BASIC_LIMIT_INFORMATION),
            ("IoInfo", _IO_COUNTERS),
            ("ProcessMemoryLimit", ctypes.c_size_t),
            ("JobMemoryLimit", ctypes.c_size_t),
            ("PeakProcessMemoryUsed", ctypes.c_size_t),
            ("PeakJobMemoryUsed", ctypes.c_size_t),
        ]

    class _PROCESSENTRY32W(ctypes.Structure):
        _fields_ = [
            ("dwSize", wintypes.DWORD),
            ("cntUsage", wintypes.DWORD),
            ("th32ProcessID", wintypes.DWORD),
            ("th32DefaultHeapID", ctypes.c_size_t),
            ("th32ModuleID", wintypes.DWORD),
            ("cntThreads", wintypes.DWORD),
            ("th32ParentProcessID", wintypes.DWORD),
            ("pcPriClassBase", wintypes.LONG),
            ("dwFlags", wintypes.DWORD),
            # The fixed executable-name tail is required by Process32FirstW;
            # it is never read or retained as durable evidence.
            ("szExeFile", wintypes.WCHAR * 260),
        ]

    _kernel32.OpenProcess.argtypes = [wintypes.DWORD, wintypes.BOOL, wintypes.DWORD]
    _kernel32.OpenProcess.restype = wintypes.HANDLE
    _kernel32.CloseHandle.argtypes = [wintypes.HANDLE]
    _kernel32.CloseHandle.restype = wintypes.BOOL
    _psapi.GetProcessMemoryInfo.argtypes = [wintypes.HANDLE, ctypes.POINTER(_PROCESS_MEMORY_COUNTERS), wintypes.DWORD]
    _psapi.GetProcessMemoryInfo.restype = wintypes.BOOL
    _kernel32.QueryFullProcessImageNameW.argtypes = [wintypes.HANDLE, wintypes.DWORD, wintypes.LPWSTR, ctypes.POINTER(wintypes.DWORD)]
    _kernel32.QueryFullProcessImageNameW.restype = wintypes.BOOL
    _kernel32.CreateToolhelp32Snapshot.argtypes = [wintypes.DWORD, wintypes.DWORD]
    _kernel32.CreateToolhelp32Snapshot.restype = wintypes.HANDLE
    _kernel32.Process32FirstW.argtypes = [wintypes.HANDLE, ctypes.POINTER(_PROCESSENTRY32W)]
    _kernel32.Process32FirstW.restype = wintypes.BOOL
    _kernel32.Process32NextW.argtypes = [wintypes.HANDLE, ctypes.POINTER(_PROCESSENTRY32W)]
    _kernel32.Process32NextW.restype = wintypes.BOOL
    _kernel32.CreateJobObjectW.argtypes = [wintypes.LPVOID, wintypes.LPCWSTR]
    _kernel32.CreateJobObjectW.restype = wintypes.HANDLE
    _kernel32.SetInformationJobObject.argtypes = [wintypes.HANDLE, wintypes.INT, wintypes.LPVOID, wintypes.DWORD]
    _kernel32.SetInformationJobObject.restype = wintypes.BOOL
    _kernel32.AssignProcessToJobObject.argtypes = [wintypes.HANDLE, wintypes.HANDLE]
    _kernel32.AssignProcessToJobObject.restype = wintypes.BOOL

    _TH32CS_SNAPPROCESS = 0x00000002
    _PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
    _PROCESS_VM_READ = 0x0010
    _JOB_OBJECT_EXTENDED_LIMIT_INFORMATION = 9
    _JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE = 0x2000
    _INVALID_HANDLE_VALUE = ctypes.c_void_p(-1).value


def _observe_child_processes(pids: set[int], env_keys: Sequence[str]) -> Mapping[int, Mapping[str, object]]:
    """Observe the live tree through the same native Windows boundary as RSS.

    CIM/PowerShell startup is slower than short-lived benchmark children and can
    turn a successful tree sample into a false unavailable result.  Image paths
    from ``QueryFullProcessImageNameW`` are sufficient for the closed MCP
    executable-name scan; command output and the root argv are checked
    separately by ``RuntimeProofProbe``.
    """
    probe = _WindowsProbe()
    if not probe.available:
        return {}
    result: dict[int, Mapping[str, object]] = {}
    for pid in sorted(pids):
        image = probe.image_path(pid)
        if image:
            result[pid] = {
                "argv": "",
                "image": image,
                "env_keys": tuple(str(key) for key in env_keys),
            }
    return result


def _decode(value: object) -> str:
    if isinstance(value, bytes):
        return value.decode(errors="replace")
    return str(value or "")


def _sha256_argv(argv: Sequence[str]) -> str:
    return hashlib.sha256("\0".join(str(x) for x in argv).encode("utf-8", "replace")).hexdigest()


class _WindowsProbe:
    def __init__(self) -> None:
        self.available = os.name == "nt"
        self.reason = None if self.available else "unsupported_platform"
        if self.available:
            try:
                # A harmless symbol check makes missing/partial Windows API support explicit.
                self.available = bool(_psapi.GetProcessMemoryInfo and _kernel32.QueryFullProcessImageNameW)
            except AttributeError:
                self.available = False
                self.reason = "windows_counter_api_unavailable"
            if not self.available and self.reason is None:
                self.reason = "windows_counter_api_unavailable"

    def _open(self, pid: int) -> wintypes.HANDLE | None:
        handle = _kernel32.OpenProcess(_PROCESS_QUERY_LIMITED_INFORMATION | _PROCESS_VM_READ, False, pid)
        return handle if handle else None

    def working_set(self, pid: int) -> int | None:
        handle = self._open(pid)
        if not handle:
            return None
        try:
            counters = _PROCESS_MEMORY_COUNTERS()
            counters.cb = ctypes.sizeof(counters)
            if not _psapi.GetProcessMemoryInfo(handle, ctypes.byref(counters), counters.cb):
                return None
            return int(counters.WorkingSetSize)
        finally:
            _kernel32.CloseHandle(handle)

    def image_path(self, pid: int) -> str | None:
        handle = self._open(pid)
        if not handle:
            return None
        try:
            buffer = ctypes.create_unicode_buffer(32768)
            length = wintypes.DWORD(len(buffer))
            if _kernel32.QueryFullProcessImageNameW(handle, 0, buffer, ctypes.byref(length)):
                return buffer.value[: length.value]
            return None
        finally:
            _kernel32.CloseHandle(handle)

    def descendants(self, root_pid: int) -> set[int]:
        snapshot = _kernel32.CreateToolhelp32Snapshot(_TH32CS_SNAPPROCESS, 0)
        if not snapshot or snapshot == _INVALID_HANDLE_VALUE:
            return set()
        try:
            entry = _PROCESSENTRY32W()
            entry.dwSize = ctypes.sizeof(entry)
            parent_map: dict[int, int] = {}
            if _kernel32.Process32FirstW(snapshot, ctypes.byref(entry)):
                while True:
                    parent_map[int(entry.th32ProcessID)] = int(entry.th32ParentProcessID)
                    if not _kernel32.Process32NextW(snapshot, ctypes.byref(entry)):
                        break
            result: set[int] = set()
            frontier = {root_pid}
            while frontier:
                children = {pid for pid, parent in parent_map.items() if parent in frontier}
                children -= result
                result.update(children)
                frontier = children
            return result
        finally:
            _kernel32.CloseHandle(snapshot)

    def create_job(self) -> wintypes.HANDLE | None:
        if not self.available:
            return None
        job = _kernel32.CreateJobObjectW(None, None)
        if not job:
            return None
        limits = _JOBOBJECT_EXTENDED_LIMIT_INFORMATION()
        limits.BasicLimitInformation.LimitFlags = _JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
        if not _kernel32.SetInformationJobObject(job, _JOB_OBJECT_EXTENDED_LIMIT_INFORMATION, ctypes.byref(limits), ctypes.sizeof(limits)):
            _kernel32.CloseHandle(job)
            return None
        return job

    def assign_job(self, job: wintypes.HANDLE, process_handle: wintypes.HANDLE) -> bool:
        return bool(_kernel32.AssignProcessToJobObject(job, process_handle))

    def close_job(self, job: wintypes.HANDLE | None) -> None:
        if job:
            _kernel32.CloseHandle(job)


class _Sampler:
    def __init__(self, pid: int, probe: _WindowsProbe, interval: float) -> None:
        self.pid = pid
        self.probe = probe
        self.interval = max(0.001, interval)
        self.stop = threading.Event()
        self.thread = threading.Thread(target=self._run, name="victory-child-sampler", daemon=True)
        self.root_peak: int | None = None
        self.tree_peak: int | None = None
        self.samples = 0
        self.tree_supported = False
        self._tree_partial = False
        self._reason_codes: set[str] = set()
        self.observed_pids: set[int] = {pid}
        self._descendants_observed: set[int] = set()

    def start(self) -> None:
        self.thread.start()

    def _sample(self) -> None:
        root = self.probe.working_set(self.pid)
        raw_descendants = self.probe.descendants(self.pid) or ()
        descendants = {
            descendant for descendant in raw_descendants
            if isinstance(descendant, int)
            and not isinstance(descendant, bool)
            and descendant > 0
            and descendant != self.pid
        }
        pids = {self.pid} | descendants
        self.observed_pids.update(pids)
        self._descendants_observed.update(descendants)

        values_by_pid = {self.pid: root}
        for descendant in descendants:
            values_by_pid[descendant] = self.probe.working_set(descendant)
        valid_values = {
            pid: value for pid, value in values_by_pid.items()
            if isinstance(value, int) and not isinstance(value, bool) and value >= 0
        }
        if self.pid in valid_values:
            self.root_peak = max(self.root_peak or 0, valid_values[self.pid])
            self.samples += 1
        else:
            self._reason_codes.add("working_set_unavailable")

        if len(pids) > 1:
            if len(valid_values) != len(pids) or self._tree_partial:
                # Once any observed descendant lacks a native working-set
                # value, the tree evidence is permanently partial. Do not
                # recover to PASS from a later complete sample.
                self._tree_partial = True
                self.tree_supported = False
                self.tree_peak = None
                self._reason_codes.add("working_set_unavailable")
            else:
                total = sum(valid_values.values())
                self.tree_peak = max(self.tree_peak or 0, total)
                self.tree_supported = self.tree_peak is not None
        elif self._descendants_observed:
            # A descendant observed in an earlier sample remains covered by the
            # already accumulated peak; do not turn its later exit into a fake
            # singleton tree or invent a missing counter.
            pass
        elif self.pid in valid_values:
            # A native root sample is complete coverage for a tree containing
            # only that process. This is a singleton tree, not a fabricated
            # child measurement.
            self.tree_peak = max(self.tree_peak or 0, valid_values[self.pid])
            self.tree_supported = self.tree_peak is not None

    def _run(self) -> None:
        while not self.stop.is_set():
            self._sample()
            self.stop.wait(self.interval)
        self._sample()

    def finish(self) -> None:
        self.stop.set()
        self.thread.join(timeout=2.0)

    def metrics(
        self, *, pid: int, returncode: int, timed_out: bool, crashed: bool,
        cleanup_status: str, spawn_error: bool = False,
    ) -> ChildMetrics:
        reason_codes = set(self._reason_codes)
        if self.root_peak is None:
            reason_codes.add("working_set_unavailable")
            status = STATUS_NOT_COMPARABLE
            reason = "working_set_unavailable"
        elif self._tree_partial:
            # A partial tree observation is not comparable, even when a later
            # sample happened to recover the missing counter.
            reason_codes.add("working_set_unavailable")
            status = STATUS_NOT_COMPARABLE
            reason = "working_set_unavailable"
        elif not self.tree_supported or self.tree_peak is None:
            reason_codes.add("tree_not_observed")
            status = STATUS_NOT_COMPARABLE
            reason = "tree_not_observed"
        else:
            status = "PASS"
            reason = None
        tree_peak = self.tree_peak if status == "PASS" else None
        return ChildMetrics(
            peak_rss_bytes=self.root_peak, tree_peak_rss_bytes=tree_peak,
            status=status, reason=reason, pid=pid, exit_code=returncode,
            failure_class=_failure_class(returncode=returncode, timed_out=timed_out, crashed=crashed, spawn_error=spawn_error),
            timed_out=timed_out, crashed=crashed, cleanup_status=cleanup_status,
            samples=self.samples, tree_supported=status == "PASS",
            reason_codes=tuple(sorted(reason_codes)), observed_pids=tuple(sorted(self.observed_pids)),
        )


def _is_crash_returncode(returncode: int) -> bool:
    if returncode < 0:
        return True
    return os.name == "nt" and (returncode & 0xFFFFFFFF) >= 0xC0000000


def _failure_class(*, returncode: int, timed_out: bool, crashed: bool, spawn_error: bool = False) -> str:
    if spawn_error:
        return "spawn_error"
    if timed_out:
        return "timeout"
    if crashed:
        return "crash"
    return "exit_nonzero" if returncode else "none"


def _terminate_tree(process: subprocess.Popen[bytes], probe: _WindowsProbe, job: object | None) -> str:
    """Terminate only the child boundary; never issue a broad system kill."""
    try:
        if job is not None:
            probe.close_job(job)  # kill-on-close terminates all assigned descendants.
        else:
            process.kill()
            if os.name == "nt":
                subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], capture_output=True, check=False)
        process.wait(timeout=2.0)
        return "clean"
    except (OSError, subprocess.TimeoutExpired):
        try:
            process.kill()
            process.wait(timeout=2.0)
            return "forced"
        except (OSError, subprocess.TimeoutExpired):
            return "failed"


def run_child(
    argv: Sequence[str], *, cwd: Path, env: Mapping[str, str], timeout_seconds: float, sample_interval: float = 0.01,
) -> ChildRunResult:
    """Run without a shell, sample the child, and classify termination honestly."""
    started = time.perf_counter()
    command = [str(item) for item in argv]
    probe = _WindowsProbe()
    job = None
    process: subprocess.Popen[bytes] | None = None
    sampler: _Sampler | None = None
    runtime_probe: RuntimeProofProbe | None = None
    timed_out = False
    cleanup_status = "not_required"
    stdout = stderr = ""
    returncode = 127
    crashed = False
    spawn_error = False
    runtime_observation: dict[str, object] | None = None
    try:
        creationflags = subprocess.CREATE_NEW_PROCESS_GROUP if os.name == "nt" else 0
        process = subprocess.Popen(
            command, cwd=str(cwd), env=dict(env), stdout=subprocess.PIPE, stderr=subprocess.PIPE,
            text=False, shell=False, creationflags=creationflags,
        )
        if probe.available:
            job = probe.create_job()
            if job is not None:
                process_handle = getattr(process, "_handle", None)
                if process_handle is None or not probe.assign_job(job, process_handle):
                    probe.close_job(job)
                    job = None
        if probe.available:
            sampler = _Sampler(process.pid, probe, sample_interval)
            sampler.start()
        runtime_probe = RuntimeProofProbe(
            process.pid,
            pid_provider=(lambda: set(sampler.observed_pids)) if sampler is not None else None,
            process_observer=(lambda pids: _observe_child_processes(pids, env.keys())) if sampler is not None else None,
            provenance="child_metrics_executor" if sampler is not None else None,
        )
        runtime_probe.start()
        try:
            out_bytes, err_bytes = process.communicate(timeout=timeout_seconds)
            stdout, stderr = _decode(out_bytes), _decode(err_bytes)
            returncode = int(process.returncode)
        except subprocess.TimeoutExpired as exc:
            timed_out = True
            stdout, stderr = _decode(exc.stdout), _decode(exc.stderr)
            cleanup_status = _terminate_tree(process, probe, job)
            job = None
            out_bytes, err_bytes = process.communicate()
            stdout += _decode(out_bytes)
            stderr += _decode(err_bytes)
            # Timeout is a harness-owned termination class; do not misreport the
            # Job Object's post-kill code as a successful child exit.
            returncode = 124
        crashed = _is_crash_returncode(returncode)
    except OSError as exc:
        stderr = f"{type(exc).__name__}"
        returncode = 127
        crashed = True
        spawn_error = True
        cleanup_status = "not_required"
    finally:
        # Stop runtime observation before the sampler's final post-exit sample.
        # The sampler intentionally retains observed descendants; allowing it
        # to add PIDs after runtime proof stops would create metadata gaps for
        # processes that were never live during a proof sample.
        if runtime_probe is not None:
            runtime_observation = runtime_probe.finish(stdout=stdout, stderr=stderr)
        if sampler is not None:
            sampler.finish()
        if job is not None:
            probe.close_job(job)
        if process is not None and process.poll() is None:
            cleanup_status = _terminate_tree(process, probe, None)
    if runtime_observation is None:
        if runtime_probe is not None:
            runtime_observation = runtime_probe.finish(stdout=stdout, stderr=stderr)
        else:
            runtime_observation = {
                "status": STATUS_NOT_COMPARABLE, "runtime_proof": False,
                "reason_code": probe.reason or "runtime_proof_unavailable",
            }
    if sampler is None:
        metrics = ChildMetrics(
            peak_rss_bytes=None, tree_peak_rss_bytes=None, status=STATUS_NOT_COMPARABLE,
            reason=probe.reason or "counter_unavailable", pid=getattr(process, "pid", None),
            exit_code=returncode, failure_class=_failure_class(returncode=returncode, timed_out=timed_out, crashed=crashed, spawn_error=spawn_error),
            timed_out=timed_out, crashed=crashed, cleanup_status=cleanup_status,
            reason_codes=(probe.reason or "counter_unavailable",),
        )
    else:
        metrics = sampler.metrics(
            pid=process.pid, returncode=returncode, timed_out=timed_out, crashed=crashed,
            cleanup_status=cleanup_status,
        )
    return ChildRunResult(
        argv=command, cwd=str(cwd), env_keys=sorted(str(key) for key in env), returncode=returncode,
        stdout=stdout, stderr=stderr, elapsed_ms=(time.perf_counter() - started) * 1000,
        timed_out=timed_out, crashed=crashed, metrics=metrics, runtime_proof=runtime_observation,
    )


class ChildMetricsExecutor:
    """Adapter-compatible executor that uses :func:`run_child`."""

    def run(self, argv: list[str], *, cwd: Path, env: Mapping[str, str], timeout_seconds: float) -> ChildRunResult:
        return run_child(argv, cwd=cwd, env=env, timeout_seconds=timeout_seconds)


__all__ = ["ChildMetrics", "ChildRunResult", "ChildMetricsExecutor", "run_child"]
