"""Versioned schemas and fail-closed records for Victory Lab v2."""
from __future__ import annotations

from dataclasses import dataclass, field
import math
import re
from typing import Any, Mapping

ADAPTER_SCHEMA = "victory-adapter-spec/v2"
RUN_SCHEMA = "victory-run-record/v2"
MANIFEST_SCHEMA = "victory-lab-manifest/v2"
CANONICAL_SCHEMA = "victory-canonical/v2"
STATUSES = frozenset({"PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN"})
OPERATIONS = frozenset({"callers", "affected", "path"})
ADAPTER_KINDS = frozenset({"current", "baseline", "graphify"})
_HEX40_RE = re.compile(r"^[0-9a-f]{40}$")
_HEX64_RE = re.compile(r"^[0-9a-f]{64}$")
_SAFE_CODE_RE = re.compile(r"^[a-z][a-z0-9_.-]{0,63}$")
_RUN_KEYS = frozenset({
    "schema", "case_id", "adapter_id", "operation", "status", "repetition", "fixture_digest", "oracle_digest",
    "executable_sha256", "source_sha256", "commit", "version", "capabilities", "argv", "cwd", "env_keys",
    "elapsed_ms", "canonical", "metrics", "error",
})
_FORBIDDEN_KEYS = frozenset({"stdout", "stderr", "raw_output", "native_output", "source_payload"})


class SchemaError(ValueError):
    """Raised when a versioned Victory Lab object is invalid."""


def _nonempty(value: Any, name: str) -> str:
    if not isinstance(value, str) or not value.strip():
        raise SchemaError(f"{name} must be a non-empty string")
    return value


def _string_list(value: Any, name: str) -> list[str]:
    if not isinstance(value, list) or any(not isinstance(x, str) or not x for x in value):
        raise SchemaError(f"{name} must be a list of non-empty strings")
    return list(value)


def _optional_pin(value: str, name: str, length: int) -> None:
    if value and not re.fullmatch(rf"[0-9a-f]{{{length}}}", value):
        raise SchemaError(f"{name} must be a lowercase hexadecimal {length}-character pin")


def _walk_forbidden(value: Any, *, where: str = "record") -> None:
    if isinstance(value, Mapping):
        for raw_key, child in value.items():
            key = str(raw_key)
            lowered = key.lower()
            if lowered in _FORBIDDEN_KEYS or "stdout" in lowered or "stderr" in lowered or "raw_output" in lowered:
                raise SchemaError(f"raw native output is forbidden in {where}")
            _walk_forbidden(child, where=f"{where}.{key}")
    elif isinstance(value, (list, tuple)):
        for index, child in enumerate(value):
            _walk_forbidden(child, where=f"{where}[{index}]")
    elif isinstance(value, float) and not math.isfinite(value):
        raise SchemaError(f"non-finite number is forbidden in {where}")


def _required_digest(value: Any, name: str) -> str:
    if not isinstance(value, str) or not _HEX64_RE.fullmatch(value):
        raise SchemaError(f"{name} must be a lowercase hexadecimal 64-character digest")
    return value


@dataclass(frozen=True)
class AdapterSpec:
    """The immutable, manifest-backed contract for one comparator adapter."""

    adapter_id: str
    kind: str
    executable: str = ""
    source: str = ""
    expected_commit: str = ""
    expected_version: str = ""
    expected_executable_sha256: str = ""
    expected_source_sha256: str = ""
    source_digest_path: str = ""
    capabilities: tuple[str, ...] = ()
    comparable_operations: tuple[str, ...] = ()
    normalizable_operations: tuple[str, ...] = ()
    env_allowlist: tuple[str, ...] = ()
    timeout_seconds: float = 30.0
    command: tuple[str, ...] = ()
    metadata_command: tuple[str, ...] = ()
    path_env: str = ""
    source_path_env: str = ""
    python_path_env: str = ""

    def validate(self) -> None:
        _nonempty(self.adapter_id, "adapter_id")
        if self.kind not in ADAPTER_KINDS:
            raise SchemaError(f"unsupported adapter kind: {self.kind}")
        _optional_pin(self.expected_commit, "expected_commit", 40)
        _optional_pin(self.expected_executable_sha256, "expected_executable_sha256", 64)
        _optional_pin(self.expected_source_sha256, "expected_source_sha256", 64)
        if self.timeout_seconds <= 0:
            raise SchemaError("timeout_seconds must be positive")
        for name, values in (
            ("capabilities", self.capabilities),
            ("comparable_operations", self.comparable_operations),
            ("normalizable_operations", self.normalizable_operations),
        ):
            for operation in values:
                if operation not in OPERATIONS:
                    raise SchemaError(f"{name} contains unsupported operation: {operation}")
        if not set(self.comparable_operations).issubset(self.capabilities):
            raise SchemaError("comparable_operations must be capabilities")
        if not set(self.normalizable_operations).issubset(self.capabilities):
            raise SchemaError("normalizable_operations must be capabilities")
        if self.kind == "baseline" and tuple(self.comparable_operations) != ("affected",):
            raise SchemaError("baseline is comparable only for affected")
        if self.kind == "graphify" and "affected" in self.comparable_operations and "affected" not in self.normalizable_operations:
            raise SchemaError("Graphify affected requires an explicitly normalizable capability")
        if self.kind == "graphify" and "path" in self.comparable_operations and "path" not in self.normalizable_operations:
            raise SchemaError("Graphify path requires an explicitly normalizable capability")
        _string_list(list(self.env_allowlist), "env_allowlist")
        if not self.command:
            raise SchemaError(f"{self.adapter_id} has no command template")

    @classmethod
    def from_dict(cls, value: Mapping[str, Any]) -> "AdapterSpec":
        if value.get("schema") != ADAPTER_SCHEMA:
            raise SchemaError("unsupported adapter spec schema")
        spec = cls(
            adapter_id=_nonempty(value.get("adapter_id"), "adapter_id"),
            kind=_nonempty(value.get("kind"), "kind"),
            executable=str(value.get("executable", "")),
            source=str(value.get("source", "")),
            expected_commit=str(value.get("expected_commit", "")),
            expected_version=str(value.get("expected_version", "")),
            expected_executable_sha256=str(value.get("expected_executable_sha256", "")),
            expected_source_sha256=str(value.get("expected_source_sha256", "")),
            source_digest_path=str(value.get("source_digest_path", "")),
            capabilities=tuple(_string_list(value.get("capabilities", []), "capabilities")),
            comparable_operations=tuple(_string_list(value.get("comparable_operations", []), "comparable_operations")),
            normalizable_operations=tuple(_string_list(value.get("normalizable_operations", []), "normalizable_operations")),
            env_allowlist=tuple(_string_list(value.get("env_allowlist", []), "env_allowlist")),
            timeout_seconds=float(value.get("timeout_seconds", 30.0)),
            command=tuple(_string_list(value.get("command", []), "command")),
            metadata_command=tuple(_string_list(value.get("metadata_command", []), "metadata_command")),
            path_env=str(value.get("path_env", "")),
            source_path_env=str(value.get("source_path_env", "")),
            python_path_env=str(value.get("python_path_env", "")),
        )
        spec.validate()
        return spec

    def to_dict(self) -> dict[str, Any]:
        return {
            "schema": ADAPTER_SCHEMA,
            "adapter_id": self.adapter_id,
            "kind": self.kind,
            "executable": self.executable,
            "source": self.source,
            "expected_commit": self.expected_commit,
            "expected_version": self.expected_version,
            "expected_executable_sha256": self.expected_executable_sha256,
            "expected_source_sha256": self.expected_source_sha256,
            "source_digest_path": self.source_digest_path,
            "capabilities": list(self.capabilities),
            "comparable_operations": list(self.comparable_operations),
            "normalizable_operations": list(self.normalizable_operations),
            "env_allowlist": list(self.env_allowlist),
            "timeout_seconds": self.timeout_seconds,
            "command": list(self.command),
            "metadata_command": list(self.metadata_command),
            "path_env": self.path_env,
            "source_path_env": self.source_path_env,
            "python_path_env": self.python_path_env,
        }


@dataclass
class RunRecord:
    """A durable, sanitized sample. Native stdout/stderr never belongs here."""

    adapter_id: str
    operation: str
    status: str
    case_id: str = ""
    repetition: int = 0
    fixture_digest: str = ""
    oracle_digest: str = ""
    executable_sha256: str = ""
    source_sha256: str = ""
    commit: str = ""
    version: str = ""
    capabilities: list[str] = field(default_factory=list)
    argv: list[str] = field(default_factory=list)
    cwd: str = ""
    env_keys: list[str] = field(default_factory=list)
    elapsed_ms: float | None = None
    canonical: Any = None
    metrics: dict[str, Any] = field(default_factory=dict)
    error: dict[str, Any] | None = None

    def validate(self) -> None:
        _nonempty(self.adapter_id, "adapter_id")
        if self.case_id and not isinstance(self.case_id, str):
            raise SchemaError("case_id must be a string")
        if self.operation not in OPERATIONS:
            raise SchemaError(f"unsupported operation: {self.operation}")
        if self.status not in STATUSES:
            raise SchemaError(f"unsupported status: {self.status}")
        if not isinstance(self.repetition, int) or isinstance(self.repetition, bool) or self.repetition < 0:
            raise SchemaError("repetition must be a non-negative integer")
        for name, value in (("fixture_digest", self.fixture_digest), ("oracle_digest", self.oracle_digest),
                            ("executable_sha256", self.executable_sha256), ("source_sha256", self.source_sha256)):
            if value:
                _required_digest(value, name)
        if self.commit and not _HEX40_RE.fullmatch(self.commit):
            raise SchemaError("commit must be a lowercase hexadecimal 40-character pin")
        if not isinstance(self.capabilities, list) or any(not isinstance(item, str) for item in self.capabilities):
            raise SchemaError("capabilities must be a list of strings")
        if self.argv or self.cwd:
            raise SchemaError("raw argv and cwd are not durable evidence")
        if not isinstance(self.env_keys, list) or any(not isinstance(item, str) for item in self.env_keys):
            raise SchemaError("env_keys must be a list of strings")
        if self.elapsed_ms is not None and (
            isinstance(self.elapsed_ms, bool) or not isinstance(self.elapsed_ms, (int, float)) or not math.isfinite(float(self.elapsed_ms))
        ):
            raise SchemaError("elapsed_ms must be a finite number")
        if not isinstance(self.metrics, dict):
            raise SchemaError("metrics must be an object")
        _walk_forbidden(self.metrics, where="metrics")
        _walk_forbidden(self.canonical, where="canonical")
        _walk_forbidden(self.error, where="error")
        if self.status == "PASS":
            if not isinstance(self.canonical, Mapping):
                raise SchemaError("PASS records require canonical payload")
            if self.canonical.get("schema") != CANONICAL_SCHEMA or self.canonical.get("operation") != self.operation:
                raise SchemaError("PASS records have canonical schema drift")
            _required_digest(self.canonical.get("digest"), "canonical.digest")
            if not isinstance(self.canonical.get("payload"), Mapping):
                raise SchemaError("PASS records require canonical payload object")
            if self.error is not None:
                raise SchemaError("PASS records cannot carry an error")
        elif self.status in {"FAIL", "BLOCKED"} and not isinstance(self.error, Mapping):
            raise SchemaError(f"{self.status} records require sanitized error")
        if self.error is not None:
            if not isinstance(self.error, Mapping) or set(self.error) - {"kind", "reason_code"}:
                raise SchemaError("error must contain only kind and reason_code")
            if any(not isinstance(value, str) or not _SAFE_CODE_RE.fullmatch(value) for value in self.error.values()):
                raise SchemaError("error fields must be bounded reason codes")

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        return {
            "schema": RUN_SCHEMA,
            "case_id": self.case_id,
            "adapter_id": self.adapter_id,
            "operation": self.operation,
            "status": self.status,
            "repetition": self.repetition,
            "fixture_digest": self.fixture_digest,
            "oracle_digest": self.oracle_digest,
            "executable_sha256": self.executable_sha256,
            "source_sha256": self.source_sha256,
            "commit": self.commit,
            "version": self.version,
            "capabilities": sorted(set(self.capabilities)),
            "argv": list(self.argv),
            "cwd": self.cwd.replace("\\", "/"),
            "env_keys": sorted(set(self.env_keys)),
            "elapsed_ms": self.elapsed_ms,
            "canonical": self.canonical,
            "metrics": self.metrics,
            "error": self.error,
        }

    @classmethod
    def from_dict(cls, value: Mapping[str, Any]) -> "RunRecord":
        if not isinstance(value, Mapping) or value.get("schema") != RUN_SCHEMA:
            raise SchemaError("unsupported run record schema")
        unknown = set(value) - _RUN_KEYS
        if unknown:
            raise SchemaError(f"run record schema drift: unexpected fields {sorted(unknown)}")
        required = {"adapter_id", "operation", "status", "repetition", "canonical", "metrics", "error"}
        missing = required - set(value)
        if missing:
            raise SchemaError(f"run record schema drift: missing fields {sorted(missing)}")
        record = cls(
            case_id=value.get("case_id", ""), adapter_id=value.get("adapter_id"), operation=value.get("operation"),
            status=value.get("status"), repetition=value.get("repetition"),
            fixture_digest=value.get("fixture_digest", ""), oracle_digest=value.get("oracle_digest", ""),
            executable_sha256=value.get("executable_sha256", ""), source_sha256=value.get("source_sha256", ""),
            commit=value.get("commit", ""), version=value.get("version", ""),
            capabilities=value.get("capabilities", []), argv=value.get("argv", []),
            cwd=value.get("cwd", ""), env_keys=value.get("env_keys", []),
            elapsed_ms=value.get("elapsed_ms"), canonical=value.get("canonical"),
            metrics=value.get("metrics"), error=value.get("error"),
        )
        record.validate()
        return record
