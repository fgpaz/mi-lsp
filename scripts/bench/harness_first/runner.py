"""One-shot sanitized Harness-first campaign runner.

Dry-run validates the campaign contract without spawning a candidate. Run mode is
explicit, serial, and never retries a query. The runner does not compare against
Victory Lab or any other implementation.
"""
from __future__ import annotations

import argparse
from dataclasses import dataclass
from datetime import datetime, timezone
import hashlib
import json
import math
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import threading
import time
from typing import Any, Iterable, Mapping, Sequence

SCHEMA = "harness-first-campaign/v1"
REPORT_SCHEMA = "harness-first-report/v1"
MARKER_SCHEMA = "harness-first-run-marker/v1"
PROTOCOL_VERSION = "mi-lsp-v1.1"
MAX_PARITY_DIFFS = 32
MAX_PARITY_DIFF_DEPTH = 8
MAX_PROJECTION_DEPTH = 8
MAX_PROJECTION_NODES = 2048
MAX_PROJECTION_MAPPING_ENTRIES = 128
MAX_PROJECTION_LIST_ITEMS = 128
MAX_PROJECTION_BYTES = 65536
MAX_PROJECTION_STRING_BYTES = 512
MAX_CREDENTIAL_VALUE_BYTES = MAX_PROJECTION_STRING_BYTES
MAX_CREDENTIAL_VALUE_CHARS = 512
MAX_DIFF_NODES = 2048
MARKER_NAME = ".harness-first-run-marker.json"
GLOBAL_MARKER_NAME = ".harness-first-candidate-registry.json"
MODES = ("direct", "daemon")
SECTIONS = ("change", "affected", "callers", "callees", "tests", "contracts", "wiki")
SHA256 = re.compile(r"^[0-9a-f]{64}$")
SOURCE_REVISION = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")
SECRET = re.compile(r"(?i)(?:token|secret|password|credential|api[_-]?key|authorization|bearer)")
KNOWN_TOKEN_PREFIX = re.compile(r"(?i)^(?:ghp_|github_pat_|glpat-|xox[baprs]-|sk_live_|sk_test_)[A-Za-z0-9_-]{16,}$")
AWS_ACCESS_KEY = re.compile(r"^AKIA[0-9A-Z]{16}$")
BEARER_TOKEN = re.compile(r"(?i)^Bearer[ \t]+[A-Za-z0-9._~+/=-]{16,}$")
JWT_SEGMENT = re.compile(r"^[A-Za-z0-9_-]{8,}$")
SENSITIVE_KEY = re.compile(r"(?i)(?:patient|paciente|diagnos(?:is|es)?|diagn[oó]stico|mrn|ssn|dob|medical|health|clinical|phi|pii|email|phone|address|insurance|account|birth)")
CLINICAL_TEXT = re.compile(r"(?i)\b(?:patient|paciente|diagnosis|diagn[oó]stico|condition|condici[oó]n|medical|health|clinical|phi|mrn|ssn|dob|medical record|historia cl[ií]nica)\b")
EMAIL = re.compile(r"\b[^\s@]+@[^\s@]+\.[^\s@]+\b")
PATH = re.compile(r"(?i)(?:^[a-z]:[\\/]|^/|(?:^|[\\/])\.\.?[\\/]|[\\/]\.git[\\/])")
RELATIVE_PATH = re.compile(r"(?i)(?:[^\\/\s]+[\\/])+[^\\/\s]+(?:\.[a-z0-9]{1,32})?$")
MRN = re.compile(r"(?i)\b(?:mrn|medical[ _-]?record)[ :#-]*[a-z0-9-]{4,}\b")
SSN = re.compile(r"\b\d{3}-\d{2}-\d{4}\b")
DOB = re.compile(r"(?i)\b(?:dob|date of birth)[ :_-]*\d{1,4}[/-]\d{1,2}[/-]\d{1,4}\b")
MAX_SAFE_METRIC_INT = int(sys.float_info.max)
VOLATILE = frozenset({
    "backend", "route", "routing_outcome", "daemon", "daemon_state", "request_id",
    "session_id", "occurred_at", "timestamp", "latency_ms", "format_ms", "tokens_est", "ms",
    "warnings", "hint", "next_hint", "telemetry", "memory_pointer",
})
FRESHNESS_STATES = frozenset({"current", "lagging", "stale", "invalid", "unknown"})
DIRECT_ONLY_KINDS = frozenset({"wiki_pack", "explain_change", "workspace_map"})
SEMANTIC_KINDS = frozenset({"wiki_pack", "explain_change", "workspace_map", "related"})
GRAPH_NATIVE_KINDS = frozenset({"workspace_map", "related"})
ALLOWED_KINDS = SEMANTIC_KINDS
FRESHNESS_TRUNCATED_STATE = "__freshness_traversal_truncated__"
REASONS = frozenset({
    "blocked", "invalid_manifest", "marker_exists", "output_reused", "missing_source_sha",
    "source_dirty", "missing_source", "missing_binary", "binary_not_file", "spawn_error",
    "timeout", "nonzero_exit", "decode_error", "schema_mismatch", "correctness_failed",
    "preview_incomplete", "parity_failed", "latency_ceiling", "rss_unavailable", "rss_ceiling",
    "worker_failed", "worker_schema", "worker_preflight_failed", "retry_amplification", "provenance_unavailable",
    "daemon_preflight_failed", "daemon_identity_mismatch", "daemon_protocol_mismatch",
    "candidate_version_failed", "candidate_version_mismatch", "projection_truncated",
    "route_unobserved", "route_mismatch", "kind_schema", "native_error",
})
DEFAULT_BUDGETS = {
    "correctness_percent": 100.0,
    "parity": True,
    "retry_amplification": 1.0,
    "latency_p95_ms": 5000.0,
    "latency_p99_ms": 10000.0,
    "peak_rss_bytes": 1073741824,
    "preview_sections": list(SECTIONS),
    "preview_expansions": ["command", "reason"],
}


class HarnessError(ValueError):
    def __init__(self, reason_code: str, message: str = "") -> None:
        self.reason_code = reason_code if reason_code in REASONS else "native_error"
        super().__init__(message or self.reason_code)


def finite(value: Any) -> bool:
    """Return whether a metric is a finite, safely representable number.

    Integer metrics are bounded before conversion so hostile arbitrary-precision
    integers cannot raise OverflowError while being inspected.
    """
    if isinstance(value, bool):
        return False
    if isinstance(value, int):
        return -MAX_SAFE_METRIC_INT <= value <= MAX_SAFE_METRIC_INT
    if isinstance(value, float):
        return math.isfinite(value)
    return False


def digest(value: Any) -> str:
    data = value if isinstance(value, bytes) else json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()
    return hashlib.sha256(data).hexdigest()


def file_digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def _reject_env(path: Path) -> None:
    if any(part.lower().startswith(".env") for part in path.parts):
        raise HarnessError("blocked")


def load_manifest(path: Path) -> dict[str, Any]:
    _reject_env(path)
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise HarnessError("invalid_manifest") from exc
    if not isinstance(value, dict):
        raise HarnessError("invalid_manifest")
    return value


def strings(value: Any, name: str) -> list[str]:
    if not isinstance(value, list) or any(not isinstance(item, str) or not item.strip() for item in value):
        raise HarnessError("invalid_manifest", name)
    return list(value)


def validate_manifest(manifest: Mapping[str, Any]) -> dict[str, Any]:
    if manifest.get("schema") != SCHEMA:
        raise HarnessError("schema_mismatch")
    campaign_id = manifest.get("campaign_id")
    if not isinstance(campaign_id, str) or not re.fullmatch(r"[a-z][a-z0-9_.-]{0,63}", campaign_id):
        raise HarnessError("invalid_manifest")
    raw_queries = manifest.get("queries")
    if not isinstance(raw_queries, list) or not raw_queries:
        raise HarnessError("invalid_manifest")
    queries: list[dict[str, Any]] = []
    seen: set[str] = set()
    for raw in raw_queries:
        if not isinstance(raw, Mapping):
            raise HarnessError("invalid_manifest")
        query_id = raw.get("id")
        kind = raw.get("kind")
        if not isinstance(query_id, str) or not re.fullmatch(r"[a-z][a-z0-9_.-]{0,63}", query_id) or query_id in seen:
            raise HarnessError("invalid_manifest")
        if not isinstance(kind, str) or kind not in ALLOWED_KINDS:
            raise HarnessError("invalid_manifest")
        if kind == "explain_change" and raw.get("preview_required") is not True:
            raise HarnessError("invalid_manifest")
        if kind in GRAPH_NATIVE_KINDS and raw.get("freshness_rank_required") is not True:
            raise HarnessError("invalid_manifest")
        seen.add(query_id)
        args = strings(raw.get("args"), f"{query_id}.args")
        if args[:1] != ["nav"]:
            raise HarnessError("invalid_manifest")
        modes = strings(raw.get("modes", list(MODES)), f"{query_id}.modes")
        if not modes or any(mode not in MODES for mode in modes) or len(set(modes)) != len(modes):
            raise HarnessError("invalid_manifest")
        if kind in DIRECT_ONLY_KINDS and "daemon" in modes:
            raise HarnessError("invalid_manifest")
        for field in ("direct_args", "daemon_args"):
            if field in raw:
                strings(raw[field], f"{query_id}.{field}")
        if raw.get("expected_digest") is not None and not SHA256.fullmatch(str(raw["expected_digest"])):
            raise HarnessError("invalid_manifest")
        if raw.get("parity_required") not in (None, True, False):
            raise HarnessError("invalid_manifest")
        queries.append(dict(raw, id=query_id, kind=kind, args=args, modes=modes, parity_required=bool(raw.get("parity_required", False))))
    supplied = manifest.get("budgets", {})
    if not isinstance(supplied, Mapping) or set(supplied) - set(DEFAULT_BUDGETS):
        raise HarnessError("invalid_manifest")
    budgets = dict(DEFAULT_BUDGETS)
    budgets.update(supplied)
    for name in ("correctness_percent", "retry_amplification", "latency_p95_ms", "latency_p99_ms", "peak_rss_bytes"):
        if not finite(budgets[name]) or float(budgets[name]) < 0:
            raise HarnessError("invalid_manifest")
    if budgets["correctness_percent"] != 100.0 or budgets["retry_amplification"] != 1.0 or budgets["parity"] is not True:
        raise HarnessError("invalid_manifest")
    if strings(budgets["preview_sections"], "preview_sections") != list(SECTIONS):
        raise HarnessError("invalid_manifest")
    if sorted(strings(budgets["preview_expansions"], "preview_expansions")) != ["command", "reason"]:
        raise HarnessError("invalid_manifest")
    worker_args = manifest.get("worker_status_args", ["worker", "status"])
    if worker_args != ["worker", "status"]:
        raise HarnessError("invalid_manifest")
    return {"schema": SCHEMA, "campaign_id": campaign_id, "queries": queries, "budgets": budgets, "worker_status_args": ["worker", "status"]}


class _TraversalBudget:
    """Shared bounded traversal state for projection and structural inspection."""

    def __init__(
        self,
        *,
        max_nodes: int = MAX_PROJECTION_NODES,
        max_depth: int = MAX_PROJECTION_DEPTH,
        max_mapping_entries: int = MAX_PROJECTION_MAPPING_ENTRIES,
        max_list_items: int = MAX_PROJECTION_LIST_ITEMS,
        max_bytes: int = MAX_PROJECTION_BYTES,
        max_string_bytes: int = MAX_PROJECTION_STRING_BYTES,
    ) -> None:
        self.max_nodes = max_nodes
        self.max_depth = max_depth
        self.max_mapping_entries = max_mapping_entries
        self.max_list_items = max_list_items
        self.max_bytes = max_bytes
        self.max_string_bytes = max_string_bytes
        self.nodes = 0
        self.mapping_entries = 0
        self.list_items = 0
        self.string_bytes = 0
        self.total_bytes = 0
        self.truncated = False

    def consume_node(self, depth: int) -> bool:
        if depth > self.max_depth or self.nodes >= self.max_nodes:
            self.truncated = True
            return False
        self.nodes += 1
        return True

    def consume_string(self, value: str) -> str | None:
        encoded = value.encode("utf-8", "replace")
        if len(encoded) > self.max_string_bytes:
            self.truncated = True
            return None
        if self.string_bytes + len(encoded) > self.max_bytes or self.total_bytes + len(encoded) > self.max_bytes:
            self.truncated = True
            return None
        self.string_bytes += len(encoded)
        self.total_bytes += len(encoded)
        return value

    def consume_mapping_entry(self) -> bool:
        if self.mapping_entries >= self.max_mapping_entries:
            self.truncated = True
            return False
        self.mapping_entries += 1
        return True

    def consume_list_item(self) -> bool:
        if self.list_items >= self.max_list_items:
            self.truncated = True
            return False
        self.list_items += 1
        return True


# Kept as an internal compatibility alias for callers that imported the old name.
_ProjectionBudget = _TraversalBudget


def _safe_text(value: Any) -> str | None:
    if isinstance(value, str):
        return value
    try:
        return str(value)
    except Exception:
        return None


def _looks_like_path(value: str) -> bool:
    return bool(PATH.search(value) or RELATIVE_PATH.search(value) or re.search(r"[\\\\/]", value))


def _sensitive_key(key: str) -> bool:
    lowered = key.strip().lower()
    return (
        lowered in {"stdout", "stderr", "raw_output", "native_output", "env", "environment", "payload"}
        or SECRET.search(lowered) is not None
        or SENSITIVE_KEY.search(lowered) is not None
        or _looks_like_path(key)
    )


_TOKEN_CHARACTERS = frozenset("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._~+/=-")
_HEX_CHARACTERS = frozenset("0123456789abcdefABCDEF")


def _looks_like_high_entropy_token(value: str) -> bool:
    if len(value) < 32 or len(value) > MAX_CREDENTIAL_VALUE_CHARS or not value.isascii():
        return False
    if all(character in _HEX_CHARACTERS for character in value):
        return False
    if any(character not in _TOKEN_CHARACTERS for character in value):
        return False
    classes = sum((
        any(character.islower() for character in value),
        any(character.isupper() for character in value),
        any(character.isdigit() for character in value),
        any(character in "._~+/=-" for character in value),
    ))
    if classes < 3 or len(set(value)) < 16:
        return False
    counts: dict[str, int] = {}
    for character in value:
        counts[character] = counts.get(character, 0) + 1
    length = len(value)
    entropy = -sum((count / length) * math.log2(count / length) for count in counts.values())
    threshold = 4.5 if any(character in "._~+/=-" for character in value) else 5.0
    return entropy >= threshold


def _looks_like_credential_value(value: str) -> bool:
    try:
        if len(value) > MAX_CREDENTIAL_VALUE_CHARS or len(value.encode("utf-8", "replace")) > MAX_CREDENTIAL_VALUE_BYTES:
            return False
    except Exception:
        return False
    if KNOWN_TOKEN_PREFIX.fullmatch(value) or AWS_ACCESS_KEY.fullmatch(value) or BEARER_TOKEN.fullmatch(value):
        return True
    segments = value.split(".")
    if len(segments) == 3 and segments[0].startswith("eyJ") and all(JWT_SEGMENT.fullmatch(segment) for segment in segments):
        return True
    return _looks_like_high_entropy_token(value)


def _sensitive_value(value: str) -> bool:
    return bool(
        _looks_like_credential_value(value)
        or EMAIL.search(value)
        or _looks_like_path(value)
        or SECRET.search(value)
        or MRN.search(value)
        or SSN.search(value)
        or DOB.search(value)
        or CLINICAL_TEXT.search(value)
    )


def _redacted_text(value: str) -> str:
    return f"[REDACTED:{digest(value)[:16]}]"


def _bounded_projection(value: Any, *, omit_volatile: bool) -> tuple[Any, bool]:
    budget = _TraversalBudget()

    def visit(current: Any, key: str, depth: int) -> Any:
        if not budget.consume_node(depth):
            return None
        if _sensitive_key(key):
            return None
        if isinstance(current, Mapping):
            result: dict[str, Any] = {}
            try:
                entries = current.items()
                for raw_key, child in entries:
                    if not budget.consume_mapping_entry():
                        break
                    name = _safe_text(raw_key)
                    if name is None or budget.consume_string(name) is None:
                        break
                    lowered = name.lower()
                    if (omit_volatile and lowered in VOLATILE) or _sensitive_key(name):
                        continue
                    projected = visit(child, name, depth + 1)
                    if projected is not None:
                        result[name] = projected
            except Exception:
                budget.truncated = True
            return result
        if isinstance(current, list):
            result: list[Any] = []
            try:
                for child in current:
                    if not budget.consume_list_item():
                        break
                    projected = visit(child, key, depth + 1)
                    if projected is not None:
                        result.append(projected)
            except Exception:
                budget.truncated = True
            return result
        if isinstance(current, str):
            bounded = budget.consume_string(current)
            if bounded is None:
                return None
            return _redacted_text(bounded) if _sensitive_value(bounded) else bounded
        if current is None or isinstance(current, bool):
            return current
        if isinstance(current, (int, float)):
            if finite(current):
                return current
            budget.truncated = True
            return None
        return None

    projected = visit(value, "", 0)
    return projected, budget.truncated


def sanitize(value: Any, key: str = "") -> Any:
    projected, _truncated = _bounded_projection({key: value} if key else value, omit_volatile=False)
    if key:
        return projected.get(key) if isinstance(projected, Mapping) else None
    return projected


def normalize(value: Any) -> Any:
    projected, _truncated = _bounded_projection(value, omit_volatile=True)
    if isinstance(projected, Mapping):
        return {str(k): projected[k] for k in sorted(projected, key=str)}
    return projected


def _semantic_projection(value: Any) -> tuple[Any, bool]:
    projected, sanitized_truncated = _bounded_projection(value, omit_volatile=True)
    if isinstance(projected, Mapping):
        projected = {str(k): projected[k] for k in sorted(projected, key=str)}
    return projected, sanitized_truncated


def _bounded_diff_value(value: Any) -> Any:
    # This is called only after a difference is detected.  It is intentionally
    # bounded and never returns the original sensitive value.
    projected = sanitize(value)
    return projected if projected is not None else "[REDACTED]"


def _diff_key_token(key: Any, *, key_text: str | None = None, budget: _TraversalBudget | None = None) -> str | None:
    text = key_text if key_text is not None else _safe_text(key)
    if text is None:
        if budget is not None:
            budget.truncated = True
        return None
    if budget is not None and budget.consume_string(text) is None:
        return None
    return f"k_{digest(text)[:12]}"


def _bounded_structural_diff(left: Any, right: Any, *, max_diffs: int = MAX_PARITY_DIFFS, max_depth: int = MAX_PARITY_DIFF_DEPTH) -> dict[str, Any]:
    differences: list[dict[str, Any]] = []
    truncated = False
    budget = _TraversalBudget(max_nodes=MAX_DIFF_NODES, max_depth=max_depth)

    def add(path: str, left_value: Any, right_value: Any) -> None:
        nonlocal truncated
        if len(differences) >= max_diffs:
            truncated = True
            return
        differences.append({"path": path, "left": _bounded_diff_value(left_value), "right": _bounded_diff_value(right_value)})

    def bounded_records(mapping: Mapping[Any, Any]) -> tuple[list[tuple[Any, str, Any]], bool]:
        records: list[tuple[Any, str, Any]] = []
        try:
            for raw_key, child in mapping.items():
                if not budget.consume_mapping_entry():
                    return records, True
                key_text = _safe_text(raw_key)
                if key_text is None or budget.consume_string(key_text) is None:
                    return records, True
                records.append((raw_key, key_text, child))
        except Exception:
            budget.truncated = True
            return records, True
        return records, False

    def visit(left_value: Any, right_value: Any, path: str, depth: int) -> None:
        nonlocal truncated
        if len(differences) >= max_diffs:
            truncated = True
            return
        if not budget.consume_node(depth):
            truncated = True
            return
        if depth >= max_depth:
            if isinstance(left_value, (Mapping, list)) or isinstance(right_value, (Mapping, list)):
                truncated = True
                return
            if isinstance(left_value, str) and budget.consume_string(left_value) is None:
                truncated = True
                return
            if isinstance(right_value, str) and budget.consume_string(right_value) is None:
                truncated = True
                return
            if isinstance(left_value, int) and not finite(left_value) or isinstance(right_value, int) and not finite(right_value):
                truncated = True
                return
            try:
                different = left_value != right_value
            except Exception:
                truncated = True
                return
            if different:
                add(path, left_value, right_value)
            else:
                truncated = True
            return
        if isinstance(left_value, Mapping) and isinstance(right_value, Mapping):
            left_records, left_truncated = bounded_records(left_value)
            right_records, right_truncated = bounded_records(right_value)
            if left_truncated or right_truncated:
                truncated = True
            left_by_key = {key: (text, child) for key, text, child in left_records}
            right_by_key = {key: (text, child) for key, text, child in right_records}
            try:
                def key_sort(key: Any) -> str:
                    record = left_by_key.get(key)
                    if record is None:
                        record = right_by_key[key]
                    return record[0]

                keys = sorted(set(left_by_key) | set(right_by_key), key=key_sort)
            except Exception:
                truncated = True
                return
            for key in keys:
                if len(differences) >= max_diffs:
                    truncated = True
                    return
                record = left_by_key.get(key) or right_by_key.get(key)
                token = _diff_key_token(key, key_text=record[0], budget=None)
                if token is None:
                    truncated = True
                    return
                child_path = f"{path}.{token}"
                if key not in left_by_key:
                    add(child_path, None, right_by_key[key][1])
                elif key not in right_by_key:
                    add(child_path, left_by_key[key][1], None)
                else:
                    visit(left_by_key[key][1], right_by_key[key][1], child_path, depth + 1)
            return
        if isinstance(left_value, list) and isinstance(right_value, list):
            try:
                left_length, right_length = len(left_value), len(right_value)
                common = min(left_length, right_length, MAX_PROJECTION_LIST_ITEMS)
                if left_length > common or right_length > common:
                    truncated = True
                for index in range(common):
                    if len(differences) >= max_diffs or not budget.consume_list_item():
                        truncated = True
                        return
                    visit(left_value[index], right_value[index], f"{path}.[{index}]", depth + 1)
                if left_length != right_length and common < MAX_PROJECTION_LIST_ITEMS:
                    add(f"{path}.length", left_length, right_length)
            except Exception:
                truncated = True
            return
        if isinstance(left_value, str) and budget.consume_string(left_value) is None:
            truncated = True
            return
        if isinstance(right_value, str) and budget.consume_string(right_value) is None:
            truncated = True
            return
        if isinstance(left_value, int) and not finite(left_value) or isinstance(right_value, int) and not finite(right_value):
            truncated = True
            return
        try:
            different = left_value != right_value
        except Exception:
            truncated = True
            return
        if different:
            add(path, left_value, right_value)

    visit(left, right, "$", 0)
    truncated = truncated or budget.truncated
    return {"status": "DIFF" if differences else ("TRUNCATED" if truncated else "EQUAL"), "count": len(differences), "truncated": truncated, "items": differences}


def _find_preview_contract(value: Any, depth: int = 0, budget: _TraversalBudget | None = None) -> tuple[list[Mapping[str, Any]], Mapping[str, Any]] | None:
    budget = budget or _TraversalBudget()
    if not budget.consume_node(depth):
        return None
    if isinstance(value, Mapping):
        try:
            preview = value.get("preview")
        except Exception:
            budget.truncated = True
            return None
        if isinstance(preview, list):
            items: list[Mapping[str, Any]] = []
            try:
                for item in preview:
                    if not budget.consume_list_item() or not budget.consume_node(depth + 1):
                        break
                    if isinstance(item, Mapping):
                        items.append(item)
            except Exception:
                budget.truncated = True
            return items, value
        try:
            for raw_key, child in value.items():
                if not budget.consume_mapping_entry():
                    break
                key = _safe_text(raw_key)
                if key is None or budget.consume_string(key) is None:
                    break
                found = _find_preview_contract(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    elif isinstance(value, list):
        try:
            for child in value:
                if not budget.consume_list_item():
                    break
                found = _find_preview_contract(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    return None


def _find_sections(value: Any, depth: int = 0, budget: _TraversalBudget | None = None) -> Mapping[str, Any] | None:
    """Compatibility reader for the old map-shaped test fixture."""
    budget = budget or _TraversalBudget()
    if not budget.consume_node(depth):
        return None
    if isinstance(value, Mapping):
        try:
            if all(section in value for section in SECTIONS):
                return value
            for raw_key, child in value.items():
                if not budget.consume_mapping_entry():
                    break
                key = _safe_text(raw_key)
                if key is None or budget.consume_string(key) is None:
                    break
                found = _find_sections(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    elif isinstance(value, list):
        try:
            for child in value:
                if not budget.consume_list_item():
                    break
                found = _find_sections(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    return None


def _find_expansions(value: Any, depth: int = 0, budget: _TraversalBudget | None = None) -> list[Mapping[str, Any]] | None:
    budget = budget or _TraversalBudget()
    if not budget.consume_node(depth):
        return None
    if isinstance(value, Mapping):
        try:
            expansions = value.get("expansions")
            if isinstance(expansions, list):
                items: list[Mapping[str, Any]] = []
                for item in expansions:
                    if not budget.consume_list_item() or not budget.consume_node(depth + 1):
                        break
                    if isinstance(item, Mapping):
                        items.append(item)
                return items
            for raw_key, child in value.items():
                if not budget.consume_mapping_entry():
                    break
                key = _safe_text(raw_key)
                if key is None or budget.consume_string(key) is None:
                    break
                found = _find_expansions(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    elif isinstance(value, list):
        try:
            for child in value:
                if not budget.consume_list_item():
                    break
                found = _find_expansions(child, depth + 1, budget)
                if found is not None:
                    return found
        except Exception:
            budget.truncated = True
    return None


def _section_explanation(plan: Mapping[str, Any], section: str, budget: _TraversalBudget | None = None) -> str | None:
    budget = budget or _TraversalBudget()
    for key in ("omissions", "fallbacks"):
        try:
            values = plan.get(key)
        except Exception:
            budget.truncated = True
            return None
        if not isinstance(values, list):
            continue
        try:
            for item in values:
                if not budget.consume_list_item() or not budget.consume_node(1):
                    return None
                if isinstance(item, Mapping) and item.get("section") == section and isinstance(item.get("reason"), str) and item["reason"].strip():
                    return item["reason"].strip()[:MAX_PROJECTION_STRING_BYTES]
        except Exception:
            budget.truncated = True
            return None
    return None


def _preview_check(payload: Any, budget: _TraversalBudget | None = None) -> dict[str, Any]:
    budget = budget or _TraversalBudget()
    found = _find_preview_contract(payload, budget=budget)
    if found is not None:
        preview_items, plan = found
        by_section: dict[str, Mapping[str, Any]] = {}
        duplicates: list[str] = []
        for item in preview_items:
            section = item.get("section")
            if isinstance(section, str) and section in SECTIONS:
                if section in by_section:
                    duplicates.append(section)
                by_section[section] = item
        missing = [section for section in SECTIONS if section not in by_section]
        unavailable: dict[str, str] = {}
        unexplained: list[str] = []
        available: list[str] = []
        for section in SECTIONS:
            item = by_section.get(section)
            if item is None:
                continue
            items = item.get("items")
            count = item.get("count")
            if isinstance(items, list) and items:
                available.append(section)
            elif finite(count) and float(count) > 0:
                available.append(section)
            elif isinstance(item.get("omission"), str) and item["omission"].strip():
                unavailable[section] = item["omission"].strip()[:512]
            else:
                explanation = _section_explanation(plan, section, budget)
                if explanation:
                    unavailable[section] = explanation
                else:
                    unexplained.append(section)
                    unavailable[section] = "section has no usable items and no explicit omission/fallback"
        expansions = _find_expansions(plan, budget=budget) or []
        invalid_expansions = []
        valid_expansions = []
        for item in expansions:
            command = item.get("command")
            reason = item.get("reason")
            if isinstance(command, str) and command.strip().startswith("mi-lsp nav ") and isinstance(reason, str) and reason.strip():
                valid_expansions.append(item)
            else:
                invalid_expansions.append({"has_command": isinstance(command, str) and bool(command.strip()), "has_reason": isinstance(reason, str) and bool(reason.strip()), "command_prefix": isinstance(command, str) and command.strip().startswith("mi-lsp nav ")})
        missing_expansion_fields = [field for field in ("command", "reason") if not any(isinstance(item.get(field), str) and item[field].strip() for item in expansions)]
        complete = not missing and not duplicates and not unexplained and bool(expansions) and not invalid_expansions
        if budget.truncated:
            complete = False
        result = {
            "status": "PASS" if complete else "FAIL",
            "sections": [section for section in SECTIONS if section in by_section],
            "missing_sections": missing,
            "available_sections": available,
            "omissions": unavailable,
            "duplicates": duplicates,
            "truncated": budget.truncated,
            "reason_code": "projection_truncated" if budget.truncated else None,
            "expansions": {"required": ["command", "reason"], "complete": complete and bool(valid_expansions), "missing": missing_expansion_fields, "invalid": invalid_expansions, "valid": len(valid_expansions)},
        }
        return result

    section_map = _find_sections(payload, budget=budget)
    expansions = _find_expansions(payload, budget=budget) or []
    missing = [section for section in SECTIONS if section_map is None or section not in section_map]
    unavailable = {} if section_map is None else {section: "section shape is legacy map; no explicit availability evidence" for section in SECTIONS if not section_map.get(section)}
    valid_expansions = [item for item in expansions if isinstance(item.get("command"), str) and item["command"].strip().startswith("mi-lsp nav ") and isinstance(item.get("reason"), str) and item["reason"].strip()]
    invalid_expansions = [item for item in expansions if item not in valid_expansions]
    missing_expansion_fields = [field for field in ("command", "reason") if not any(isinstance(item.get(field), str) and item[field].strip() for item in expansions)]
    complete = not missing and not unavailable and bool(expansions) and not invalid_expansions and not budget.truncated
    return {"status": "PASS" if complete else "FAIL", "sections": list(SECTIONS) if section_map is not None and not missing else [section for section in SECTIONS if section_map and section in section_map], "missing_sections": missing, "available_sections": [], "omissions": unavailable, "duplicates": [], "truncated": budget.truncated, "reason_code": "projection_truncated" if budget.truncated else None, "expansions": {"required": ["command", "reason"], "complete": complete and bool(valid_expansions), "missing": missing_expansion_fields, "invalid": invalid_expansions, "valid": len(valid_expansions)}}


def _freshness_records(value: Any, depth: int = 0, budget: _TraversalBudget | None = None) -> tuple[list[float], list[Mapping[str, Any]], list[list[Any]]]:
    budget = budget or _TraversalBudget()
    ranks: list[float] = []
    graph: list[Mapping[str, Any]] = []
    rank_lists: list[list[Any]] = []

    def truncated() -> tuple[list[float], list[Mapping[str, Any]], list[list[Any]]]:
        budget.truncated = True
        return ranks, graph + [{"state": FRESHNESS_TRUNCATED_STATE}], rank_lists

    if not budget.consume_node(depth):
        return truncated()
    if isinstance(value, Mapping):
        try:
            freshness_rank = value.get("freshness_rank")
            if finite(freshness_rank):
                ranks.append(float(freshness_rank))
            freshness = value.get("graph_freshness")
            if isinstance(freshness, Mapping):
                graph.append(freshness)
            graph_ranks = value.get("graph_ranks")
            if isinstance(graph_ranks, list):
                bounded_ranks: list[Any] = []
                for rank in graph_ranks:
                    if not budget.consume_list_item() or not budget.consume_node(depth + 1):
                        return truncated()
                    bounded_ranks.append(rank)
                rank_lists.append(bounded_ranks)
            for raw_key, child in value.items():
                if not budget.consume_mapping_entry():
                    return truncated()
                key = _safe_text(raw_key)
                if key is None or budget.consume_string(key) is None:
                    return truncated()
                child_ranks, child_graph, child_lists = _freshness_records(child, depth + 1, budget)
                ranks.extend(child_ranks)
                graph.extend(child_graph)
                rank_lists.extend(child_lists)
        except Exception:
            return truncated()
    elif isinstance(value, list):
        try:
            for child in value:
                if not budget.consume_list_item():
                    return truncated()
                child_ranks, child_graph, child_lists = _freshness_records(child, depth + 1, budget)
                ranks.extend(child_ranks)
                graph.extend(child_graph)
                rank_lists.extend(child_lists)
        except Exception:
            return truncated()
    return ranks, graph, rank_lists


def _kind_check(query: Mapping[str, Any], payload: Any, budget: _TraversalBudget | None = None) -> tuple[bool, str | None]:
    budget = budget or _TraversalBudget()
    try:
        items = payload.get("items") if isinstance(payload, Mapping) else None
    except Exception:
        budget.truncated = True
        return False, "items must be a non-empty list"
    if not isinstance(payload, Mapping) or not isinstance(items, list) or not items:
        return False, "items must be a non-empty list"
    kind = query.get("kind")
    try:
        bounded_items = []
        for item in items:
            if not budget.consume_list_item() or not budget.consume_node(1):
                break
            bounded_items.append(item)
    except Exception:
        budget.truncated = True
        return False, "items must be a non-empty list"
    if kind == "wiki_pack":
        for item in bounded_items:
            if not isinstance(item, Mapping):
                continue
            docs = item.get("docs")
            if isinstance(docs, list):
                for doc in docs:
                    if not budget.consume_list_item() or not budget.consume_node(2):
                        break
                    if isinstance(doc, Mapping) and isinstance(doc.get("path"), str) and doc["path"].strip():
                        return (False, "projection traversal truncated") if budget.truncated else (True, None)
            for key in ("evidence", "evidence_paths", "evidence_paths_used"):
                evidence = item.get(key)
                if isinstance(evidence, list):
                    for path in evidence:
                        if not budget.consume_list_item() or not budget.consume_node(2):
                            break
                        if isinstance(path, str) and path.strip():
                            return (False, "projection traversal truncated") if budget.truncated else (True, None)
        return False, "wiki pack has no usable docs/evidence"
    if kind == "explain_change":
        return (False, "projection traversal truncated") if budget.truncated else (True, None)
    if kind in {"workspace_map", "related"}:
        for item in bounded_items:
            if not isinstance(item, Mapping):
                continue
            freshness = item.get("graph_freshness")
            if not isinstance(freshness, Mapping) or freshness.get("state") not in FRESHNESS_STATES:
                return False, "graph_freshness.state is missing or invalid"
            if "graph_ranks" not in item or not isinstance(item.get("graph_ranks"), list):
                return False, "graph_ranks list is missing"
        return (False, "projection traversal truncated") if budget.truncated else (True, None)
    return (False, "projection traversal truncated") if budget.truncated else (True, None)


def _freshness_check(query: Mapping[str, Any], payload: Any, budget: _TraversalBudget | None = None) -> dict[str, Any]:
    required = bool(query.get("freshness_rank_required"))
    budget = budget or _TraversalBudget()
    ranks, graph, rank_lists = _freshness_records(payload, budget=budget) if required else ([], [], [])
    kind = query.get("kind")
    if not required:
        return {"required": False, "status": "NOT_REQUIRED", "observed": 0, "min": None, "max": None, "graph_states": [], "rank_lists": 0}
    graph_states = [item.get("state") for item in graph]
    current_only = bool(graph_states) and all(state == "current" for state in graph_states)
    if kind in {"workspace_map", "related"}:
        passed = current_only and bool(rank_lists)
    else:
        passed = current_only and (bool(ranks) or bool(rank_lists))
    if budget.truncated:
        passed = False
    observed = len(ranks) + len(graph)
    return {"required": True, "status": "PASS" if passed else "FAIL", "observed": observed, "min": min(ranks) if ranks else None, "max": max(ranks) if ranks else None, "graph_states": graph_states, "rank_lists": len(rank_lists), "truncated": budget.truncated, **({"reason_code": "projection_truncated"} if budget.truncated else {})}


def _preview_usefulness(preview: Mapping[str, Any], output_bytes: int | None, estimated_tokens: int | None) -> dict[str, Any]:
    if preview.get("status") == "NOT_REQUIRED":
        return {"status": "NOT_REQUIRED", "output_bytes": output_bytes, "estimated_tokens": estimated_tokens, "utility_ratio": None}
    section_ratio = len(preview.get("sections", [])) / len(SECTIONS)
    expansion = preview.get("expansions", {})
    expansion_ratio = 1.0 if expansion.get("complete") else 0.0
    return {"status": "PASS" if preview.get("status") == "PASS" else "FAIL", "output_bytes": output_bytes, "estimated_tokens": estimated_tokens, "section_ratio": section_ratio, "expansion_ratio": expansion_ratio, "utility_ratio": round((section_ratio + expansion_ratio) / 2.0, 4)}


def _sample_measurements_ok(output_bytes: int | None, estimated_tokens: int | None) -> bool:
    return finite(output_bytes) and float(output_bytes) > 0 and finite(estimated_tokens) and float(estimated_tokens) > 0


def query_check(query: Mapping[str, Any], payload: Any, *, output_bytes: int | None = None, estimated_tokens: int | None = None) -> dict[str, Any]:
    normalized, projection_truncated = _semantic_projection(payload)
    correct = isinstance(payload, Mapping) and payload.get("ok") is True and not projection_truncated
    expected = query.get("expected_digest")
    if expected and not projection_truncated:
        correct = correct and digest(normalized) == expected
    inspection_budget = _TraversalBudget()
    preview = {"required": bool(query.get("preview_required")), "status": "NOT_REQUIRED", "sections": [], "missing_sections": [], "expansions": {"required": ["command", "reason"], "complete": True}}
    if query.get("preview_required"):
        preview = {"required": True, **_preview_check(payload, inspection_budget)}
        correct = correct and preview["status"] == "PASS"
    freshness = _freshness_check(query, payload, inspection_budget)
    if freshness["required"]:
        correct = correct and freshness["status"] == "PASS"
    kind_ok, kind_reason = _kind_check(query, payload, inspection_budget)
    if query.get("kind") in SEMANTIC_KINDS:
        correct = correct and kind_ok
    measurements = {"status": "PASS" if _sample_measurements_ok(output_bytes, estimated_tokens) else "FAIL", "output_bytes": output_bytes, "estimated_tokens": estimated_tokens}
    correct = correct and measurements["status"] == "PASS"
    result = {
        "status": "PASS" if correct else "FAIL",
        "normalized_digest": None if projection_truncated else digest(normalized),
        "semantic_projection": {"status": "FAIL" if projection_truncated else "PASS", "truncated": projection_truncated, **({"reason_code": "projection_truncated"} if projection_truncated else {})},
        "preview": preview,
        "freshness_rank": freshness,
        "preview_usefulness": _preview_usefulness(preview, output_bytes, estimated_tokens),
        "sample_measurements": measurements,
    }
    if kind_reason:
        result["kind_schema"] = {"status": "FAIL", "reason": kind_reason}
    elif query.get("kind") in SEMANTIC_KINDS:
        result["kind_schema"] = {"status": "PASS"}
    return result


def worker_status_check(payload: Any) -> dict[str, Any]:
    result: dict[str, Any] = {"status": "FAIL", "evidence": "none"}
    if not isinstance(payload, Mapping):
        result["reason_code"] = "worker_schema"
        return result
    if payload.get("ok") is not True or payload.get("backend") != "worker":
        result["reason_code"] = "worker_schema"
        return result
    items = payload.get("items")
    if not isinstance(items, list) or not items or not isinstance(items[0], Mapping):
        result["reason_code"] = "worker_schema"
        return result
    item = items[0]
    compatible = item.get("selected_compatible")
    if not isinstance(compatible, bool):
        result["reason_code"] = "worker_schema"
        return result
    if compatible:
        usable = isinstance(item.get("selected_source"), str) and bool(item["selected_source"].strip()) and isinstance(item.get("selected_path"), str) and bool(item["selected_path"].strip())
        result["status"] = "PASS" if usable else "FAIL"
        result["evidence"] = "usable" if usable else "none"
    else:
        terminal = isinstance(item.get("selected_error"), str) and bool(item["selected_error"].strip())
        result["status"] = "FAIL"
        result["evidence"] = "terminal_unusable" if terminal else "none"
    if result["status"] != "PASS":
        result["reason_code"] = "worker_schema"
    return result


@dataclass
class ProcessResult:
    returncode: int
    elapsed_ms: float
    payload: Any = None
    rss_bytes: int | None = None
    reason_code: str | None = None
    output_bytes: int = 0
    estimated_tokens: int = 0


class _RSSSampler:
    def __init__(self, process: subprocess.Popen[str]) -> None:
        self.process = process
        self.peak: int | None = None
        self.stop_event = threading.Event()
        self.thread: threading.Thread | None = None
        self.failure = False
        try:
            import psutil  # type: ignore
            self.psutil = psutil
        except ImportError:
            self.psutil = None

    @property
    def available(self) -> bool:
        return self.psutil is not None and not self.failure

    def start(self) -> None:
        if self.psutil is not None:
            self.thread = threading.Thread(target=self._sample, daemon=True)
            self.thread.start()

    def _is_no_such_process(self, error: BaseException) -> bool:
        no_such_process = getattr(self.psutil, "NoSuchProcess", None)
        return isinstance(no_such_process, type) and isinstance(error, no_such_process)

    def _sample(self) -> None:
        try:
            root = self.psutil.Process(self.process.pid)
        except Exception as error:
            if not self._is_no_such_process(error):
                self.failure = True
            return
        while not self.stop_event.is_set():
            processes = [root]
            try:
                processes += root.children(recursive=True)
            except Exception as error:
                if self._is_no_such_process(error):
                    # A child/root disappearing between observations is a normal
                    # process-lifecycle race; keep only completed observations.
                    return
                self.failure = True
                return
            current = 0
            for proc in processes:
                try:
                    current += int(proc.memory_info().rss)
                except Exception as error:
                    if self._is_no_such_process(error):
                        continue
                    self.failure = True
                    return
            if current:
                self.peak = max(self.peak or 0, current)
            time.sleep(0.01)

    def stop(self) -> int | None:
        self.stop_event.set()
        if self.thread is not None:
            self.thread.join(timeout=1.0)
        return self.peak if self.available and self.peak is not None else None


def run_process(argv: Sequence[str], timeout_seconds: float) -> ProcessResult:
    started = time.perf_counter()
    try:
        process = subprocess.Popen(list(argv), stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, encoding="utf-8", errors="replace")
    except OSError as exc:
        raise HarnessError("spawn_error") from exc
    sampler = _RSSSampler(process)
    sampler.start()
    try:
        stdout, _stderr = process.communicate(timeout=timeout_seconds)
    except subprocess.TimeoutExpired:
        process.kill()
        process.communicate()
        return ProcessResult(process.returncode or 124, (time.perf_counter() - started) * 1000.0, rss_bytes=sampler.stop(), reason_code="timeout")
    rss = sampler.stop()
    elapsed = (time.perf_counter() - started) * 1000.0
    output_bytes = len(stdout.encode("utf-8", "replace"))
    estimated_tokens = math.ceil(output_bytes / 4) if output_bytes else 0
    if process.returncode != 0:
        return ProcessResult(process.returncode, elapsed, rss_bytes=rss, reason_code="nonzero_exit", output_bytes=output_bytes, estimated_tokens=estimated_tokens)
    try:
        payload = json.loads(stdout)
    except json.JSONDecodeError:
        return ProcessResult(process.returncode, elapsed, rss_bytes=rss, reason_code="decode_error", output_bytes=output_bytes, estimated_tokens=estimated_tokens)
    return ProcessResult(process.returncode, elapsed, payload=payload, rss_bytes=rss, output_bytes=output_bytes, estimated_tokens=estimated_tokens)


def _command(binary: Path, source_root: Path, campaign_id: str, query: Mapping[str, Any], mode: str) -> list[str]:
    command = [str(binary), "--workspace", str(source_root), "--format", "json", "--client-name", "harness-first", "--session-id", campaign_id]
    if mode == "direct":
        command.append("--no-daemon")
    return command + list(query["args"]) + list(query.get(f"{mode}_args", []))


def source_revision(source_root: Path) -> str:
    try:
        status = subprocess.run(["git", "-C", str(source_root), "status", "--porcelain"], capture_output=True, text=True, timeout=10, check=True)
        if status.stdout.strip():
            raise HarnessError("source_dirty")
        result = subprocess.run(["git", "-C", str(source_root), "rev-parse", "HEAD"], capture_output=True, text=True, timeout=10, check=True)
    except HarnessError:
        raise
    except (OSError, subprocess.SubprocessError) as exc:
        raise HarnessError("missing_source_sha") from exc
    value = result.stdout.strip().lower()
    if not SOURCE_REVISION.fullmatch(value):
        raise HarnessError("missing_source_sha")
    return value


# Backwards-compatible name retained for callers that used the original runner.
def source_sha(source_root: Path) -> str:
    return source_revision(source_root)


def parse_go_version_m(text: str) -> dict[str, Any]:
    revision = None
    modified: bool | None = None
    for line in text.splitlines():
        match = re.match(r"^\s*(?:build\s+)?vcs\.revision(?:=|\s+)([0-9a-fA-F]{40}|[0-9a-fA-F]{64})\s*$", line)
        if match:
            revision = match.group(1).lower()
        modified_match = re.match(r"^\s*(?:build\s+)?vcs\.modified(?:=|\s+)(true|false)\s*$", line, re.IGNORECASE)
        if modified_match:
            modified = modified_match.group(1).lower() == "true"
    return {"revision": revision, "modified": modified, "parsed": revision is not None and modified is not None}


def provenance(binary: Path, expected_revision: str | None = None) -> dict[str, Any]:
    result: dict[str, Any] = {
        "status": "NOT_RUN",
        "binary_sha256": file_digest(binary),
        "source_revision": expected_revision,
        "go_version_m": {"status": "NOT_RUN", "revision": None, "modified": None, "parsed": False, "reason_code": "provenance_unavailable"},
    }
    go = shutil.which("go")
    if not go:
        return result
    try:
        version = subprocess.run([go, "version", "-m", str(binary)], capture_output=True, text=True, timeout=20)
    except (OSError, subprocess.SubprocessError):
        return result
    parsed = parse_go_version_m(version.stdout) if version.returncode == 0 else {"revision": None, "modified": None, "parsed": False}
    matches = bool(expected_revision and parsed["revision"] == expected_revision)
    valid = version.returncode == 0 and parsed["parsed"] and parsed["modified"] is False and (expected_revision is None or matches)
    result["go_version_m"] = {"status": "PASS" if valid else "FAIL", "revision": parsed["revision"], "modified": parsed["modified"], "parsed": parsed["parsed"], "revision_match": matches if expected_revision else None, "reason_code": None if valid else "provenance_unavailable"}
    result["status"] = "PASS" if valid else "FAIL"
    return result


def _worker_status_probe(binary: Path, source_root: Path, campaign_id: str, worker_args: Sequence[str], timeout_seconds: float) -> dict[str, Any]:
    argv = [
        str(binary),
        "--workspace",
        str(source_root),
        "--format",
        "json",
        "--client-name",
        "harness-first",
        "--session-id",
        campaign_id,
        "--no-daemon",
        *worker_args,
    ]
    try:
        result = run_process(argv, timeout_seconds)
    except HarnessError:
        return {"status": "FAIL", "usable": False, "attempts": 1, "elapsed_ms": None, "reason_code": "worker_preflight_failed"}
    report: dict[str, Any] = {
        "status": "FAIL",
        "usable": False,
        "attempts": 1,
        "elapsed_ms": result.elapsed_ms,
        "peak_rss_bytes": result.rss_bytes,
        "output_bytes": result.output_bytes,
        "estimated_tokens": result.estimated_tokens,
    }
    if result.reason_code is not None:
        report["reason_code"] = "worker_preflight_failed"
        return report
    report.update(worker_status_check(result.payload))
    report["usable"] = report["status"] == "PASS"
    report["payload_digest"] = digest(normalize(sanitize(result.payload)))
    if not report["usable"]:
        report["reason_code"] = "worker_preflight_failed"
    return report


def _daemon_state(payload: Any) -> Mapping[str, Any] | None:
    if not isinstance(payload, Mapping) or payload.get("ok") is not True:
        return None
    items = payload.get("items")
    if not isinstance(items, list) or not items or not isinstance(items[0], Mapping):
        return None
    state = items[0].get("state")
    return state if isinstance(state, Mapping) else None


def _version_identity(payload: Any, expected_sha256: str) -> dict[str, Any]:
    item: Mapping[str, Any] | None = None
    if isinstance(payload, Mapping) and payload.get("ok") is True and isinstance(payload.get("items"), list):
        first = payload["items"][0] if payload["items"] else None
        if isinstance(first, Mapping):
            item = first
    version = item.get("version") if item else None
    protocol = item.get("protocol_version") if item else None
    observed_sha256 = item.get("executable_sha256") if item else None
    version_ok = isinstance(version, str) and bool(version.strip()) and not _sensitive_value(version)
    protocol_ok = protocol == PROTOCOL_VERSION
    hash_ok = isinstance(observed_sha256, str) and SHA256.fullmatch(observed_sha256) is not None and observed_sha256 == expected_sha256
    return {
        "status": "PASS" if version_ok and protocol_ok and hash_ok else "FAIL",
        "version": version[:MAX_PROJECTION_STRING_BYTES] if version_ok else None,
        "protocol_version": protocol if protocol_ok else None,
        "executable_sha256": observed_sha256 if hash_ok else None,
        "version_present": version_ok,
        "protocol_compatible": protocol_ok,
        "identity_match": hash_ok,
        "reason_code": None if version_ok and protocol_ok and hash_ok else "candidate_version_failed",
    }


def _version_probe(binary: Path, timeout_seconds: float, expected_sha256: str) -> dict[str, Any]:
    try:
        result = run_process([str(binary), "--format", "json", "version"], timeout_seconds)
    except HarnessError:
        return {"status": "FAIL", "elapsed_ms": None, "reason_code": "candidate_version_failed", "version": None, "protocol_version": None, "executable_sha256": None}
    report = _version_identity(result.payload if result.reason_code is None else None, expected_sha256)
    report["elapsed_ms"] = result.elapsed_ms
    if result.reason_code is not None:
        report["status"] = "FAIL"
        report["reason_code"] = "candidate_version_failed"
    return report


def _daemon_identity_check(payload: Any, expected_sha256: str, candidate: Mapping[str, Any] | None = None) -> dict[str, Any]:
    state = _daemon_state(payload)
    if state is None:
        return {
            "status": "FAIL",
            "daemon_identity_match": False,
            "protocol_compatible": False,
            "version_compatible": False,
            "version_match": False,
            "reason_code": "daemon_preflight_failed",
        }
    observed_sha256 = state.get("executable_sha256")
    protocol_version = state.get("protocol_version")
    version = state.get("version")
    identity_match = isinstance(observed_sha256, str) and observed_sha256 == expected_sha256
    protocol_compatible = protocol_version == PROTOCOL_VERSION
    version_present = isinstance(version, str) and bool(version.strip()) and not _sensitive_value(version)
    version_match = candidate is None or (version_present and version == candidate.get("version"))
    version_compatible = version_present and version_match
    if not identity_match:
        reason_code = "daemon_identity_mismatch"
    elif not protocol_compatible or not version_compatible:
        reason_code = "candidate_version_mismatch" if candidate is not None and not version_match else "daemon_protocol_mismatch"
    else:
        reason_code = None
    return {
        "status": "PASS" if reason_code is None else "FAIL",
        "daemon_identity_match": identity_match,
        "protocol_compatible": protocol_compatible,
        "version_compatible": version_compatible,
        "version_match": version_match,
        "protocol_version": protocol_version if protocol_compatible else None,
        "version": version[:MAX_PROJECTION_STRING_BYTES] if version_present else None,
        "executable_sha256": observed_sha256 if identity_match else None,
        "version_present": version_present,
        "protocol_match": protocol_compatible,
        **({} if reason_code is None else {"reason_code": reason_code}),
    }


def _daemon_preflight(binary: Path, expected_sha256: str, timeout_seconds: float, candidate: Mapping[str, Any] | None = None) -> dict[str, Any]:
    started = time.perf_counter()
    stop_report: dict[str, Any] = {"status": "IGNORED", "best_effort": True}
    try:
        stop_result = run_process([str(binary), "--format", "json", "daemon", "stop"], timeout_seconds)
        stop_report["status"] = "PASS" if stop_result.reason_code is None else "IGNORED"
        stop_report["elapsed_ms"] = stop_result.elapsed_ms
    except HarnessError:
        stop_report["status"] = "IGNORED"
        stop_report["elapsed_ms"] = None

    try:
        start_result = run_process([str(binary), "--format", "json", "daemon", "start"], timeout_seconds)
    except HarnessError:
        return {
            "status": "BLOCKED",
            "reason_code": "daemon_preflight_failed",
            "daemon_identity_match": False,
            "elapsed_ms": (time.perf_counter() - started) * 1000.0,
            "stop": stop_report,
            "start": {"status": "FAIL", "elapsed_ms": None},
            "status_probe": {"status": "NOT_RUN"},
        }
    start_ok = start_result.reason_code is None and isinstance(start_result.payload, Mapping) and start_result.payload.get("ok") is True
    if not start_ok:
        return {
            "status": "BLOCKED",
            "reason_code": "daemon_preflight_failed",
            "daemon_identity_match": False,
            "elapsed_ms": (time.perf_counter() - started) * 1000.0,
            "stop": stop_report,
            "start": {"status": "FAIL", "elapsed_ms": start_result.elapsed_ms},
            "status_probe": {"status": "NOT_RUN"},
        }

    try:
        status_result = run_process([str(binary), "--format", "json", "daemon", "status"], timeout_seconds)
    except HarnessError:
        return {
            "status": "BLOCKED",
            "reason_code": "daemon_preflight_failed",
            "daemon_identity_match": False,
            "elapsed_ms": (time.perf_counter() - started) * 1000.0,
            "stop": stop_report,
            "start": {"status": "PASS", "elapsed_ms": start_result.elapsed_ms},
            "status_probe": {"status": "FAIL", "elapsed_ms": None},
        }
    identity = _daemon_identity_check(status_result.payload if status_result.reason_code is None else None, expected_sha256, candidate)
    return {
        "status": "PASS" if identity["status"] == "PASS" else "BLOCKED",
        "reason_code": identity.get("reason_code"),
        "daemon_identity_match": identity["daemon_identity_match"],
        "protocol_compatible": identity["protocol_compatible"],
        "version_compatible": identity["version_compatible"],
        "protocol_version": identity.get("protocol_version"),
        "version": identity.get("version"),
        "executable_sha256": identity.get("executable_sha256"),
        "version_match": identity.get("version_match", False),
        "version_present": identity.get("version_present", False),
        "elapsed_ms": (time.perf_counter() - started) * 1000.0,
        "stop": stop_report,
        "start": {"status": "PASS", "elapsed_ms": start_result.elapsed_ms},
        "status_probe": {"status": "PASS" if status_result.reason_code is None else "FAIL", "elapsed_ms": status_result.elapsed_ms},
    }


def _candidate_preflight(binary: Path, source_root: Path, campaign_id: str, worker_args: Sequence[str], expected_sha256: str, timeout_seconds: float, *, daemon_required: bool) -> dict[str, Any]:
    started = time.perf_counter()
    if worker_args != ["worker", "status"]:
        raise HarnessError("invalid_manifest")
    version = _version_probe(binary, timeout_seconds, expected_sha256)
    if version["status"] != "PASS":
        return {
            "status": "BLOCKED",
            "reason_code": version.get("reason_code", "candidate_version_failed"),
            "worker_usable": False,
            "daemon_identity_match": None,
            "worker_elapsed_ms": None,
            "daemon_elapsed_ms": None,
            "elapsed_ms": (time.perf_counter() - started) * 1000.0,
            "version": version,
            "worker": {"status": "NOT_RUN"},
            "daemon": {"status": "NOT_RUN", "daemon_identity_match": None},
        }
    worker = _worker_status_probe(binary, source_root, campaign_id, ["worker", "status"], timeout_seconds)
    if worker["status"] != "PASS":
        return {
            "status": "BLOCKED",
            "reason_code": "worker_preflight_failed",
            "worker_usable": False,
            "daemon_identity_match": None,
            "worker_elapsed_ms": worker.get("elapsed_ms"),
            "daemon_elapsed_ms": None,
            "elapsed_ms": (time.perf_counter() - started) * 1000.0,
            "version": version,
            "worker": worker,
            "daemon": {"status": "NOT_RUN", "daemon_identity_match": None},
        }
    if not daemon_required:
        daemon = {"status": "NOT_REQUIRED", "daemon_identity_match": None, "elapsed_ms": 0.0}
    else:
        daemon = _daemon_preflight(binary, expected_sha256, timeout_seconds, version)
    return {
        "status": "PASS" if daemon["status"] in {"PASS", "NOT_REQUIRED"} else "BLOCKED",
        "reason_code": daemon.get("reason_code"),
        "worker_usable": True,
        "daemon_identity_match": daemon.get("daemon_identity_match"),
        "worker_elapsed_ms": worker.get("elapsed_ms"),
        "daemon_elapsed_ms": daemon.get("elapsed_ms"),
        "elapsed_ms": (time.perf_counter() - started) * 1000.0,
        "version": version,
        "worker": worker,
        "daemon": daemon,
    }


def _write_blocked_report(output_path: Path, marker_path: Path, contract: Mapping[str, Any], source: str, manifest_hash: str, provenance_report: Mapping[str, Any], candidate_preflight: Mapping[str, Any], started: float) -> dict[str, Any]:
    reason_code = str(candidate_preflight.get("reason_code", "native_error"))
    report = {
        "schema": REPORT_SCHEMA,
        "campaign_id": contract["campaign_id"],
        "status": "BLOCKED",
        "source_revision": source,
        "manifest_sha256": manifest_hash,
        "provenance": provenance_report,
        "budgets": contract["budgets"],
        "candidate_preflight": candidate_preflight,
        "worker_status": candidate_preflight.get("worker", {"status": "NOT_RUN"}),
        "samples": [],
        "parity": {"required_queries": [query["id"] for query in contract["queries"] if query.get("parity_required")], "results": {}, "status": "BLOCKED", "diagnostics": {}},
        "duration_ms": (time.perf_counter() - started) * 1000.0,
        "sanitized": True,
        "comparators": {"graphify": "not_used"},
        "failure_reasons": [reason_code if reason_code in REASONS else "native_error"],
    }
    (output_path / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    (output_path / "report.yaml").write_text(to_yaml(report), encoding="utf-8")
    finish_marker(marker_path, "BLOCKED")
    return report


def _registry_path(source_root: Path) -> Path:
    root = Path(tempfile.gettempdir()) / "mi-lsp-harness-first" / digest(str(source_root).casefold())
    return root / GLOBAL_MARKER_NAME


def _claim_path(source_root: Path, candidate: Mapping[str, str]) -> Path:
    key = {
        "campaign_id": str(candidate.get("campaign_id", "")),
        "source_revision": str(candidate.get("source_revision", "")),
        "binary_sha256": str(candidate.get("binary_sha256", "")),
    }
    return _registry_path(source_root).parent / f".harness-first-claim-{digest(key)}.json"


def claim_global_marker(source_root: Path, candidate: Mapping[str, str]) -> None:
    """Claim a candidate tuple with one kernel-level exclusive create.

    The claim filename is a digest of the tuple, so no read-check-write registry
    update is needed and competing processes cannot both claim the same tuple.
    A created claim is intentionally never removed: an interrupted run remains
    claimed and must fail closed on a later attempt.
    """
    path = _claim_path(source_root, candidate)
    path.parent.mkdir(parents=True, exist_ok=True)
    value = {
        "schema": "harness-first-candidate-claim/v1",
        "campaign_id": str(candidate.get("campaign_id", "")),
        "source_revision": str(candidate.get("source_revision", "")),
        "binary_sha256": str(candidate.get("binary_sha256", "")),
    }
    try:
        descriptor = os.open(str(path), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError as exc:
        raise HarnessError("marker_exists") from exc
    except OSError as exc:
        raise HarnessError("marker_exists") from exc
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(value, indent=2, sort_keys=True) + "\n")
    except Exception:
        # Leave the exclusive claim in place; a partial claim is safer than a
        # silently reusable tuple after a process or host failure.
        raise HarnessError("marker_exists")


def _marker(output: Path, campaign_id: str, source: str, manifest: str, binary: str) -> dict[str, Any]:
    return {"schema": MARKER_SCHEMA, "campaign_id": campaign_id, "source_revision": source, "manifest_sha256": manifest, "binary_sha256": binary, "status": "RUNNING"}


def write_marker(path: Path, value: Mapping[str, Any]) -> None:
    try:
        with path.open("x", encoding="utf-8", newline="\n") as stream:
            stream.write(json.dumps(value, indent=2, sort_keys=True) + "\n")
    except FileExistsError as exc:
        raise HarnessError("marker_exists") from exc


def finish_marker(path: Path, status: str) -> None:
    value = json.loads(path.read_text(encoding="utf-8"))
    value["status"] = status
    path.write_text(json.dumps(value, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def percentile(values: Iterable[float], p: float) -> float | None:
    ordered = sorted(float(v) for v in values)
    if not ordered:
        return None
    rank = (len(ordered) - 1) * p / 100.0
    low = int(rank)
    high = min(low + 1, len(ordered) - 1)
    return ordered[low] + (ordered[high] - ordered[low]) * (rank - low)


def stats(values: Sequence[float]) -> dict[str, Any]:
    return {"n": len(values), "p50": percentile(values, 50), "p95": percentile(values, 95), "p99": percentile(values, 99), "max": max(values) if values else None}


def _numeric_event_value(value: Any) -> int | None:
    if isinstance(value, int) and not isinstance(value, bool) and value >= 0:
        return value
    if isinstance(value, str) and re.fullmatch(r"[0-9]+", value):
        return int(value)
    return None


def _event_timestamp(event: Mapping[str, Any]) -> datetime | None:
    for key in ("occurred_at", "timestamp"):
        value = event.get(key)
        if not isinstance(value, str):
            continue
        try:
            parsed = datetime.fromisoformat(value.replace("Z", "+00:00"))
        except ValueError:
            continue
        if parsed.tzinfo is None:
            parsed = parsed.replace(tzinfo=timezone.utc)
        return parsed.astimezone(timezone.utc)
    return None


def _select_latest_telemetry_event(
    payload: Sequence[Any],
    *,
    campaign_id: str,
    client_name: str,
    operation: str,
) -> Mapping[str, Any] | None:
    """Select the newest event after applying the complete telemetry scope.

    Numeric event IDs are authoritative because they are database sequence IDs.
    When no scoped event has a usable ID, occurred_at (then seq) is used. If
    those fields are unavailable or malformed too, preserve the admin export's
    documented newest-first order and select its first scoped event.
    """
    events = [
        item
        for item in payload
        if isinstance(item, Mapping)
        and item.get("operation") == operation
        and item.get("client_name") == client_name
        and item.get("session_id") == campaign_id
    ]
    if not events:
        return None

    indexed = list(enumerate(events))
    identified = [(index, event, _numeric_event_value(event.get("id"))) for index, event in indexed]
    with_ids = [item for item in identified if item[2] is not None]
    if with_ids:
        _, event, _ = max(
            with_ids,
            key=lambda item: (
                item[2],
                _event_timestamp(item[1]) or datetime.min.replace(tzinfo=timezone.utc),
                _numeric_event_value(item[1].get("seq")) or -1,
                -item[0],
            ),
        )
        return event

    with_timestamps = [(index, event, timestamp) for index, event in indexed if (timestamp := _event_timestamp(event)) is not None]
    if with_timestamps:
        _, event, _ = max(
            with_timestamps,
            key=lambda item: (
                item[2],
                _numeric_event_value(item[1].get("seq")) or -1,
                -item[0],
            ),
        )
        return event

    with_seq = [(index, event, _numeric_event_value(event.get("seq"))) for index, event in indexed]
    usable_seq = [item for item in with_seq if item[2] is not None]
    if usable_seq:
        _, event, _ = max(usable_seq, key=lambda item: (item[2], -item[0]))
        return event

    return events[0]


def _telemetry_route_probe(binary: Path, campaign_id: str, operation: str, timeout_seconds: float) -> tuple[str | None, str | None]:
    probe = [str(binary), "--format", "json", "--client-name", "harness-first", "--session-id", campaign_id, "admin", "export", "--since", "1h", "--session-id", campaign_id, "--client-name", "harness-first", "--operation", operation, "--format", "json", "--limit", "50"]
    result = run_process(probe, timeout_seconds)
    if result.reason_code is not None or not isinstance(result.payload, list):
        return None, None
    event = _select_latest_telemetry_event(
        result.payload,
        campaign_id=campaign_id,
        client_name="harness-first",
        operation=operation,
    )
    if event is None:
        return None, None
    route = event.get("route") if isinstance(event.get("route"), str) else None
    backend = event.get("backend") if isinstance(event.get("backend"), str) else None
    return route, backend


def _rss_report(samples: Sequence[Mapping[str, Any]], worker: Mapping[str, Any], budget: float) -> dict[str, Any]:
    observations = [item.get("peak_rss_bytes") for item in samples] + [worker.get("peak_rss_bytes")]
    usable = [float(value) for value in observations if isinstance(value, int) and value >= 0]
    if len(usable) != len(observations):
        return {**stats(usable), "status": "NOT_RUN", "observations": len(observations), "reason_code": "rss_unavailable", "includes_worker_status": True}
    status = "PASS" if usable and max(usable) <= budget else "FAIL"
    return {**stats(usable), "status": status, "observations": len(observations), "includes_worker_status": True}


def run_campaign(manifest: Mapping[str, Any], *, binary: str | Path, source_root: str | Path, output: str | Path, timeout_seconds: float = 60.0) -> dict[str, Any]:
    contract = validate_manifest(manifest)
    binary_path, source_path, output_path = Path(binary).resolve(), Path(source_root).resolve(), Path(output).resolve()
    for path in (binary_path, source_path):
        _reject_env(path)
    if not source_path.is_dir():
        raise HarnessError("missing_source")
    if not binary_path.exists():
        raise HarnessError("missing_binary")
    if not binary_path.is_file():
        raise HarnessError("binary_not_file")
    source = source_revision(source_path)
    manifest_hash = digest(contract)
    prov = provenance(binary_path, source)
    if prov["status"] != "PASS":
        raise HarnessError("provenance_unavailable")
    output_path.mkdir(parents=True, exist_ok=True)
    marker_path = output_path / MARKER_NAME
    if marker_path.exists():
        raise HarnessError("marker_exists")
    if any((output_path / name).exists() for name in ("report.json", "report.yaml")) or any(item.name != MARKER_NAME for item in output_path.iterdir()):
        raise HarnessError("output_reused")
    write_marker(marker_path, _marker(output_path, contract["campaign_id"], source, manifest_hash, prov["binary_sha256"]))
    samples: list[dict[str, Any]] = []
    failures: list[str] = []
    projections: dict[tuple[str, str], Any] = {}
    campaign_started = time.perf_counter()
    try:
        candidate_preflight = _candidate_preflight(
            binary_path,
            source_path,
            contract["campaign_id"],
            contract["worker_status_args"],
            prov["binary_sha256"],
            timeout_seconds,
            daemon_required=any("daemon" in query["modes"] for query in contract["queries"]),
        )
        if candidate_preflight["status"] != "PASS":
            return _write_blocked_report(output_path, marker_path, contract, source, manifest_hash, prov, candidate_preflight, campaign_started)
        worker_report = candidate_preflight["worker"]
        claim_global_marker(source_path, {"campaign_id": contract["campaign_id"], "source_revision": source, "binary_sha256": prov["binary_sha256"]})
        for query in contract["queries"]:
            for mode in query["modes"]:
                result = run_process(_command(binary_path, source_path, contract["campaign_id"], query, mode), min(float(query.get("timeout_seconds", timeout_seconds)), timeout_seconds))
                sample: dict[str, Any] = {"query_id": query["id"], "kind": query["kind"], "mode": mode, "attempts": 1, "elapsed_ms": result.elapsed_ms, "peak_rss_bytes": result.rss_bytes, "output_bytes": result.output_bytes, "estimated_tokens": result.estimated_tokens}
                if result.reason_code:
                    sample.update({"status": "FAIL", "reason_code": result.reason_code})
                    failures.append(result.reason_code)
                else:
                    sample.update(query_check(query, result.payload, output_bytes=result.output_bytes, estimated_tokens=result.estimated_tokens))
                    projection, projection_truncated = _semantic_projection(result.payload)
                    projections[(query["id"], mode)] = projection
                    sample["semantic_projection_digest"] = None if projection_truncated else digest(projection)
                    if projection_truncated:
                        sample["status"] = "FAIL"
                        sample["reason_code"] = "projection_truncated"
                    if query["kind"] in {"related", "wiki_pack", "explain_change", "workspace_map"}:
                        route, backend = _telemetry_route_probe(binary_path, contract["campaign_id"], "nav.related" if query["kind"] == "related" else ("nav.wiki.pack" if query["kind"] == "wiki_pack" else "nav.intent" if query["kind"] == "explain_change" else "nav.workspace-map"), timeout_seconds)
                        if query["kind"] == "related":
                            expected_route = mode
                            sample["routing"] = {"observed_route": route, "observed_backend": backend, "expected_route": expected_route, "status": "PASS" if route == expected_route and bool(backend) and (mode != "daemon" or route != "direct_fallback") else "FAIL"}
                            if sample["routing"]["status"] != "PASS":
                                sample["status"] = "FAIL"
                                sample["reason_code"] = "route_unobserved" if route is None or backend is None else "route_mismatch"
                    if sample["status"] != "PASS":
                        failures.append(str(sample.get("reason_code", "correctness_failed")))
                samples.append(sample)
        latencies = [float(item["elapsed_ms"]) for item in samples if finite(item.get("elapsed_ms"))]
        budgets = contract["budgets"]
        latency = stats(latencies)
        latency_ok = finite(latency["p95"]) and finite(latency["p99"]) and latency["p95"] <= budgets["latency_p95_ms"] and latency["p99"] <= budgets["latency_p99_ms"]
        rss_report = _rss_report(samples, worker_report, budgets["peak_rss_bytes"])
        if not latency_ok:
            failures.append("latency_ceiling")
        if rss_report["status"] != "PASS":
            failures.append(str(rss_report.get("reason_code", "rss_ceiling")))
        grouped: dict[str, dict[str, dict[str, Any]]] = {}
        for sample in samples:
            grouped.setdefault(sample["query_id"], {})[sample["mode"]] = sample
        parity_ids = [query["id"] for query in contract["queries"] if query.get("parity_required")]
        parity_results = {}
        parity_diagnostics: dict[str, dict[str, Any]] = {}
        for query_id in parity_ids:
            pair = grouped.get(query_id, {})
            direct = pair.get("direct", {})
            daemon = pair.get("daemon", {})
            diff = _bounded_structural_diff(projections.get((query_id, "direct")), projections.get((query_id, "daemon")))
            passed = (
                direct.get("status") == "PASS"
                and daemon.get("status") == "PASS"
                and direct.get("normalized_digest") is not None
                and direct.get("normalized_digest") == daemon.get("normalized_digest")
                and not diff.get("truncated")
                and direct.get("semantic_projection", {}).get("truncated") is False
                and daemon.get("semantic_projection", {}).get("truncated") is False
            )
            parity_results[query_id] = passed
            parity_diagnostics[query_id] = {
                "status": "PASS" if passed else "FAIL",
                "direct_digest": direct.get("semantic_projection_digest"),
                "daemon_digest": daemon.get("semantic_projection_digest"),
                "diff": diff,
            }
        parity = all(parity_results.values()) if parity_results else True
        if not parity:
            failures.append("parity_failed")
        expected_calls = sum(len(query["modes"]) for query in contract["queries"])
        retry = {"attempts": len(samples), "queries": expected_calls, "amplification": len(samples) / expected_calls if expected_calls else None, "max_attempts_per_query": max((sample["attempts"] for sample in samples), default=0), "status": "PASS" if len(samples) == expected_calls and all(sample["attempts"] == 1 for sample in samples) else "FAIL"}
        if retry["status"] != "PASS":
            failures.append("retry_amplification")
        correctness_value = sum(sample["status"] == "PASS" for sample in samples) / len(samples) * 100.0 if samples else 0.0
        if correctness_value != budgets["correctness_percent"]:
            failures.append("correctness_failed")
        preview_samples = [sample for sample in samples if sample.get("preview", {}).get("required")]
        preview_status = all(sample.get("preview", {}).get("status") == "PASS" for sample in preview_samples)
        preview_tokens = sum(int(sample.get("estimated_tokens", 0)) for sample in preview_samples)
        preview_bytes = sum(int(sample.get("output_bytes", 0)) for sample in preview_samples)
        preview_ratios = [sample.get("preview_usefulness", {}).get("utility_ratio") for sample in preview_samples if finite(sample.get("preview_usefulness", {}).get("utility_ratio"))]
        if not preview_status:
            failures.append("preview_incomplete")
        kind_reports: dict[str, dict[str, Any]] = {}
        for kind in SEMANTIC_KINDS:
            kind_samples = [sample for sample in samples if sample.get("kind") == kind]
            kind_reports[kind] = {"status": "PASS" if kind_samples and all(sample["status"] == "PASS" for sample in kind_samples) else "FAIL", "queries": [sample["query_id"] for sample in kind_samples]}
        report = {
            "schema": REPORT_SCHEMA,
            "campaign_id": contract["campaign_id"],
            "status": "PASS" if not failures else "FAIL",
            "source_revision": source,
            "manifest_sha256": manifest_hash,
            "provenance": prov,
            "budgets": budgets,
            "correctness": {"percent": correctness_value, "status": "PASS" if correctness_value == 100.0 else "FAIL"},
            "latency_ms": {**latency, "status": "PASS" if latency_ok else "FAIL"},
            "peak_rss_bytes": rss_report,
            "preview_usefulness": {"status": "PASS" if preview_status else "FAIL", "sections": list(SECTIONS), "expansions": ["command", "reason"], "preview_count": len(preview_samples), "output_bytes": preview_bytes, "estimated_tokens": preview_tokens, "utility_ratio": round(sum(preview_ratios) / len(preview_ratios), 4) if preview_ratios else None},
            "wiki_pack": kind_reports["wiki_pack"],
            "explain_change": kind_reports["explain_change"],
            "workspace_map": kind_reports["workspace_map"],
            "related": kind_reports["related"],
            "parity": {"required_queries": parity_ids, "results": parity_results, "status": "PASS" if parity else "FAIL", "diagnostics": parity_diagnostics},
            "retry_amplification": retry,
            "candidate_preflight": candidate_preflight,
            "worker_status": worker_report,
            "samples": samples,
            "duration_ms": (time.perf_counter() - campaign_started) * 1000.0,
            "sanitized": True,
            "comparators": {"graphify": "not_used"},
            "failure_reasons": sorted(set(reason if reason in REASONS else "native_error" for reason in failures)),
        }
        (output_path / "report.json").write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        (output_path / "report.yaml").write_text(to_yaml(report), encoding="utf-8")
        finish_marker(marker_path, report["status"])
        return report
    except Exception:
        finish_marker(marker_path, "BLOCKED")
        raise


def _yaml_scalar(value: Any) -> str:
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, (int, float)):
        return str(value).lower()
    return json.dumps(str(value), ensure_ascii=False)


def to_yaml(value: Any, indent: int = 0) -> str:
    """Emit a deterministic YAML subset and sanitize the top-level object."""
    if indent == 0:
        value = sanitize(value)
    pad = " " * indent
    if isinstance(value, Mapping):
        lines = []
        for key in sorted(value, key=str):
            child = value[key]
            if isinstance(child, (Mapping, list)) and child:
                lines.append(f"{pad}{key}:")
                lines.append(to_yaml(child, indent + 2).rstrip("\n"))
            else:
                lines.append(f"{pad}{key}: {_yaml_scalar(child)}")
        return "\n".join(lines) + "\n"
    if isinstance(value, list):
        lines = []
        for child in value:
            if isinstance(child, Mapping) and child:
                first = True
                for key in sorted(child, key=str):
                    nested = child[key]
                    prefix = f"{pad}- " if first else f"{pad}  "
                    if isinstance(nested, (Mapping, list)) and nested:
                        lines.append(f"{prefix}{key}:")
                        lines.append(to_yaml(nested, indent + 4).rstrip("\n"))
                    else:
                        lines.append(f"{prefix}{key}: {_yaml_scalar(nested)}")
                    first = False
            else:
                lines.append(f"{pad}- {_yaml_scalar(child)}")
        return "\n".join(lines) + "\n"
    return f"{pad}{_yaml_scalar(value)}\n"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--binary", type=Path)
    parser.add_argument("--source-root", type=Path, default=Path("."))
    parser.add_argument("--output", type=Path)
    parser.add_argument("--dry-run", action="store_true")
    parser.add_argument("--run", action="store_true")
    args = parser.parse_args(argv)
    if args.dry_run == args.run:
        parser.error("choose exactly one of --dry-run or --run")
    try:
        manifest = validate_manifest(load_manifest(args.manifest))
        if args.dry_run:
            print(json.dumps({"schema": SCHEMA, "status": "PASS", "campaign_id": manifest["campaign_id"], "queries": len(manifest["queries"]), "invocations": sum(len(query["modes"]) for query in manifest["queries"]) + 1, "route_probes": sum(1 for query in manifest["queries"] for _mode in query["modes"] if query["kind"] == "related"), "one_shot": True, "retry_limit": 1}, sort_keys=True))
            return 0
        if args.binary is None:
            raise HarnessError("missing_binary")
        if args.output is None:
            raise HarnessError("invalid_manifest")
        report = run_campaign(manifest, binary=args.binary, source_root=args.source_root, output=args.output)
        print(json.dumps({"schema": REPORT_SCHEMA, "status": report["status"]}, sort_keys=True))
        return 0 if report["status"] == "PASS" else 2
    except HarnessError as exc:
        print(json.dumps({"schema": REPORT_SCHEMA, "status": "BLOCKED", "reason_code": exc.reason_code}, sort_keys=True), file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
