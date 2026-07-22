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
MARKER_NAME = ".harness-first-run-marker.json"
GLOBAL_MARKER_NAME = ".harness-first-candidate-registry.json"
MODES = ("direct", "daemon")
SECTIONS = ("change", "affected", "callers", "callees", "tests", "contracts", "wiki")
SHA256 = re.compile(r"^[0-9a-f]{64}$")
SOURCE_REVISION = re.compile(r"^(?:[0-9a-f]{40}|[0-9a-f]{64})$")
SECRET = re.compile(r"(?i)(?:token|secret|password|credential|api[_-]?key|authorization|bearer)")
EMAIL = re.compile(r"\b[^\s@]+@[^\s@]+\.[^\s@]+\b")
PATH = re.compile(r"(?i)(?:^[a-z]:[\\/]|^/|(?:^|[\\/])\.\.?[\\/]|[\\/]\.git[\\/])")
VOLATILE = frozenset({
    "backend", "route", "routing_outcome", "daemon", "daemon_state", "request_id",
    "session_id", "occurred_at", "timestamp", "latency_ms", "format_ms", "tokens_est",
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
    "worker_failed", "worker_schema", "retry_amplification", "provenance_unavailable",
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
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(float(value))


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
    worker_args = strings(manifest.get("worker_status_args", ["worker", "status"]), "worker_status_args")
    return {"schema": SCHEMA, "campaign_id": campaign_id, "queries": queries, "budgets": budgets, "worker_status_args": worker_args}


def sanitize(value: Any, key: str = "") -> Any:
    lowered = key.lower()
    if SECRET.search(lowered) or lowered in {"stdout", "stderr", "raw_output", "native_output", "env", "environment", "payload"}:
        return None
    if isinstance(value, Mapping):
        result: dict[str, Any] = {}
        for raw_key, child in value.items():
            name = str(raw_key)
            if name.lower() in VOLATILE or SECRET.search(name.lower()):
                continue
            projected = sanitize(child, name)
            if projected is not None:
                result[name] = projected
        return result
    if isinstance(value, list):
        result = []
        for child in value:
            projected = sanitize(child, key)
            if projected is not None:
                result.append(projected)
        return result
    if isinstance(value, str):
        return None if EMAIL.search(value) or PATH.search(value) or SECRET.search(value) else value[:512]
    return value if value is None or isinstance(value, bool) or finite(value) else None


def normalize(value: Any) -> Any:
    if isinstance(value, Mapping):
        return {str(k): normalize(v) for k, v in sorted(value.items(), key=lambda item: str(item[0])) if str(k).lower() not in VOLATILE}
    if isinstance(value, list):
        return [normalize(item) for item in value]
    return value


def _find_preview_contract(value: Any, depth: int = 0) -> tuple[list[Mapping[str, Any]], Mapping[str, Any]] | None:
    if depth > 8:
        return None
    if isinstance(value, Mapping):
        preview = value.get("preview")
        if isinstance(preview, list):
            return [item for item in preview if isinstance(item, Mapping)], value
        for child in value.values():
            found = _find_preview_contract(child, depth + 1)
            if found is not None:
                return found
    elif isinstance(value, list):
        for child in value[:64]:
            found = _find_preview_contract(child, depth + 1)
            if found is not None:
                return found
    return None


def _find_sections(value: Any, depth: int = 0) -> Mapping[str, Any] | None:
    """Compatibility reader for the old map-shaped test fixture."""
    if depth > 8:
        return None
    if isinstance(value, Mapping):
        if all(section in value for section in SECTIONS):
            return value
        for child in value.values():
            found = _find_sections(child, depth + 1)
            if found is not None:
                return found
    elif isinstance(value, list):
        for child in value[:64]:
            found = _find_sections(child, depth + 1)
            if found is not None:
                return found
    return None


def _find_expansions(value: Any, depth: int = 0) -> list[Mapping[str, Any]] | None:
    if depth > 8:
        return None
    if isinstance(value, Mapping):
        if isinstance(value.get("expansions"), list):
            return [item for item in value["expansions"] if isinstance(item, Mapping)]
        for child in value.values():
            found = _find_expansions(child, depth + 1)
            if found is not None:
                return found
    elif isinstance(value, list):
        for child in value[:64]:
            found = _find_expansions(child, depth + 1)
            if found is not None:
                return found
    return None


def _section_explanation(plan: Mapping[str, Any], section: str) -> str | None:
    for key in ("omissions", "fallbacks"):
        values = plan.get(key)
        if not isinstance(values, list):
            continue
        for item in values:
            if isinstance(item, Mapping) and item.get("section") == section and isinstance(item.get("reason"), str) and item["reason"].strip():
                return item["reason"].strip()[:512]
    return None


def _preview_check(payload: Any) -> dict[str, Any]:
    found = _find_preview_contract(payload)
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
                explanation = _section_explanation(plan, section)
                if explanation:
                    unavailable[section] = explanation
                else:
                    unexplained.append(section)
                    unavailable[section] = "section has no usable items and no explicit omission/fallback"
        expansions = _find_expansions(plan) or []
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
        result = {
            "status": "PASS" if complete else "FAIL",
            "sections": [section for section in SECTIONS if section in by_section],
            "missing_sections": missing,
            "available_sections": available,
            "omissions": unavailable,
            "duplicates": duplicates,
            "expansions": {"required": ["command", "reason"], "complete": complete and bool(valid_expansions), "missing": missing_expansion_fields, "invalid": invalid_expansions, "valid": len(valid_expansions)},
        }
        return result

    section_map = _find_sections(payload)
    expansions = _find_expansions(payload) or []
    missing = [section for section in SECTIONS if section_map is None or section not in section_map]
    unavailable = {} if section_map is None else {section: "section shape is legacy map; no explicit availability evidence" for section in SECTIONS if not section_map.get(section)}
    valid_expansions = [item for item in expansions if isinstance(item.get("command"), str) and item["command"].strip().startswith("mi-lsp nav ") and isinstance(item.get("reason"), str) and item["reason"].strip()]
    invalid_expansions = [item for item in expansions if item not in valid_expansions]
    missing_expansion_fields = [field for field in ("command", "reason") if not any(isinstance(item.get(field), str) and item[field].strip() for item in expansions)]
    complete = not missing and not unavailable and bool(expansions) and not invalid_expansions
    return {"status": "PASS" if complete else "FAIL", "sections": list(SECTIONS) if section_map is not None and not missing else [section for section in SECTIONS if section_map and section in section_map], "missing_sections": missing, "available_sections": [], "omissions": unavailable, "duplicates": [], "expansions": {"required": ["command", "reason"], "complete": complete and bool(valid_expansions), "missing": missing_expansion_fields, "invalid": invalid_expansions, "valid": len(valid_expansions)}}


def _freshness_records(value: Any, depth: int = 0) -> tuple[list[float], list[Mapping[str, Any]], list[list[Any]]]:
    if depth > 8:
        return [], [{"state": FRESHNESS_TRUNCATED_STATE}], []
    ranks: list[float] = []
    graph: list[Mapping[str, Any]] = []
    rank_lists: list[list[Any]] = []
    if isinstance(value, Mapping):
        if finite(value.get("freshness_rank")):
            ranks.append(float(value["freshness_rank"]))
        freshness = value.get("graph_freshness")
        if isinstance(freshness, Mapping):
            graph.append(freshness)
        if isinstance(value.get("graph_ranks"), list):
            rank_lists.append(value["graph_ranks"])
        for child in value.values():
            child_ranks, child_graph, child_lists = _freshness_records(child, depth + 1)
            ranks.extend(child_ranks)
            graph.extend(child_graph)
            rank_lists.extend(child_lists)
    elif isinstance(value, list):
        for child in value[:64]:
            child_ranks, child_graph, child_lists = _freshness_records(child, depth + 1)
            ranks.extend(child_ranks)
            graph.extend(child_graph)
            rank_lists.extend(child_lists)
        if len(value) > 64:
            graph.append({"state": FRESHNESS_TRUNCATED_STATE})
    return ranks, graph, rank_lists


def _kind_check(query: Mapping[str, Any], payload: Any) -> tuple[bool, str | None]:
    if not isinstance(payload, Mapping) or not isinstance(payload.get("items"), list) or not payload["items"]:
        return False, "items must be a non-empty list"
    kind = query.get("kind")
    items = payload["items"]
    if kind == "wiki_pack":
        for item in items:
            if not isinstance(item, Mapping):
                continue
            docs = item.get("docs")
            if isinstance(docs, list) and any(isinstance(doc, Mapping) and isinstance(doc.get("path"), str) and doc["path"].strip() for doc in docs):
                return True, None
            for key in ("evidence", "evidence_paths", "evidence_paths_used"):
                evidence = item.get(key)
                if isinstance(evidence, list) and any(isinstance(path, str) and path.strip() for path in evidence):
                    return True, None
        return False, "wiki pack has no usable docs/evidence"
    if kind == "explain_change":
        return True, None
    if kind in {"workspace_map", "related"}:
        for item in items:
            if not isinstance(item, Mapping):
                continue
            freshness = item.get("graph_freshness")
            if not isinstance(freshness, Mapping) or freshness.get("state") not in FRESHNESS_STATES:
                return False, "graph_freshness.state is missing or invalid"
            if "graph_ranks" not in item or not isinstance(item.get("graph_ranks"), list):
                return False, "graph_ranks list is missing"
        return True, None
    return True, None


def _freshness_check(query: Mapping[str, Any], payload: Any) -> dict[str, Any]:
    required = bool(query.get("freshness_rank_required"))
    ranks, graph, rank_lists = _freshness_records(payload) if required else ([], [], [])
    kind = query.get("kind")
    if not required:
        return {"required": False, "status": "NOT_REQUIRED", "observed": 0, "min": None, "max": None, "graph_states": [], "rank_lists": 0}
    graph_states = [item.get("state") for item in graph]
    current_only = bool(graph_states) and all(state == "current" for state in graph_states)
    if kind in {"workspace_map", "related"}:
        passed = current_only and bool(rank_lists)
    else:
        passed = current_only and (bool(ranks) or bool(rank_lists))
    observed = len(ranks) + len(graph)
    return {"required": True, "status": "PASS" if passed else "FAIL", "observed": observed, "min": min(ranks) if ranks else None, "max": max(ranks) if ranks else None, "graph_states": graph_states, "rank_lists": len(rank_lists)}


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
    projected = sanitize(payload)
    normalized = normalize(projected)
    correct = isinstance(payload, Mapping) and payload.get("ok") is True
    expected = query.get("expected_digest")
    if expected:
        correct = correct and digest(normalized) == expected
    preview = {"required": bool(query.get("preview_required")), "status": "NOT_REQUIRED", "sections": [], "missing_sections": [], "expansions": {"required": ["command", "reason"], "complete": True}}
    if query.get("preview_required"):
        preview = {"required": True, **_preview_check(payload)}
        correct = correct and preview["status"] == "PASS"
    freshness = _freshness_check(query, payload)
    if freshness["required"]:
        correct = correct and freshness["status"] == "PASS"
    kind_ok, kind_reason = _kind_check(query, payload)
    if query.get("kind") in SEMANTIC_KINDS:
        correct = correct and kind_ok
    measurements = {"status": "PASS" if _sample_measurements_ok(output_bytes, estimated_tokens) else "FAIL", "output_bytes": output_bytes, "estimated_tokens": estimated_tokens}
    correct = correct and measurements["status"] == "PASS"
    result = {"status": "PASS" if correct else "FAIL", "normalized_digest": digest(normalized), "preview": preview, "freshness_rank": freshness, "preview_usefulness": _preview_usefulness(preview, output_bytes, estimated_tokens), "sample_measurements": measurements}
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
    claim_global_marker(source_path, {"campaign_id": contract["campaign_id"], "source_revision": source, "binary_sha256": prov["binary_sha256"]})
    write_marker(marker_path, _marker(output_path, contract["campaign_id"], source, manifest_hash, prov["binary_sha256"]))
    samples: list[dict[str, Any]] = []
    failures: list[str] = []
    started = time.perf_counter()
    try:
        for query in contract["queries"]:
            for mode in query["modes"]:
                result = run_process(_command(binary_path, source_path, contract["campaign_id"], query, mode), min(float(query.get("timeout_seconds", timeout_seconds)), timeout_seconds))
                sample: dict[str, Any] = {"query_id": query["id"], "kind": query["kind"], "mode": mode, "attempts": 1, "elapsed_ms": result.elapsed_ms, "peak_rss_bytes": result.rss_bytes, "output_bytes": result.output_bytes, "estimated_tokens": result.estimated_tokens}
                if result.reason_code:
                    sample.update({"status": "FAIL", "reason_code": result.reason_code})
                    failures.append(result.reason_code)
                else:
                    sample.update(query_check(query, result.payload, output_bytes=result.output_bytes, estimated_tokens=result.estimated_tokens))
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
        worker = run_process([str(binary_path), "--workspace", str(source_path), "--format", "json", "--client-name", "harness-first", "--session-id", contract["campaign_id"], *contract["worker_status_args"]], timeout_seconds)
        worker_report: dict[str, Any] = {"status": "FAIL", "attempts": 1, "elapsed_ms": worker.elapsed_ms, "peak_rss_bytes": worker.rss_bytes, "output_bytes": worker.output_bytes, "estimated_tokens": worker.estimated_tokens}
        if worker.reason_code:
            worker_report["reason_code"] = worker.reason_code
            failures.append("worker_failed")
        else:
            worker_report.update(worker_status_check(worker.payload))
            worker_report["payload_digest"] = digest(normalize(sanitize(worker.payload)))
            if worker_report["status"] != "PASS":
                failures.append(str(worker_report.get("reason_code", "worker_failed")))
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
        for query_id in parity_ids:
            pair = grouped.get(query_id, {})
            passed = pair.get("direct", {}).get("status") == "PASS" and pair.get("daemon", {}).get("status") == "PASS" and pair.get("direct", {}).get("normalized_digest") == pair.get("daemon", {}).get("normalized_digest")
            parity_results[query_id] = passed
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
            "parity": {"required_queries": parity_ids, "results": parity_results, "status": "PASS" if parity else "FAIL"},
            "retry_amplification": retry,
            "worker_status": worker_report,
            "samples": samples,
            "duration_ms": (time.perf_counter() - started) * 1000.0,
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
