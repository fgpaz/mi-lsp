"""Versioned schemas and fail-closed records for Victory Lab v2."""
from __future__ import annotations

from dataclasses import dataclass, field
import re
from typing import Any, Mapping

ADAPTER_SCHEMA = "victory-adapter-spec/v2"
RUN_SCHEMA = "victory-run-record/v2"
MANIFEST_SCHEMA = "victory-lab-manifest/v2"
CANONICAL_SCHEMA = "victory-canonical/v2"
STATUSES = frozenset({"PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN"})
OPERATIONS = frozenset({"callers", "affected", "path"})
ADAPTER_KINDS = frozenset({"current", "baseline", "graphify"})


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
        if self.operation not in OPERATIONS:
            raise SchemaError(f"unsupported operation: {self.operation}")
        if self.status not in STATUSES:
            raise SchemaError(f"unsupported status: {self.status}")
        if self.repetition < 0:
            raise SchemaError("repetition must be non-negative")
        if any(key.lower() in {"stdout", "stderr", "raw_output", "native_output", "secret", "token"} for key in self.metrics):
            raise SchemaError("raw native output and secrets cannot be durable")
        if self.status == "PASS" and self.canonical is None:
            raise SchemaError("PASS records require canonical payload")

    def to_dict(self) -> dict[str, Any]:
        self.validate()
        return {
            "schema": RUN_SCHEMA,
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
        if value.get("schema") != RUN_SCHEMA:
            raise SchemaError("unsupported run record schema")
        record = cls(
            adapter_id=str(value.get("adapter_id", "")), operation=str(value.get("operation", "")),
            status=str(value.get("status", "")), repetition=int(value.get("repetition", 0)),
            fixture_digest=str(value.get("fixture_digest", "")), oracle_digest=str(value.get("oracle_digest", "")),
            executable_sha256=str(value.get("executable_sha256", "")), source_sha256=str(value.get("source_sha256", "")),
            commit=str(value.get("commit", "")), version=str(value.get("version", "")),
            capabilities=list(value.get("capabilities", [])), argv=list(value.get("argv", [])),
            cwd=str(value.get("cwd", "")), env_keys=list(value.get("env_keys", [])),
            elapsed_ms=value.get("elapsed_ms"), canonical=value.get("canonical"),
            metrics=dict(value.get("metrics", {})), error=value.get("error"),
        )
        record.validate()
        return record
