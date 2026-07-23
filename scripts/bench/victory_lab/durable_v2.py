"""Shared fail-closed rules for durable Victory Lab evidence."""
from __future__ import annotations

from collections.abc import Mapping
import re
from typing import Any

EMAIL_RE = re.compile(r"(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b")
PHI_RE = re.compile(r"(?i)\b(?:ssn|social\s+security|patient|medical|phi|email|telephone|phone|health\s+record)\b")
SSN_RE = re.compile(r"\b\d{3}-\d{2}-\d{4}\b")
SECRET_RE = re.compile(r"(?i)(?:secret|token|password|passwd|api[_-]?key|authorization|cookie|private[_-]?key|credential)")
WINDOWS_ABSOLUTE_RE = re.compile(r"^[A-Za-z]:[\\/]")
POSIX_ABSOLUTE_RE = re.compile(r"^/")
UNC_RE = re.compile(r"^(?:\\\\|//)")
RELATIVE_PATH_RE = re.compile(r"^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)+$")
DURABLE_ID_RE = re.compile(r"^[a-z][a-z0-9_.-]{0,63}$")
GROUP_KEY_RE = re.compile(r"^[a-z][a-z0-9_.-]{0,63}:[a-z][a-z0-9_.-]{0,63}:[a-z][a-z0-9_.-]{0,63}$")

SAFE_FIXTURE_RELATIVE_PATHS = frozenset({
    "corpus/fixture-metadata.json", "corpus/go.mod", "corpus/go/ambiguous/alpha/shared.go",
    "corpus/go/ambiguous/beta/shared.go", "corpus/go/app/app.go", "corpus/go/callers/callers.go",
    "corpus/go/mutation/mutation.go", "corpus/go/subject/subject.go", "corpus/go/unrelated/unrelated.go",
    "corpus/go/unresolved/unresolved.go", "go/ambiguous/alpha/shared.go", "go/ambiguous/beta/shared.go",
    "go/app/app.go", "go/callers/callers.go", "go/go.mod", "go/mutation/mutation.go",
    "go/subject/subject.go", "go/unrelated/unrelated.go", "go/unresolved/unresolved.go",
    "ambiguous/alpha/shared.go", "ambiguous/beta/shared.go", "app/app.go", "callers/callers.go",
    "mutation/mutation.go", "subject/subject.go", "unrelated/unrelated.go", "unresolved/unresolved.go",
})
_IDENTITY_KEYS = frozenset({
    "adapter_id", "case_id", "fixture_id", "group_id", "group_key", "identity", "profile_id", "repository_id", "run_id",
})


def validate_identifier(value: Any, name: str = "identifier") -> str:
    if not isinstance(value, str) or not DURABLE_ID_RE.fullmatch(value):
        raise ValueError(f"{name} must match the closed durable identifier grammar")
    return value


def validate_group_key(value: Any, name: str = "group_key") -> str:
    if not isinstance(value, str) or not GROUP_KEY_RE.fullmatch(value):
        raise ValueError(f"{name} must match the closed durable group-key grammar")
    return value


def is_safe_fixture_relative_path(value: str, *, allowed_paths: frozenset[str] = SAFE_FIXTURE_RELATIVE_PATHS) -> bool:
    normalized = value.replace("\\", "/")
    if normalized != value and (WINDOWS_ABSOLUTE_RE.match(value) or UNC_RE.match(value)):
        return False
    if not RELATIVE_PATH_RE.fullmatch(normalized):
        return False
    parts = normalized.split("/")
    if any(part in {"", ".", ".."} for part in parts) or ":" in normalized or "\x00" in normalized:
        return False
    return normalized in allowed_paths


def _looks_like_path(value: str) -> bool:
    if value.startswith("victory-"):
        return False
    normalized = value.replace("\\", "/")
    return bool(WINDOWS_ABSOLUTE_RE.match(value) or POSIX_ABSOLUTE_RE.match(normalized) or UNC_RE.match(value) or RELATIVE_PATH_RE.fullmatch(normalized))


def validate_durable(value: Any, *, where: str = "record", allowed_paths: frozenset[str] = SAFE_FIXTURE_RELATIVE_PATHS) -> None:
    """Reject identities, group keys, sensitive data, and arbitrary paths."""
    if isinstance(value, Mapping):
        for raw_key, child in value.items():
            key = str(raw_key)
            lowered = key.lower()
            if (lowered != "token_units" and SECRET_RE.search(lowered)) or lowered in {"stdout", "stderr", "raw_output", "native_output", "source_payload"}:
                raise ValueError(f"sensitive durable key in {where}.{key}")
            if lowered in _IDENTITY_KEYS:
                validate_identifier(child, f"{where}.{key}")
            if lowered in {"group", "groups"} and isinstance(child, Mapping):
                for group_key in child:
                    validate_group_key(group_key, f"{where}.{key}")
            if ":" in key and (lowered.startswith("group") or where.endswith("groups")):
                validate_group_key(key, f"{where}.{key}")
            validate_durable(child, where=f"{where}.{key}", allowed_paths=allowed_paths)
        return
    if isinstance(value, (list, tuple, set, frozenset)):
        for index, child in enumerate(value):
            validate_durable(child, where=f"{where}[{index}]", allowed_paths=allowed_paths)
        return
    if not isinstance(value, str):
        return
    if EMAIL_RE.search(value) or PHI_RE.search(value) or SSN_RE.search(value):
        raise ValueError(f"PII/PHI in {where}")
    if WINDOWS_ABSOLUTE_RE.match(value) or POSIX_ABSOLUTE_RE.match(value) or UNC_RE.match(value):
        raise ValueError(f"absolute path in {where}")
    if _looks_like_path(value) and not is_safe_fixture_relative_path(value, allowed_paths=allowed_paths):
        raise ValueError(f"unsafe relative path in {where}")


__all__ = [
    "DURABLE_ID_RE", "GROUP_KEY_RE", "SAFE_FIXTURE_RELATIVE_PATHS", "is_safe_fixture_relative_path",
    "validate_durable", "validate_group_key", "validate_identifier",
]
