"""Fail-closed sanitization for Victory Lab v2 process evidence.

Durable process evidence is intentionally reduced to numbers, digests, opaque
IDs, and bounded reason codes.  Native output is only hashed transiently.
"""
from __future__ import annotations

import hashlib
import math
import os
import re
from pathlib import Path
from typing import Any, Mapping, Sequence

_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_ID_RE = re.compile(r"^[A-Za-z0-9_.:/-]{1,128}$")
_REASON_RE = re.compile(r"^[a-z][a-z0-9_.-]{0,63}$")
_SECRET_RE = re.compile(r"(?i)(password|passwd|secret|token|api[_-]?key|private[_-]?key|authorization|cookie)")
_PATH_RE = re.compile(r"(?:[A-Za-z]:[\\/]|\\\\|/(?:Users|home|root|private|tmp)/)")
_FORBIDDEN_KEYS = frozenset({"stdout", "stderr", "raw_output", "native_output", "source", "source_payload", "env", "environment"})


def digest_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def digest_text(value: object) -> str:
    return digest_bytes(str(value).encode("utf-8", "replace"))


def bounded_reason(value: object, default: str = "unspecified") -> str:
    raw = str(value or default).strip()
    if _PATH_RE.search(raw) or _SECRET_RE.search(raw):
        return default
    text = raw.lower().replace(" ", "_")
    text = re.sub(r"[^a-z0-9_.-]", "_", text)[:64]
    return text if _REASON_RE.fullmatch(text) else default


def sanitize_env(env: Mapping[str, object], allowlist: Sequence[str]) -> list[str]:
    """Return only allowlisted variable names; never persist values."""
    allowed = {str(name) for name in allowlist}
    return sorted(name for name in env if str(name) in allowed and not _SECRET_RE.search(str(name)))


def sanitize_argv(argv: Sequence[object]) -> dict[str, object]:
    """Persist command shape and digest, not command/path/secret values."""
    values = [str(value) for value in argv]
    return {"argc": len(values), "argv_sha256": digest_text("\0".join(values))}


def _safe_scalar(key: str, value: object) -> tuple[str, object] | None:
    lowered = key.lower()
    if lowered in _FORBIDDEN_KEYS or _SECRET_RE.search(lowered) or "path" in lowered or lowered in {"cwd", "command", "argv"}:
        return None
    if isinstance(value, bool):
        return key, value
    if isinstance(value, int):
        return key, value
    if isinstance(value, float):
        return (key, value) if math.isfinite(value) else None
    if isinstance(value, str):
        if lowered in {"reason", "reason_code"}:
            return key, bounded_reason(value)
        if _PATH_RE.search(value) or _SECRET_RE.search(value):
            return None
        if _SHA256_RE.fullmatch(value) and ("digest" in lowered or lowered.endswith("sha256")):
            return key, value
        if (lowered.endswith("_id") or lowered == "id") and _ID_RE.fullmatch(value):
            return key, value
        if lowered in {"status", "failure_class", "cleanup_status"} and _ID_RE.fullmatch(value):
            return key, value
        return None
    return None


def sanitize_metrics(metrics: Mapping[str, object] | None) -> dict[str, object]:
    """Keep a bounded, flat-ish projection of metric evidence."""
    if not isinstance(metrics, Mapping):
        return {}
    result: dict[str, object] = {}
    reason_codes: list[str] = []
    for raw_key, raw_value in metrics.items():
        key = str(raw_key)
        if key.lower() in _FORBIDDEN_KEYS or _SECRET_RE.search(key.lower()):
            continue
        if key == "reason_codes" and isinstance(raw_value, (list, tuple)):
            reason_codes.extend(bounded_reason(item) for item in raw_value)
            continue
        if isinstance(raw_value, Mapping):
            nested = sanitize_metrics(raw_value)
            if nested:
                result[key] = nested
            continue
        pair = _safe_scalar(key, raw_value)
        if pair is not None:
            result[pair[0]] = pair[1]
    if reason_codes:
        result["reason_codes"] = sorted(set(reason_codes))[:32]
    return result


def sanitize_process_result(result: object, *, env_allowlist: Sequence[str] = ()) -> dict[str, object]:
    """Project an executor result without persisting output, env, or paths."""
    metrics = getattr(result, "metrics", None)
    metric_dict = metrics.to_dict() if hasattr(metrics, "to_dict") else metrics
    output: dict[str, object] = {
        "returncode": int(getattr(result, "returncode", 127)),
        "timed_out": bool(getattr(result, "timed_out", False)),
        "crashed": bool(getattr(result, "crashed", False)),
        "elapsed_ms": float(getattr(result, "elapsed_ms", 0.0)),
        "argv": sanitize_argv(getattr(result, "argv", [])),
        "env_keys": sanitize_env({str(key): None for key in getattr(result, "env_keys", [])}, env_allowlist),
        "metrics": sanitize_metrics(metric_dict if isinstance(metric_dict, Mapping) else {}),
    }
    stdout = getattr(result, "stdout", "")
    stderr = getattr(result, "stderr", "")
    # Hashes are useful diagnostics but payloads remain absent from the object.
    output["stdout_sha256"] = digest_text(stdout)
    output["stderr_sha256"] = digest_text(stderr)
    return output


def sanitize_error(kind: object, message: object = "") -> dict[str, str]:
    """Return bounded diagnostics with no path or exception payload."""
    code = bounded_reason(kind, "unknown")
    msg_code = bounded_reason(message, code)
    return {"kind": code, "reason_code": msg_code}


def sanitize_paths(paths: Sequence[str | os.PathLike[str]]) -> list[str]:
    """Validate paths for use by a gate, returning opaque IDs rather than paths."""
    return [digest_text(Path(path).resolve().as_posix()) for path in paths]


__all__ = [
    "bounded_reason", "digest_bytes", "digest_text", "sanitize_argv", "sanitize_env",
    "sanitize_error", "sanitize_metrics", "sanitize_paths", "sanitize_process_result",
]
