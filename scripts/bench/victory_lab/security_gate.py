"""Advisory command scan and no-write integrity gate for Victory Lab v2."""
from __future__ import annotations

from dataclasses import dataclass
import hashlib
import os
from pathlib import Path
import re
from typing import Mapping, Sequence

try:
    from .sanitize_v2 import bounded_reason, digest_text
except ImportError:  # pragma: no cover
    from sanitize_v2 import bounded_reason, digest_text


_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_NETWORK_RE = re.compile(r"(?i)(https?://|ftp://|\\\\|\b(socket|requests|urllib|curl|wget|nc|netcat|network|webclient|invoke-webrequest)\b)")
_MCP_RE = re.compile(r"(?i)\bmcp\b|model.context.protocol")
_SECRET_RE = re.compile(r"(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key|authorization|credential|cookie)")


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
    if _MCP_RE.search(command_text) or _MCP_RE.search(env_names) or _MCP_RE.search(env_values):
        findings.add("mcp_indicator")
    if _SECRET_RE.search(command_text) or _SECRET_RE.search(env_names) or _SECRET_RE.search(env_values):
        findings.add("secret_indicator")
    return {
        "scan_mode": "static_advisory",
        "runtime_proof": False,
        "status": "FAIL" if findings else "PASS",
        "reason_codes": sorted(findings),
    }


class SecurityGate:
    """Configurable before/after no-write gate with honest scan semantics."""

    def __init__(self, protected_paths: Mapping[str, str | os.PathLike[str]] | Sequence[str | os.PathLike[str]] = ()) -> None:
        self.protected_paths = protected_paths
        self.before: IntegritySnapshot | None = None

    def start(self, argv: Sequence[object] = (), env: Mapping[str, object] | None = None) -> dict[str, object]:
        self.before = snapshot_paths(self.protected_paths)
        return {
            "integrity_before": self.before.to_dict(),
            "advisory_scan": scan_command_env(argv, env or {}),
        }

    def finish(self, argv: Sequence[object] = (), env: Mapping[str, object] | None = None) -> dict[str, object]:
        if self.before is None:
            raise RuntimeError("security gate was not started")
        after = snapshot_paths(self.protected_paths)
        integrity = compare_snapshots(self.before, after)
        scan = scan_command_env(argv, env or {})
        # The integrity result is the no-write gate.  Command scanning remains
        # advisory and cannot be promoted to runtime network proof.
        status = str(integrity["status"])
        return {
            "status": status,
            "integrity_after": after.to_dict(),
            "integrity": integrity,
            "advisory_scan": scan,
            "runtime_proof": False,
        }


def run_security_gate(
    argv: Sequence[object], env: Mapping[str, object], protected_paths: Mapping[str, str | os.PathLike[str]] | Sequence[str | os.PathLike[str]] = (),
) -> dict[str, object]:
    gate = SecurityGate(protected_paths)
    gate.start(argv, env)
    return gate.finish(argv, env)


__all__ = [
    "IntegritySnapshot", "SecurityGate", "compare_snapshots", "run_security_gate", "scan_command_env", "snapshot_paths",
]
