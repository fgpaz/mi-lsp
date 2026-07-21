"""Deterministic payload normalization for heterogeneous Victory Lab outputs."""
from __future__ import annotations

import hashlib
import json
import ntpath
import re
from pathlib import Path
from typing import Any

try:
    from .schema_v2 import CANONICAL_SCHEMA
    from .durable_v2 import EMAIL_RE, PHI_RE, POSIX_ABSOLUTE_RE, SSN_RE, WINDOWS_ABSOLUTE_RE, UNC_RE
except ImportError:  # pragma: no cover
    from schema_v2 import CANONICAL_SCHEMA
    from durable_v2 import EMAIL_RE, PHI_RE, POSIX_ABSOLUTE_RE, SSN_RE, WINDOWS_ABSOLUTE_RE, UNC_RE

_VOLATILE_KEYS = frozenset({"elapsed_ms", "duration_ms", "duration_ns", "started_at", "finished_at", "pid", "hostname", "rss_bytes", "raw_output", "stdout", "stderr"})
_SECRET_KEY_RE = re.compile(r"(secret|token|password|api[_-]?key|authorization|cookie)", re.I)
_SENSITIVE_VALUE_RE = re.compile(
    r"(?ix)(?:"
    r"[A-Z]:[\\/][^\s\r\n]+|\\\\[^\s\r\n]+|/[^\s\r\n]+|"
    r"(?:^|[\s\"'])(?:\.?\.?[\\/]|[\w.-]+[\\/])[^\s\r\n]+|"
    r"[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b|"
    r"\b(?:ssn|social\s+security|patient|medical|phi|email|telephone|phone)\b|"
    r"\b\d{3}-\d{2}-\d{4}\b"
    r")",
    re.I,
)
_VALID_BACKENDS = frozenset({
    "go", "csharp", "roslyn", "typescript", "ts", "tsserver", "graph", "graph-native",
    "graphify", "catalog", "mi-lsp", "router", "current", "baseline", "native", "sqlite-direct", "index-job", "version",
    "git+catalog+heuristic",
})
_VALID_COMPLETENESS = frozenset({"complete", "exact", "full"})
_FIXTURE_ROOT_SENTINEL = "<FIXTURE_ROOT>"


def _relative(value: str, fixture_root: Path | None) -> str:
    value = value.replace("\\", "/")
    if fixture_root is None:
        return value
    root = fixture_root.resolve().as_posix().rstrip("/") + "/"
    candidate = value
    if candidate.startswith(root):
        return candidate[len(root):]
    return value


def _windows_path_key(value: str) -> str:
    """Normalize a path for Windows identity comparisons on any host OS."""
    return ntpath.normcase(ntpath.normpath(value.replace("/", "\\")))


def _is_fixture_identity(value: str, fixture_root: Path | None) -> bool:
    """Recognize only the fixture basename or its equivalent full path."""
    if fixture_root is None:
        return False
    if value.casefold() == fixture_root.name.casefold():
        return True
    fixture_paths = {
        _windows_path_key(str(fixture_root)),
        _windows_path_key(str(fixture_root.resolve())),
    }
    return _windows_path_key(value) in fixture_paths


def _is_fixture_relative_path(value: str, fixture_root: Path | None) -> bool:
    """Allow only existing benchmark files to retain relative path identity."""
    if fixture_root is None or not value or value.startswith(("/", "\\")):
        return False
    parts = tuple(part for part in value.replace("\\", "/").split("/") if part not in ("", "."))
    if not parts or ".." in parts:
        return False
    root = fixture_root.resolve()
    base = root / "corpus" / "go"
    relative = Path(*parts)
    candidates = [base / relative]
    # Native tools may report either a fixture-root-relative path or a path
    # relative to the configured Go corpus.  Both forms must use the same
    # existing-file allowlist, while PII/PHI is checked before this function.
    if parts[:2] == ("corpus", "go"):
        candidates.insert(0, root / relative)
    elif parts[0] == "go":
        candidates.insert(0, root / "corpus" / relative)
    for raw_candidate in candidates:
        candidate = raw_candidate.resolve()
        try:
            candidate.relative_to(root)
        except ValueError:
            continue
        if candidate.is_file():
            return True
    return False


def canonicalize(value: Any, fixture_root: Path | None = None, *, key: str = "") -> Any:
    if _SECRET_KEY_RE.search(key):
        return "<REDACTED>"
    if isinstance(value, dict):
        return {
            str(k): canonicalize(v, fixture_root, key=str(k))
            for k, v in sorted(value.items(), key=lambda item: str(item[0]))
            if str(k) not in _VOLATILE_KEYS and not _SECRET_KEY_RE.search(str(k))
        }
    if isinstance(value, list):
        # Result order is part of the oracle.  In particular, shortest paths
        # and distance-ordered relation results must never be re-sorted here.
        return [canonicalize(v, fixture_root, key=key) for v in value]
    if isinstance(value, str):
        if key == "workspace" and _is_fixture_identity(value, fixture_root):
            return _FIXTURE_ROOT_SENTINEL
        original = value.replace("\\", "/")
        normalized = _relative(value.replace("\r\n", "\n").replace("\r", "\n"), fixture_root)
        # Workspace is sentinel-only: an unrecognized path remains redacted,
        # even when it happens to be below the fixture root.
        if key == "workspace" and normalized != original:
            return "<REDACTED>"
        # Relativization is only a normalization step.  It must not bypass the
        # PII/PHI gate: a sensitive value can become fixture-relative first.
        if EMAIL_RE.search(normalized) or PHI_RE.search(normalized) or SSN_RE.search(normalized):
            return "<REDACTED>"
        # After PII/PHI checks, a path made relative to the fixture is
        # comparable even when the fixture is synthetic or not materialized.
        if fixture_root is not None and normalized != original:
            return normalized
        if key == "path" and _is_fixture_relative_path(normalized, fixture_root):
            return normalized
        if _SENSITIVE_VALUE_RE.search(normalized):
            return "<REDACTED>"
        return normalized
    return value


def canonical_json(value: Any, fixture_root: Path | None = None) -> str:
    return json.dumps(canonicalize(value, fixture_root), ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def payload_digest(value: Any, fixture_root: Path | None = None) -> str:
    return hashlib.sha256(canonical_json(value, fixture_root).encode("utf-8")).hexdigest()


def token_count(value: Any, fixture_root: Path | None = None) -> int:
    text = value if isinstance(value, str) else canonical_json(value, fixture_root)
    return len(re.findall(r"\w+|[^\w\s]", text, re.UNICODE))


def validate_terminal_state(native: Any) -> dict[str, Any]:
    """Require a complete terminal response before canonicalization.

    A timeout, partial envelope, backend error, or truncated result is not a
    successful query merely because it contains an ``items`` array.
    """
    if not isinstance(native, dict):
        raise ValueError("terminal output must be an object")
    if native.get("ok") is not True:
        raise ValueError("terminal output is not ok=true")
    if native.get("done") is not True:
        raise ValueError("terminal output is not done=true")
    for key in ("state", "terminal_state", "phase"):
        state = native.get(key)
        if state is not None and str(state).lower() not in {"done", "complete", "completed", "terminal", "success"}:
            raise ValueError("terminal output has an incompatible state")
    backend = native.get("backend")
    if not isinstance(backend, str) or backend not in _VALID_BACKENDS:
        raise ValueError("terminal output has invalid backend")
    completeness = native.get("completeness")
    if completeness is None and native.get("complete") is True:
        completeness = "complete"
    if completeness is None and native.get("truncated") is False and native.get("error") in (None, "", {}, []):
        # mi-lsp v2 envelopes use ok=true + truncated=false as the complete
        # terminal contract and omit a redundant completeness string.
        completeness = "complete"
    if completeness not in _VALID_COMPLETENESS:
        raise ValueError("terminal output has invalid completeness")
    if native.get("truncated") is not False:
        raise ValueError("terminal output is truncated or lacks truncation=false")
    if native.get("error") not in (None, "", {}, []):
        raise ValueError("terminal output contains an error")
    if native.get("errors") not in (None, "", {}, []):
        raise ValueError("terminal output contains errors")
    if native.get("partial") not in (None, False):
        raise ValueError("terminal output is partial")
    return native


def canonical_payload(operation: str, native: Any, fixture_root: Path | None = None) -> dict[str, Any]:
    """Extract only comparator-facing fields and attach a stable digest."""
    if not isinstance(native, dict):
        raise ValueError("native output must be a JSON object")
    payload = dict(native)
    payload.pop("raw_output", None)
    payload.pop("stdout", None)
    payload.pop("stderr", None)
    payload["operation"] = operation
    normalized = canonicalize(payload, fixture_root)
    return {
        "schema": CANONICAL_SCHEMA,
        "operation": operation,
        "payload": normalized,
        "digest": payload_digest(normalized),
        "token_units": token_count(normalized),
    }


def parse_json_output(stdout: str) -> Any:
    """Parse one JSON document, tolerating a single diagnostic line before it."""
    text = stdout.strip()
    if not text:
        raise ValueError("empty native output")
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = min((pos for pos in (text.find("{"), text.find("[")) if pos >= 0), default=-1)
        if start < 0:
            raise ValueError("native output is not JSON")
        try:
            return json.loads(text[start:])
        except json.JSONDecodeError as exc:
            raise ValueError("native output contains invalid JSON") from exc
