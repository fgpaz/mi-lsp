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

try:
    from .durable_v2 import EMAIL_RE, PHI_RE, SECRET_RE, SSN_RE
except ImportError:  # pragma: no cover
    from durable_v2 import EMAIL_RE, PHI_RE, SECRET_RE, SSN_RE

_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_ID_RE = re.compile(r"^[A-Za-z0-9_.:/-]{1,128}$")
_REASON_RE = re.compile(r"^[a-z][a-z0-9_.-]{0,63}$")
_SECRET_RE = SECRET_RE
_PATH_RE = re.compile(
    r"(?ix)(?:[A-Z]:[\/]|\\|^/|(?:^|[\s\"'])(?:\.\.?[\/]|[\w.-]+[\/])\S+|\.(?:exe|dll|pdb|py|go|cs|ts|tsx|json|toml|yaml|yml|db)(?:$|[\s:#]))"
)


def _pii_match(value: str) -> bool:
    return bool(EMAIL_RE.search(value) or PHI_RE.search(value) or SSN_RE.search(value))
_FORBIDDEN_KEYS = frozenset({"stdout", "stderr", "raw_output", "native_output", "source", "source_payload", "env", "environment"})

# This is deliberately finite.  Error text is diagnostic input only and can
# never become a durable reason code or an accidental PII oracle.
REASON_CATALOG = frozenset({
    "unknown", "unspecified", "spawn", "timeout", "crash", "prepare", "decode", "oracle",
    "provenance", "security", "capability", "comparability", "integrity", "cleanup", "blocked",
    "not_measured", "unavailable", "counter_unavailable", "unsupported_platform", "working_set_unavailable",
    "tree_not_observed", "network_indicator", "mcp_indicator", "secret_indicator", "protected_path_changed",
    "runtime_proof_unavailable", "metadata_missing", "metadata_mismatch", "executable_sha_mismatch",
    "source_sha_mismatch", "source_revision_mismatch", "missing_executable", "missing_source",
    "not_comparable", "nonzero_exit", "invalid_terminal_state", "truncated_output", "native_error",
})
STATUS_CATALOG = frozenset({"PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN"})
FAILURE_CLASS_CATALOG = frozenset({"none", "timeout", "crash", "exit_nonzero", "spawn_error"})
CLEANUP_STATUS_CATALOG = frozenset({"not_required", "clean", "forced", "failed"})
ERROR_KIND_CATALOG = frozenset({
    "unknown", "spawn", "timeout", "crash", "prepare", "decode", "oracle", "provenance",
    "security", "capability", "comparability", "integrity", "cleanup", "blocked",
})


def digest_bytes(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


def digest_text(value: object) -> str:
    return digest_bytes(str(value).encode("utf-8", "replace"))


def bounded_reason(value: object, default: str = "unspecified") -> str:
    """Map an opaque diagnostic to a fixed catalog entry.

    This function is intentionally not a slugifier.  Unknown text, paths,
    secrets, names, emails, SSNs, and PHI all collapse to the caller's fixed
    fallback rather than becoming new durable taxonomy.
    """
    fallback = default if default in REASON_CATALOG else "unspecified"
    raw = str(value or "").strip()
    if not raw or _PATH_RE.search(raw) or _SECRET_RE.search(raw) or _pii_match(raw):
        return fallback
    text = raw.lower().replace(" ", "_")
    text = re.sub(r"[^a-z0-9_.-]", "_", text)[:64]
    return text if _REASON_RE.fullmatch(text) and text in REASON_CATALOG else fallback


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
        if _PATH_RE.search(value) or _SECRET_RE.search(value) or _pii_match(value):
            return None
        if _SHA256_RE.fullmatch(value) and ("digest" in lowered or lowered.endswith("sha256")):
            return key, value
        if (lowered.endswith("_id") or lowered == "id") and _ID_RE.fullmatch(value):
            return key, value
        if lowered == "status" and value in STATUS_CATALOG:
            return key, value
        if lowered == "failure_class" and value in FAILURE_CLASS_CATALOG:
            return key, value
        if lowered == "cleanup_status" and value in CLEANUP_STATUS_CATALOG:
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
    """Return a catalog-only diagnostic; message is never an authority input."""
    candidate = bounded_reason(kind, "unknown")
    code = candidate if candidate in ERROR_KIND_CATALOG else "unknown"
    # Keep the argument for API compatibility, but never derive a code from
    # free-form exception text.  This prevents email/name/SSN/PHI leakage and
    # keeps the taxonomy stable across platforms and child processes.
    _ = message
    return {"kind": code, "reason_code": code}


def sanitize_paths(paths: Sequence[str | os.PathLike[str]]) -> list[str]:
    """Validate paths for use by a gate, returning opaque IDs rather than paths."""
    return [digest_text(Path(path).resolve().as_posix()) for path in paths]


__all__ = [
    "REASON_CATALOG", "bounded_reason", "digest_bytes", "digest_text", "sanitize_argv", "sanitize_env",
    "sanitize_error", "sanitize_metrics", "sanitize_paths", "sanitize_process_result",
]
