"""Deterministic payload normalization for heterogeneous Victory Lab outputs."""
from __future__ import annotations

import hashlib
import json
import re
from pathlib import Path
from typing import Any

try:
    from .schema_v2 import CANONICAL_SCHEMA
except ImportError:  # pragma: no cover
    from schema_v2 import CANONICAL_SCHEMA

_VOLATILE_KEYS = frozenset({"elapsed_ms", "duration_ms", "duration_ns", "started_at", "finished_at", "pid", "hostname", "rss_bytes", "raw_output", "stdout", "stderr"})
_SECRET_KEY_RE = re.compile(r"(secret|token|password|api[_-]?key|authorization|cookie)", re.I)


def _relative(value: str, fixture_root: Path | None) -> str:
    value = value.replace("\\", "/")
    if fixture_root is None:
        return value
    root = fixture_root.resolve().as_posix().rstrip("/") + "/"
    candidate = value
    if candidate.startswith(root):
        return candidate[len(root):]
    return value


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
        normalized = [canonicalize(v, fixture_root, key=key) for v in value]
        # Items are sorted only when they are maps; preserving path order is meaningful.
        if all(isinstance(item, dict) for item in normalized):
            return sorted(normalized, key=lambda item: json.dumps(item, ensure_ascii=False, sort_keys=True, separators=(",", ":")))
        return normalized
    if isinstance(value, str):
        return _relative(value.replace("\r\n", "\n").replace("\r", "\n"), fixture_root)
    return value


def canonical_json(value: Any, fixture_root: Path | None = None) -> str:
    return json.dumps(canonicalize(value, fixture_root), ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def payload_digest(value: Any, fixture_root: Path | None = None) -> str:
    return hashlib.sha256(canonical_json(value, fixture_root).encode("utf-8")).hexdigest()


def token_count(value: Any, fixture_root: Path | None = None) -> int:
    text = value if isinstance(value, str) else canonical_json(value, fixture_root)
    return len(re.findall(r"\w+|[^\w\s]", text, re.UNICODE))


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
