"""No-write integrity and provenance-backed Windows runtime security gate."""
from __future__ import annotations

from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import threading
from typing import Callable, Mapping, Sequence

try:
    from .sanitize_v2 import bounded_reason, digest_text
except ImportError:  # pragma: no cover
    from sanitize_v2 import bounded_reason, digest_text

_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_NETWORK_RE = re.compile(r"(?i)(https?://|ftp://|\\\\|\b(socket|requests|urllib|curl|wget|nc|netcat|network|webclient|invoke-webrequest)\b)")
_MCP_RE = re.compile(r"(?i)\bmcp\b|model.context.protocol")
_SECRET_RE = re.compile(r"(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key|authorization|credential|cookie)")
_RUNTIME_REASON_CODES = frozenset({
    "runtime_proof_unavailable", "root_metadata_missing", "observer_race",
    "network_indicator", "mcp_indicator", "metadata_missing", "metadata_mismatch",
    "unsupported_platform", "counter_unavailable", "working_set_unavailable",
})


def _canonical_runtime_pids(value: object) -> list[int]:
    if not isinstance(value, (list, tuple, set)):
        return []
    return sorted({pid for pid in value if isinstance(pid, int) and not isinstance(pid, bool) and pid > 0})


def _valid_runtime_pid_list(value: object, *, nonempty: bool) -> bool:
    return (
        isinstance(value, list)
        and (bool(value) or not nonempty)
        and all(isinstance(pid, int) and not isinstance(pid, bool) and pid > 0 for pid in value)
        and len(set(value)) == len(value)
    )


def runtime_evidence_digest(envelope: Mapping[str, object]) -> str:
    """Digest the bounded runtime-proof envelope, never raw process diagnostics."""
    network_count = envelope.get("network_count", envelope.get("observed_network_count"))
    mcp_count = envelope.get("mcp_count", envelope.get("observed_mcp_count"))
    reason = envelope.get("reason") if "reason" in envelope else envelope.get("reason_code")
    canonical = {
        "provenance": envelope.get("provenance"),
        "probe_mode": envelope.get("probe_mode"),
        "observed_pids": _canonical_runtime_pids(envelope.get("observed_pids")),
        "metadata_observed_pids": _canonical_runtime_pids(envelope.get("metadata_observed_pids")),
        "sample_count": envelope.get("sample_count"),
        "network_count": network_count,
        "mcp_count": mcp_count,
        "status": envelope.get("status"),
        "runtime_proof": envelope.get("runtime_proof"),
        "reason": reason,
    }
    return hashlib.sha256(json.dumps(canonical, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")).hexdigest()


def _sanitize_runtime_reason(value: object) -> str:
    candidate = str(value or "").strip().lower()
    return candidate if candidate in _RUNTIME_REASON_CODES else "runtime_proof_unavailable"


def _safe_counter(value: object) -> int:
    return value if isinstance(value, int) and not isinstance(value, bool) and value >= 0 else 0


def _safe_pid_set(value: object) -> set[int] | None:
    if not isinstance(value, (list, tuple, set)):
        return None
    if any(not isinstance(pid, int) or isinstance(pid, bool) or pid <= 0 for pid in value):
        return None
    return set(value)


def _runtime_reason(observation: Mapping[str, object]) -> str:
    observed = _safe_pid_set(observation.get("observed_pids"))
    metadata = _safe_pid_set(observation.get("metadata_observed_pids"))
    if observed is not None and (metadata is None or not observed <= metadata):
        return "metadata_missing"
    return _sanitize_runtime_reason(observation.get("reason", observation.get("reason_code")))


def _runtime_digest_matches(observation: Mapping[str, object]) -> bool:
    try:
        return runtime_evidence_digest(observation) == observation.get("evidence_digest")
    except (TypeError, ValueError):
        return False


@dataclass(frozen=True)
class IntegritySnapshot:
    """Only path IDs and content digests are retained."""

    paths: dict[str, str]

    def to_dict(self) -> dict[str, object]:
        return {"paths": dict(sorted(self.paths.items()))}


def _file_digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _tree_digest(path: Path) -> str:
    digest = hashlib.sha256()
    if path.is_file():
        return _file_digest(path)
    if not path.is_dir():
        return digest.hexdigest()
    for item in sorted(path.rglob("*"), key=lambda candidate: candidate.relative_to(path).as_posix()):
        relative = item.relative_to(path).as_posix().encode("utf-8")
        digest.update(relative)
        if item.is_file():
            digest.update(_file_digest(item).encode("ascii"))
    return digest.hexdigest()


def _git_head(source: Path) -> str:
    try:
        result = subprocess.run(
            ["git", "-C", str(source), "rev-parse", "HEAD"], capture_output=True, text=True,
            check=False, timeout=10, env={"PATH": os.environ.get("PATH", "")},
        )
    except (OSError, subprocess.TimeoutExpired) as exc:
        raise OSError("source HEAD snapshot unavailable") from exc
    if result.returncode != 0 or not re.fullmatch(r"[0-9a-f]{40}", result.stdout.strip()):
        raise OSError("source HEAD snapshot unavailable")
    return result.stdout.strip()


def snapshot_source_inputs(source_inputs: Mapping[str, tuple[str | os.PathLike[str], Sequence[str | os.PathLike[str]]]]) -> IntegritySnapshot:
    """Snapshot only pinned HEAD plus explicit build/import input paths."""
    result: dict[str, str] = {}
    for label, (raw_root, raw_paths) in sorted(source_inputs.items()):
        root = Path(raw_root)
        result[digest_text(f"{label}:HEAD")] = digest_text(_git_head(root))
        for raw_path in raw_paths:
            path = Path(raw_path)
            absolute = path if path.is_absolute() else root / path
            result[digest_text(f"{label}:{path.as_posix()}")] = _tree_digest(absolute)
    return IntegritySnapshot(result)


def snapshot_paths(paths: Mapping[str, str | os.PathLike[str]] | Sequence[str | os.PathLike[str]]) -> IntegritySnapshot:
    """Hash configured fixture/protected paths before or after execution."""
    if isinstance(paths, Mapping):
        items = paths.items()
    else:
        items = ((digest_text(Path(path).resolve().as_posix()), path) for path in paths)
    result: dict[str, str] = {}
    for raw_id, raw_path in items:
        path_id = bounded_reason(raw_id, digest_text(str(raw_id))) if str(raw_id).isidentifier() else digest_text(str(raw_id))
        result[path_id] = _tree_digest(Path(raw_path))
    return IntegritySnapshot(result)


def compare_snapshots(before: IntegritySnapshot, after: IntegritySnapshot) -> dict[str, object]:
    changed = sorted(path_id for path_id in set(before.paths) | set(after.paths) if before.paths.get(path_id) != after.paths.get(path_id))
    return {"status": "PASS" if not changed else "FAIL", "changed_path_ids": changed, "reason_code": None if not changed else "protected_path_changed"}


def scan_command_env(argv: Sequence[object], env: Mapping[str, object]) -> dict[str, object]:
    """Static/advisory scan only; it is never runtime proof of absence."""
    command_text = "\0".join(str(item) for item in argv)
    env_names = "\0".join(str(key) for key in env)
    env_values = "\0".join(str(value) for value in env.values())
    findings: set[str] = set()
    if _NETWORK_RE.search(command_text) or _NETWORK_RE.search(env_values):
        findings.add("network_indicator")
    if _MCP_RE.search(command_text) or _MCP_RE.search(env_names):
        findings.add("mcp_indicator")
    if _SECRET_RE.search(command_text) or _SECRET_RE.search(env_names) or _SECRET_RE.search(env_values):
        findings.add("secret_indicator")
    return {
        "scan_mode": "static_advisory",
        "runtime_proof": False,
        "status": "FAIL" if findings else "PASS",
        "reason_codes": sorted(findings),
    }


def _observable_processes(pids: set[int], env_keys: Sequence[str]) -> Mapping[int, Mapping[str, object]]:
    """Read only observable command/image metadata, never process env values."""
    if os.name != "nt":
        return {}
    shell = shutil.which("powershell") or shutil.which("pwsh")
    if not shell:
        return {}
    script = (
        "$ids=@(" + ",".join(str(pid) for pid in sorted(pids)) + "); "
        "Get-CimInstance Win32_Process | Where-Object {$ids -contains $_.ProcessId} | "
        "Select-Object ProcessId,Name,CommandLine | ConvertTo-Json -Compress"
    )
    try:
        completed = subprocess.run(
            [shell, "-NoProfile", "-NonInteractive", "-Command", script], capture_output=True,
            text=True, check=False, timeout=2, creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
        )
        if completed.returncode != 0 or not completed.stdout.strip():
            return {}
        decoded = json.loads(completed.stdout)
        rows = decoded if isinstance(decoded, list) else [decoded]
        result: dict[int, Mapping[str, object]] = {}
        for row in rows:
            if isinstance(row, Mapping) and str(row.get("ProcessId", "")).isdigit():
                result[int(row["ProcessId"])] = {
                    "argv": str(row.get("CommandLine") or ""),
                    "image": str(row.get("Name") or ""),
                    "env_keys": tuple(str(key) for key in env_keys),
                }
        return result
    except (OSError, subprocess.TimeoutExpired, ValueError, json.JSONDecodeError):
        return {}


class RuntimeProofProbe:
    """Observe every PID supplied by ChildMetricsExecutor while it lives."""

    def __init__(
        self,
        pid: int,
        *,
        pid_provider: Callable[[], set[int]] | None = None,
        process_observer: Callable[[set[int]], Mapping[int, Mapping[str, object]]] | None = None,
        provenance: str | None = None,
        interval: float = 0.02,
    ) -> None:
        self.pid = int(pid)
        self.interval = max(0.005, interval)
        self.pid_provider = pid_provider
        self.process_observer = process_observer
        self.provenance = provenance
        self.available = (
            os.name == "nt" and shutil.which("netstat") is not None
            and pid_provider is not None and process_observer is not None
            and provenance == "child_metrics_executor"
        )
        self.reason = None if self.available else "runtime_proof_unavailable"
        self._stop = threading.Event()
        self._thread: threading.Thread | None = None
        self._snapshots: set[str] = set()
        self._mcp_indicators: set[tuple[int, str]] = set()
        self.observed_pids: set[int] = set()
        self.metadata_observed_pids: set[int] = set()
        self._metadata_incomplete = False
        self.samples = 0
        self.probe_errors = 0
        self._reason_code = self.reason

    def _sample(self) -> None:
        if not self.available or self.pid_provider is None or self.process_observer is None:
            return
        try:
            pids = {int(pid) for pid in self.pid_provider() if int(pid) > 0}
            if self.pid not in pids:
                # The child can disappear between the PID and metadata reads.
                # This is a bounded observer race, not raw probe diagnostics.
                if self.samples == 0:
                    self._reason_code = "observer_race"
                return
            self.observed_pids.update(pids)

            # Metadata is deliberately acquired before netstat.  A network
            # listing without a live root identity is not a valid sample.
            raw_observations = self.process_observer(pids)
            normalized_observations: dict[int, Mapping[str, object]] = {}
            if isinstance(raw_observations, Mapping):
                for observed_pid, item in raw_observations.items():
                    try:
                        normalized_pid = int(observed_pid)
                    except (TypeError, ValueError):
                        continue
                    if normalized_pid in pids and isinstance(item, Mapping):
                        normalized_observations[normalized_pid] = item
            root_metadata = normalized_observations.get(self.pid)
            if root_metadata is None:
                if self.samples == 0:
                    self._reason_code = "root_metadata_missing"
                else:
                    self._metadata_incomplete = True
                    self._reason_code = "metadata_missing"
                return
            missing_metadata = pids - set(normalized_observations)
            self._metadata_incomplete = bool(missing_metadata)
            if missing_metadata:
                # A child omitted from metadata is not evidence that the child
                # was safe or absent.  The final set comparison below remains
                # fail-closed if that PID is still missing at finish.
                self._reason_code = "metadata_missing"

            completed = subprocess.run(
                ["netstat", "-ano"], capture_output=True, text=True, check=False, timeout=2,
                creationflags=getattr(subprocess, "CREATE_NO_WINDOW", 0),
            )
            if completed.returncode != 0:
                self.probe_errors += 1
                self._reason_code = "runtime_proof_unavailable"
                return

            self.samples += 1
            if not self._metadata_incomplete:
                self._reason_code = None
            for line in completed.stdout.splitlines():
                fields = line.split()
                if fields and fields[-1].isdigit() and int(fields[-1]) in pids:
                    self._snapshots.add(digest_text(line))
            for normalized_pid, item in normalized_observations.items():
                self.metadata_observed_pids.add(normalized_pid)
                text = "\0".join(str(item.get(key, "")) for key in ("argv", "image", "env_keys"))
                if _MCP_RE.search(text):
                    self._mcp_indicators.add((normalized_pid, "mcp_indicator"))
        except Exception:
            # Observer failures are diagnostic-only and must never escape as
            # durable text or become a new reason taxonomy entry.
            self.probe_errors += 1
            self._reason_code = "observer_race"

    def start(self) -> None:
        if not self.available:
            return
        # Establish one observation in the caller before returning.  This
        # closes the launch/exit window where a short-lived child could finish
        # before the background observer ever sampled it.
        self._sample()
        self._thread = threading.Thread(target=self._run, name="victory-runtime-proof", daemon=True)
        self._thread.start()

    def _run(self) -> None:
        while not self._stop.is_set():
            self._sample()
            self._stop.wait(self.interval)

    def finish(self, *, stdout: str = "", stderr: str = "") -> dict[str, object]:
        if self._thread is not None:
            self._stop.set()
            self._thread.join(timeout=2)
            # Do not sample after the child has exited: the executor's PID
            # set intentionally retains observed descendants for evidence, so
            # a post-exit observer lookup would manufacture probe errors.
        if _MCP_RE.search(stdout) or _MCP_RE.search(stderr):
            self._mcp_indicators.add((self.pid, "mcp_indicator"))
        missing_metadata = self.observed_pids - self.metadata_observed_pids
        if (
            not self.available
            or self.probe_errors
            or self.samples == 0
            or not self.observed_pids
            or self.pid not in self.metadata_observed_pids
            or missing_metadata
        ):
            status = "NOT_COMPARABLE"
            if self.probe_errors or self.samples == 0 or not self.observed_pids:
                reason = _sanitize_runtime_reason(self._reason_code)
            else:
                reason = "metadata_missing"
        elif self._snapshots:
            status = "FAIL"
            reason = "network_indicator"
        elif self._mcp_indicators:
            status = "FAIL"
            reason = "mcp_indicator"
        else:
            status = "PASS"
            reason = None
        evidence = {
            "status": status,
            "runtime_proof": status == "PASS",
            "probe_mode": "windows_netstat_child_tree_observation",
            "provenance": self.provenance,
            "observed_pids": sorted(self.observed_pids),
            "metadata_observed_pids": sorted(self.metadata_observed_pids),
            "sample_count": self.samples,
            "network_count": len(self._snapshots),
            "mcp_count": len(self._mcp_indicators),
            "observed_network_count": len(self._snapshots),
            "observed_mcp_count": len(self._mcp_indicators),
            "reason": reason,
            "reason_code": reason,
        }
        evidence["evidence_digest"] = runtime_evidence_digest(evidence)
        return evidence


class SecurityGate:
    """Configurable before/after no-write gate with honest scan semantics."""

    def __init__(self, protected_paths: Mapping[str, str | os.PathLike[str]] | Sequence[str | os.PathLike[str]] = (), *, source_inputs: Mapping[str, tuple[str | os.PathLike[str], Sequence[str | os.PathLike[str]]]] | None = None) -> None:
        self.protected_paths = protected_paths
        self.source_inputs = source_inputs or {}
        self.before: IntegritySnapshot | None = None
        self.source_before: IntegritySnapshot | None = None

    def start(self, argv: Sequence[object] = (), env: Mapping[str, object] | None = None) -> dict[str, object]:
        self.before = snapshot_paths(self.protected_paths)
        self.source_before = snapshot_source_inputs(self.source_inputs) if self.source_inputs else IntegritySnapshot({})
        return {"integrity_before": self.before.to_dict(), "advisory_scan": scan_command_env(argv, env or {})}

    def finish(
        self, argv: Sequence[object] = (), env: Mapping[str, object] | None = None,
        runtime_observation: Mapping[str, object] | None = None,
    ) -> dict[str, object]:
        if self.before is None:
            raise RuntimeError("security gate was not started")
        after = snapshot_paths(self.protected_paths)
        source_after = snapshot_source_inputs(self.source_inputs) if self.source_inputs else IntegritySnapshot({})
        integrity = compare_snapshots(self.before, after)
        source_integrity = compare_snapshots(self.source_before or IntegritySnapshot({}), source_after)
        if source_integrity["status"] != "PASS":
            integrity = {"status": "FAIL", "changed_path_ids": sorted(set(integrity["changed_path_ids"]) | set(source_integrity["changed_path_ids"])), "reason_code": "protected_path_changed"}
        scan = scan_command_env(argv, env or {})
        status = str(integrity["status"])
        advisory_codes = set(scan.get("reason_codes", []))
        if {"network_indicator", "mcp_indicator"} & advisory_codes:
            status = "NOT_COMPARABLE"
        elif "secret_indicator" in advisory_codes and status == "PASS":
            status = "BLOCKED"
        observation = dict(runtime_observation or {})
        observed_pids = _safe_pid_set(observation.get("observed_pids"))
        metadata_observed_pids = _safe_pid_set(observation.get("metadata_observed_pids"))
        complete_runtime = (
            observation.get("status") in {"PASS", "FAIL"}
            and observation.get("provenance") == "child_metrics_executor"
            and isinstance(observation.get("probe_mode"), str) and bool(observation.get("probe_mode"))
            and isinstance(observation.get("runtime_proof"), bool)
            and isinstance(observation.get("sample_count"), int) and not isinstance(observation.get("sample_count"), bool) and observation.get("sample_count", 0) > 0
            and isinstance(observation.get("network_count"), int) and not isinstance(observation.get("network_count"), bool) and observation.get("network_count", -1) >= 0
            and isinstance(observation.get("mcp_count"), int) and not isinstance(observation.get("mcp_count"), bool) and observation.get("mcp_count", -1) >= 0
            and _valid_runtime_pid_list(observation.get("observed_pids"), nonempty=True)
            and _valid_runtime_pid_list(observation.get("metadata_observed_pids"), nonempty=False)
            and observed_pids is not None and bool(observed_pids)
            and metadata_observed_pids is not None
            and observed_pids <= metadata_observed_pids
            and "reason" in observation
            and (observation.get("reason") is None or observation.get("reason") in _RUNTIME_REASON_CODES)
            and isinstance(observation.get("evidence_digest"), str) and _SHA256_RE.fullmatch(observation["evidence_digest"])
            and _runtime_digest_matches(observation)
        )
        if not complete_runtime:
            # Preserve bounded diagnostic information when the envelope itself
            # is incomplete.  A fallback must not erase the observer's reason
            # or turn already observed counters into zeros.
            fallback: dict[str, object] = {
                "status": "NOT_COMPARABLE", "runtime_proof": False,
                "provenance": observation.get("provenance") if isinstance(observation.get("provenance"), str) else None,
                "probe_mode": observation.get("probe_mode") if isinstance(observation.get("probe_mode"), str) else "runtime_proof_unavailable",
                "reason": _runtime_reason(observation),
                "reason_code": _runtime_reason(observation),
                "sample_count": _safe_counter(observation.get("sample_count")),
                "network_count": _safe_counter(observation.get("network_count", observation.get("observed_network_count"))),
                "mcp_count": _safe_counter(observation.get("mcp_count", observation.get("observed_mcp_count"))),
                "observed_network_count": _safe_counter(observation.get("network_count", observation.get("observed_network_count"))),
                "observed_mcp_count": _safe_counter(observation.get("mcp_count", observation.get("observed_mcp_count"))),
            }
            if "probe_errors" in observation:
                fallback["probe_errors"] = _safe_counter(observation.get("probe_errors"))
            raw_pids = observation.get("observed_pids")
            if isinstance(raw_pids, (list, tuple, set)):
                fallback["observed_pids"] = sorted({pid for pid in raw_pids if isinstance(pid, int) and not isinstance(pid, bool) and pid > 0})
            raw_metadata_pids = observation.get("metadata_observed_pids")
            if isinstance(raw_metadata_pids, (list, tuple, set)):
                fallback["metadata_observed_pids"] = sorted({pid for pid in raw_metadata_pids if isinstance(pid, int) and not isinstance(pid, bool) and pid > 0})
            else:
                fallback["metadata_observed_pids"] = []
            fallback["evidence_digest"] = runtime_evidence_digest(fallback)
            observation = fallback
            status = "NOT_COMPARABLE"
        elif observation["status"] == "FAIL":
            status = "BLOCKED"
        elif status == "PASS" and observation.get("runtime_proof") is not True:
            status = "NOT_COMPARABLE"
        if status == "PASS" and (observation.get("network_count", observation.get("observed_network_count", 0)) or observation.get("mcp_count", observation.get("observed_mcp_count", 0))):
            status = "BLOCKED"
        if integrity.get("status") != "PASS":
            status = "BLOCKED"
        return {
            "status": status,
            "integrity_after": after.to_dict(),
            "integrity": integrity,
            "source_integrity": source_integrity,
            "advisory_scan": scan,
            "runtime": observation,
            "runtime_proof": status == "PASS" and observation.get("runtime_proof") is True,
        }


def run_security_gate(
    argv: Sequence[object], env: Mapping[str, object], protected_paths: Mapping[str, str | os.PathLike[str]] | Sequence[str | os.PathLike[str]] = (),
) -> dict[str, object]:
    gate = SecurityGate(protected_paths)
    gate.start(argv, env)
    return gate.finish(argv, env)


__all__ = ["IntegritySnapshot", "RuntimeProofProbe", "SecurityGate", "compare_snapshots", "run_security_gate", "runtime_evidence_digest", "scan_command_env", "snapshot_paths", "snapshot_source_inputs"]
