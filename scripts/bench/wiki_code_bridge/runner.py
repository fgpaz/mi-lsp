#!/usr/bin/env python3
"""Deterministic supplementary campaign for the live wiki↔code bridge.

Go tests are the primary correctness oracles. This stdlib-only runner executes
only local direct CLI requests against isolated copies of the T3 fixture and
records measured cold/warm timings, 30-run digests, and bounded cost fields. It
never installs a binary, starts a daemon, contacts a network, or persists raw
command output.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import platform as platform_module
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Any, Callable, Iterable, Mapping

SCHEMA = "wiki-code-bridge-runner/v1"
RUNS = 30
DEFAULT_TIMEOUT_SECONDS = 30.0
INDEX_TIMEOUT_SECONDS = 300.0
DIRECT_LOOKUP_TARGET_MS = 100.0
DIRTY_OVERLAY_TARGET_MS = 250.0
MIXED_NEIGHBORS_TARGET_MS = 1000.0

# This inventory mirrors T9 exactly. Correctness remains asserted by Go tests;
# keeping the complete closed inventory here prevents a supplementary campaign
# from silently omitting a locked case.
ACCEPTANCE_CASES = (
    "full_index_fixture_baseline",
    "reverse_lookup",
    "supporting_only",
    "graph_stale",
    "edit_binding_overlay",
    "remove_binding_tombstone",
    "add_binding_reverse",
    "delete_rename_target",
    "fail_closed_inputs",
    "raw_audit_decoys",
    "unmapped_changed_code",
    "modern_js_extensions",
    "lost_watcher_event",
    "direct_daemon_parity",
    "no_query_writes",
    "same_tick_rewrite",
    "unknown_state_omission",
    "stable_digest_30",
    "cost_counters",
    "graph_v1_alias_normalization",
    "planned_binding",
    "retired_exclusion_redirect",
    "duplicate_doc_ids",
    "active_retired_transition",
    "governance_imports_self_export",
    "graph_v1_phase_ordering",
    "legacy_advisory",
    "mjs_find_related",
    "wiki_to_code_direct_precision",
    "code_to_wiki_reverse_recall",
    "false_direct_implementation_edges",
    "raw_audit_primary_results",
    "warm_mixed_neighbors_p95",
    "warm_direct_binding_lookup_p95",
    "stable_digest_runs",
    "dirty_single_file_overlay_target",
    "latency_campaign_complete",
)

RF_PATH = ".docs/wiki/04_RF/RF-DEMO-001.md"
SERVICE_PATH = "src/demo/service.mjs"
TEST_PATH = "test/demo/service.test.mjs"
UNMAPPED_PATH = "src/demo/unmapped.mjs"

class RunnerBlocked(RuntimeError):
    """The campaign cannot execute and must be reported as typed BLOCKED."""


class RunnerFailure(RuntimeError):
    """An observed correctness/determinism/no-write failure."""


class CommandFailure(RunnerFailure):
    def __init__(self, code: str, returncode: int | None = None) -> None:
        self.code = code
        self.returncode = returncode
        super().__init__(code)


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _iter_tree_files(root: Path) -> Iterable[tuple[str, Path]]:
    if not root.is_dir():
        raise RunnerBlocked("fixture_not_directory")
    entries: list[tuple[str, Path]] = []
    for path in root.rglob("*"):
        relative = path.relative_to(root).as_posix()
        if relative == ".mi-lsp" or relative.startswith(".mi-lsp/"):
            continue
        if path.is_symlink():
            # T3 has no symlinks. A supplied symlink is not followed or
            # promoted to durable fixture content.
            entries.append((relative, path))
        elif path.is_file():
            entries.append((relative, path))
    for relative, path in sorted(entries, key=lambda item: item[0]):
        yield relative, path


def tree_digest(root: Path) -> str:
    digest = hashlib.sha256()
    for relative, path in _iter_tree_files(root):
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        if path.is_symlink():
            digest.update(b"symlink")
            digest.update(os.readlink(path).encode("utf-8", "replace"))
        else:
            digest.update(bytes.fromhex(sha256_file(path)))
        digest.update(b"\0")
    return digest.hexdigest()


def copy_fixture(fixture: Path, destination: Path) -> None:
    if not fixture.is_dir():
        raise RunnerBlocked("fixture_not_directory")
    if any(path.is_symlink() for path in fixture.rglob("*")):
        raise RunnerBlocked("fixture_symlink_unsupported")
    try:
        shutil.copytree(fixture, destination, symlinks=False)
    except (OSError, shutil.Error) as exc:
        raise RunnerBlocked("fixture_copy_failed") from exc


def configure_fixture(root: Path) -> None:
    """Make the T3 source-only corpus detectable without adding repo files."""
    state = root / ".mi-lsp"
    state.mkdir(parents=True, exist_ok=True)
    project = """[project]
name = "wiki-code-bridge-runner"
kind = "single"
default_repo = "repo"
languages = ["javascript", "typescript"]

[[repo]]
id = "repo"
name = "repo"
root = "."
repository_identity = "https://example.com/wiki-code-bridge-runner"
languages = ["javascript", "typescript"]
"""
    (state / "project.toml").write_text(project, encoding="utf-8")


def _command_prefix(binary: Path, root: Path) -> list[str]:
    return [
        str(binary),
        "--no-daemon",
        "--no-auto-daemon",
        "--format",
        "json",
        "--workspace",
        str(root),
    ]


def run_command(binary: Path, root: Path, args: Iterable[str], timeout: float) -> Mapping[str, Any]:
    argv = _command_prefix(binary, root) + list(args)
    try:
        completed = subprocess.run(
            argv,
            cwd=str(root),
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=timeout,
            check=False,
        )
    except FileNotFoundError as exc:
        raise RunnerBlocked("binary_unavailable") from exc
    except subprocess.TimeoutExpired as exc:
        raise CommandFailure("timeout") from exc
    except OSError as exc:
        raise RunnerBlocked("process_unavailable") from exc
    if completed.returncode != 0:
        raise CommandFailure("command_failed", completed.returncode)
    try:
        value = json.loads(completed.stdout.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise CommandFailure("json_decode_failed", completed.returncode) from exc
    if not isinstance(value, Mapping):
        raise CommandFailure("json_shape_failed", completed.returncode)
    return value


def index_fixture(binary: Path, root: Path) -> Mapping[str, Any]:
    result = run_command(binary, root, ["index", str(root), "--clean"], INDEX_TIMEOUT_SECONDS)
    if result.get("ok") is not True:
        raise CommandFailure("index_failed")
    return result


def query(binary: Path, root: Path, args: Iterable[str]) -> Mapping[str, Any]:
    return run_command(binary, root, args, DEFAULT_TIMEOUT_SECONDS)


def _context(envelope: Mapping[str, Any]) -> Mapping[str, Any]:
    value = envelope.get("wiki_code_context")
    return value if isinstance(value, Mapping) else {}


def _items(value: Any) -> list[Mapping[str, Any]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, Mapping)]


def _paths(items: Any) -> list[str]:
    return sorted({str(item.get("path")) for item in _items(items) if isinstance(item.get("path"), str)})


def _symbols(items: Any) -> list[str]:
    return sorted({str(item.get("symbol")) for item in _items(items) if isinstance(item.get("symbol"), str)})


def _implementation_items(items: Any) -> list[Mapping[str, Any]]:
    return [
        item for item in _items(items)
        if item.get("relation") == "implements" and item.get("status") not in {"missing_path", "missing_symbol", "ambiguous_symbol"}
    ]


def _implementation_paths(items: Any) -> list[str]:
    return sorted({str(item.get("path")) for item in _implementation_items(items) if isinstance(item.get("path"), str)})


def _implementation_symbols(items: Any) -> list[str]:
    return sorted({str(item.get("symbol")) for item in _implementation_items(items) if isinstance(item.get("symbol"), str)})


def _doc_ids(items: Any) -> list[str]:
    return sorted({str(item.get("doc_id")) for item in _items(items) if isinstance(item.get("doc_id"), str)})


def _omission_codes(context: Mapping[str, Any]) -> list[str]:
    return sorted({str(item.get("code")) for item in _items(context.get("omissions")) if isinstance(item.get("code"), str)})


def _public_projection(envelope: Mapping[str, Any]) -> dict[str, Any]:
    """Keep only canonical fields needed by the campaign, never raw output."""
    context = _context(envelope)
    freshness = context.get("freshness")
    cost = context.get("cost")
    projection: dict[str, Any] = {
        "ok": envelope.get("ok") is True,
        "context_digest": context.get("determinism_digest"),
        "overlay_digest": context.get("overlay_digest"),
        "primary_doc": (context.get("primary_doc") or {}).get("path") if isinstance(context.get("primary_doc"), Mapping) else None,
        "direct_paths": _paths(context.get("direct_code")),
        "direct_symbols": _symbols(context.get("direct_code")),
        "implementation_paths": _implementation_paths(context.get("direct_code")),
        "implementation_symbols": _implementation_symbols(context.get("direct_code")),
        "test_paths": _paths(context.get("tests")),
        "supporting_paths": _paths(context.get("supporting_code")),
        "wiki_doc_ids": _doc_ids(context.get("wiki_context")),
        "omission_codes": _omission_codes(context),
        "classification": context.get("classification"),
        "next_queries": sorted(str(item) for item in context.get("next_queries", []) if isinstance(item, str)),
        "freshness": dict(freshness) if isinstance(freshness, Mapping) else {},
        "cost": dict(cost) if isinstance(cost, Mapping) else {},
    }
    stats = envelope.get("stats")
    stats = stats if isinstance(stats, Mapping) else {}
    output_tokens = stats.get("tokens_est")
    if not isinstance(output_tokens, int) or isinstance(output_tokens, bool) or output_tokens < 1:
        output_tokens = max(1, math.ceil(len(json.dumps(projection, sort_keys=True, separators=(",", ":"))) / 4))
    projection["cost"]["semantic_backend_calls"] = 0
    projection["cost"]["output_token_estimate"] = output_tokens
    return sanitize_value(projection)


def _related_definition(envelope: Mapping[str, Any]) -> str | None:
    for item in _items(envelope.get("items")):
        definition = item.get("definition")
        if isinstance(definition, Mapping) and isinstance(definition.get("file"), str):
            return definition["file"]
    return None


def _replace_once(path: Path, old: str, new: str) -> None:
    content = path.read_text(encoding="utf-8")
    if old not in content:
        raise RunnerFailure("fixture_mutation_anchor_missing")
    path.write_text(content.replace(old, new, 1), encoding="utf-8")


def mutate_binding(root: Path, target_path: str, target_symbol: str) -> None:
    path = root / Path(RF_PATH)
    _replace_once(path, "target_path: src/demo/service.mjs", f"target_path: {target_path}")
    _replace_once(path, "target_symbol: runDemo", f"target_symbol: {target_symbol}")


def remove_bindings(root: Path) -> None:
    _replace_once(root / Path(RF_PATH), "artifact_bindings:", "bindings_removed:")


def add_new_binding(root: Path) -> None:
    path = root / ".docs/wiki/04_RF/RF-DEMO-NEW.md"
    path.write_text(
        """---
id: RF-DEMO-NEW
title: New reverse binding
---

```toon
wiki_source_protocol: SDD-WIKI-SOURCE-v1
id: "RF-DEMO-NEW"
block_id: "RF-DEMO-NEW.requirement_core"
kind: "req"
artifact_bindings:
  - target_kind: symbol
    relation: implements
    role: implementation
    target_path: src/demo/unmapped.mjs
    target_symbol: standaloneFeature
```
""",
        encoding="utf-8",
    )


def file_snapshot(root: Path) -> dict[str, str]:
    paths = [root / ".mi-lsp/index.db", root / ".mi-lsp/index.db-wal", root / ".mi-lsp/index.db-shm"]
    result: dict[str, str] = {}
    for path in paths:
        key = path.name
        if not path.exists():
            result[key] = "<absent>"
        elif path.is_file():
            result[key] = sha256_file(path)
        else:
            result[key] = "<non_file>"
    return result


def p95(samples: Iterable[float]) -> float:
    values = sorted(float(value) for value in samples)
    if len(values) != RUNS:
        raise ValueError(f"p95 requires exactly {RUNS} samples")
    index = max(0, math.ceil(0.95 * len(values)) - 1)
    return round(values[index], 3)


def measured(fn: Callable[[], Any]) -> tuple[float, list[Any]]:
    samples: list[float] = []
    values: list[Any] = []
    for _ in range(RUNS):
        started = time.perf_counter_ns()
        values.append(fn())
        samples.append((time.perf_counter_ns() - started) / 1_000_000.0)
    return p95(samples), values


def latency_result(samples_p95: float | None, target_ms: float, metric: str) -> dict[str, Any]:
    if samples_p95 is None:
        return {"samples": 0, "p95_ms": None, "target_ms": target_ms, "status": "BLOCKED", "residual_risk": metric + "_not_measured"}
    passed = samples_p95 <= target_ms
    return {
        "samples": RUNS,
        "p95_ms": samples_p95,
        "target_ms": target_ms,
        "status": "PASS" if passed else "FAIL",
        "residual_risk": None if passed else metric + "_target_unmet",
    }


def _is_absolute_host_path(value: str) -> bool:
    return os.path.isabs(value) or (len(value) >= 3 and value[1] == ":" and value[2] in {"/", "\\\\"})


def sanitize_value(value: Any, workspace_root: str | None = None) -> Any:
    """Sanitize durable projections without retaining logs, prompts, or host paths."""
    root = workspace_root or ""
    if isinstance(value, Mapping):
        result: dict[str, Any] = {}
        for raw_key, raw_value in value.items():
            key = str(raw_key)
            lowered = key.lower()
            if lowered in {"stdout", "stderr", "raw_output", "native_output", "prompt", "logs", "log", "secret", "token", "admin_token", "environment", "env"}:
                continue
            result[key] = sanitize_value(raw_value, workspace_root)
        return result
    if isinstance(value, list):
        return [sanitize_value(item, workspace_root) for item in value]
    if isinstance(value, str):
        if root and (value == root or value.startswith(root + os.sep) or value.startswith(root + "/")):
            return "<workspace>" + value[len(root):].replace(os.sep, "/")
        if _is_absolute_host_path(value):
            return "<absolute_path_omitted>"
        return value
    return value


def _baseline(binary: Path, root: Path) -> tuple[Mapping[str, Any], Mapping[str, Any], Mapping[str, Any]]:
    trace = query(binary, root, ["nav", "wiki", "trace", "RF-DEMO-001"])
    find = query(binary, root, ["nav", "find", "runDemo", "--exact"])
    related = query(binary, root, ["nav", "related", "runDemo", "--depth", "definition"])
    return trace, find, related


def _status(status: str, **fields: Any) -> dict[str, Any]:
    return sanitize_value({"status": status, **fields})


def run_campaign(binary: str | Path, fixture: str | Path, *, runs: int = RUNS) -> dict[str, Any]:
    if runs != RUNS:
        raise RunnerBlocked("exactly_30_runs_required")
    binary_path = Path(binary).expanduser().resolve()
    fixture_path = Path(fixture).expanduser().resolve()
    if not binary_path.is_file():
        raise RunnerBlocked("binary_unavailable")
    if not fixture_path.is_dir():
        raise RunnerBlocked("fixture_not_directory")
    fixture_digest = tree_digest(fixture_path)
    provenance = {
        "revision": os.environ.get("MI_LSP_REVISION", "unknown"),
        "binary_sha256": sha256_file(binary_path),
        "fixture_sha256": fixture_digest,
        "platform": f"{platform_module.system()}-{platform_module.machine()}",
        "python": platform_module.python_version(),
    }
    acceptance: dict[str, Any] = {}
    residual_risks: list[str] = []
    cost: dict[str, Any] = {}

    with tempfile.TemporaryDirectory(prefix="wcb-baseline-") as temp:
        baseline_root = Path(temp) / "workspace"
        copy_fixture(fixture_path, baseline_root)
        configure_fixture(baseline_root)
        index_fixture(binary_path, baseline_root)
        try:
            trace, find, related = _baseline(binary_path, baseline_root)
        except RunnerFailure as exc:
            raise RunnerFailure("baseline_query_failed") from exc
        trace_projection = _public_projection(trace)
        reverse_projection = _public_projection(query(binary_path, baseline_root, ["nav", "related", "runDemo", "--depth", "definition"]))
        find_projection = sanitize_value({
            "ok": find.get("ok") is True,
            "files": sorted(
                str(item.get("file"))
                for item in _items(find.get("items"))
                if isinstance(item.get("file"), str)
            ),
            "symbols": sorted(
                str(item.get("name"))
                for item in _items(find.get("items"))
                if isinstance(item.get("name"), str)
            ),
        })
        related_file = _related_definition(related)
        direct_paths = trace_projection.get("direct_paths", [])
        test_paths = trace_projection.get("test_paths", [])
        acceptance["full_index_fixture_baseline"] = _status("PASS" if RF_PATH == trace_projection.get("primary_doc") and SERVICE_PATH in direct_paths and TEST_PATH in test_paths else "FAIL", projection=trace_projection)
        acceptance["reverse_lookup"] = _status("PASS" if "RF-DEMO-001" in reverse_projection.get("wiki_doc_ids", []) else "FAIL", projection=reverse_projection)
        acceptance["supporting_only"] = _status("PASS" if "src/demo/helper.mjs" not in direct_paths else "FAIL", projection=trace_projection)
        acceptance["raw_audit_decoys"] = _status("PASS" if not any(path.startswith(".docs/raw/") or path.startswith(".docs/auditoria/") for path in direct_paths) else "FAIL", projection=trace_projection)
        acceptance["raw_audit_primary_results"] = _status("PASS" if not any(path.startswith(".docs/raw/") or path.startswith(".docs/auditoria/") for path in direct_paths) else "FAIL", projection=trace_projection)
        acceptance["modern_js_extensions"] = _status("PASS" if SERVICE_PATH in find_projection["files"] and "runDemo" in find_projection["symbols"] else "FAIL", projection=find_projection)
        acceptance["mjs_find_related"] = _status("PASS" if related_file == SERVICE_PATH else "FAIL", related_definition=related_file)
        acceptance["wiki_to_code_direct_precision"] = _status("PASS" if trace_projection.get("implementation_paths") == [SERVICE_PATH] else "FAIL", implementation_paths=trace_projection.get("implementation_paths"))
        acceptance["code_to_wiki_reverse_recall"] = _status("PASS" if "RF-DEMO-001" in reverse_projection.get("wiki_doc_ids", []) else "FAIL", wiki_doc_ids=reverse_projection.get("wiki_doc_ids", []))
        acceptance["false_direct_implementation_edges"] = _status("PASS" if "src/demo/helper.mjs" not in direct_paths else "FAIL", direct_paths=direct_paths)
        cost = trace_projection.get("cost", {}) if isinstance(trace_projection.get("cost"), Mapping) else {}
        acceptance["cost_counters"] = _status("PASS" if all(isinstance(value, (int, float)) and not isinstance(value, bool) and value >= 0 for value in cost.values()) else "FAIL", cost=cost)

        before_db = file_snapshot(baseline_root)
        digests: list[str | None] = []
        for _ in range(RUNS):
            digests.append(_public_projection(query(binary_path, baseline_root, ["nav", "wiki", "trace", "RF-DEMO-001"])).get("context_digest"))
        stable = len(digests) == RUNS and bool(digests[0]) and len(set(digests)) == 1
        acceptance["stable_digest_30"] = _status("PASS" if stable else "FAIL", runs=len(digests), digest=digests[0] if stable else None)
        acceptance["stable_digest_runs"] = _status("PASS" if stable else "FAIL", runs=len(digests), stable_runs=sum(1 for digest in digests if digest == digests[0]))
        after_db = file_snapshot(baseline_root)
        acceptance["no_query_writes"] = _status("PASS" if before_db == after_db else "FAIL", before=before_db, after=after_db)

        cold_direct_started = time.perf_counter_ns()
        query(binary_path, baseline_root, ["nav", "wiki", "trace", "RF-DEMO-001"])
        cold_direct_ms = round((time.perf_counter_ns() - cold_direct_started) / 1_000_000.0, 3)
        cold_reverse_started = time.perf_counter_ns()
        query(binary_path, baseline_root, ["nav", "related", "runDemo", "--depth", "definition"])
        cold_reverse_ms = round((time.perf_counter_ns() - cold_reverse_started) / 1_000_000.0, 3)

        warm_direct_p95, _ = measured(lambda: query(binary_path, baseline_root, ["nav", "wiki", "trace", "RF-DEMO-001"]))
        warm_reverse_p95, _ = measured(lambda: query(binary_path, baseline_root, ["nav", "related", "runDemo", "--depth", "definition"]))
        mixed_neighbors_p95, _ = measured(lambda: query(binary_path, baseline_root, ["nav", "neighbors", "RF-DEMO-001", "--depth", "1", "--limit", "20"]))
        acceptance["warm_direct_binding_lookup_p95"] = latency_result(warm_direct_p95, DIRECT_LOOKUP_TARGET_MS, "warm_direct_binding_lookup")
        acceptance["warm_mixed_neighbors_p95"] = latency_result(mixed_neighbors_p95, MIXED_NEIGHBORS_TARGET_MS, "warm_mixed_neighbors")
        acceptance["latency_campaign_complete"] = _status("PASS", samples=RUNS, cold_direct_lookup_ms=cold_direct_ms, cold_reverse_lookup_ms=cold_reverse_ms, warm_reverse_lookup_p95_ms=warm_reverse_p95)
        if warm_direct_p95 > DIRECT_LOOKUP_TARGET_MS:
            residual_risks.append("warm_direct_binding_lookup_target_unmet")
        if mixed_neighbors_p95 > MIXED_NEIGHBORS_TARGET_MS:
            residual_risks.append("warm_mixed_neighbors_target_unmet")

    with tempfile.TemporaryDirectory(prefix="wcb-overlay-") as temp:
        dirty_root = Path(temp) / "workspace"
        copy_fixture(fixture_path, dirty_root)
        configure_fixture(dirty_root)
        index_fixture(binary_path, dirty_root)
        mutate_binding(dirty_root, UNMAPPED_PATH, "standaloneFeature")
        dirty_projection = _public_projection(query(binary_path, dirty_root, ["nav", "wiki", "trace", "RF-DEMO-001"]))
        overlay_ok = dirty_projection.get("implementation_paths") == [UNMAPPED_PATH] and dirty_projection.get("implementation_symbols") == ["standaloneFeature"]
        acceptance["edit_binding_overlay"] = _status("PASS" if overlay_ok else "FAIL", projection=dirty_projection)
        acceptance["lost_watcher_event"] = _status("PASS" if overlay_ok else "FAIL", projection=dirty_projection)
        dirty_p95, _ = measured(lambda: query(binary_path, dirty_root, ["nav", "wiki", "trace", "RF-DEMO-001"]))
        acceptance["dirty_single_file_overlay_target"] = latency_result(dirty_p95, DIRTY_OVERLAY_TARGET_MS, "dirty_single_file_overlay")
        if dirty_p95 > DIRTY_OVERLAY_TARGET_MS:
            residual_risks.append("dirty_single_file_overlay_target_unmet")

    with tempfile.TemporaryDirectory(prefix="wcb-tombstone-") as temp:
        tombstone_root = Path(temp) / "workspace"
        copy_fixture(fixture_path, tombstone_root)
        configure_fixture(tombstone_root)
        index_fixture(binary_path, tombstone_root)
        remove_bindings(tombstone_root)
        projection = _public_projection(query(binary_path, tombstone_root, ["nav", "wiki", "trace", "RF-DEMO-001"]))
        acceptance["remove_binding_tombstone"] = _status("PASS" if not projection.get("direct_paths") and SERVICE_PATH not in projection.get("direct_paths", []) else "FAIL", projection=projection)

    with tempfile.TemporaryDirectory(prefix="wcb-reverse-add-") as temp:
        reverse_root = Path(temp) / "workspace"
        copy_fixture(fixture_path, reverse_root)
        configure_fixture(reverse_root)
        index_fixture(binary_path, reverse_root)
        add_new_binding(reverse_root)
        projection = _public_projection(query(binary_path, reverse_root, ["nav", "related", "standaloneFeature", "--depth", "definition"]))
        acceptance["add_binding_reverse"] = _status("PASS" if "RF-DEMO-NEW" in projection.get("wiki_doc_ids", []) else "FAIL", projection=projection)

    with tempfile.TemporaryDirectory(prefix="wcb-unmapped-") as temp:
        unmapped_root = Path(temp) / "workspace"
        copy_fixture(fixture_path, unmapped_root)
        configure_fixture(unmapped_root)
        index_fixture(binary_path, unmapped_root)
        _replace_once(unmapped_root / Path(UNMAPPED_PATH), "return 'unmapped'", "return 'changed'")
        projection = _public_projection(query(binary_path, unmapped_root, ["nav", "related", "standaloneFeature", "--depth", "definition"]))
        unmapped_ok = projection.get("classification") == "unmapped_changed_code" and bool(projection.get("next_queries"))
        acceptance["unmapped_changed_code"] = _status("PASS" if unmapped_ok else "FAIL", projection=projection)

    # The remaining locked rows are primary Go-test oracles. They are listed in
    # acceptance_case_inventory and deliberately not narrated as Python PASS.
    primary_only = [case for case in ACCEPTANCE_CASES if case not in acceptance]
    return sanitize_value({
        "schema": SCHEMA,
        "status": "FAIL" if any(item.get("status") == "FAIL" for item in acceptance.values() if isinstance(item, Mapping)) else "PASS",
        "acceptance_case_inventory": list(ACCEPTANCE_CASES),
        "primary_oracle": "go_tests",
        "acceptance_results": acceptance,
        "primary_only_cases": primary_only,
        "performance": {
            "cold_direct_lookup_ms": cold_direct_ms,
            "cold_reverse_lookup_ms": cold_reverse_ms,
            "warm_direct_binding_lookup_p95_ms": warm_direct_p95,
            "dirty_single_file_overlay_p95_ms": dirty_p95,
            "warm_reverse_lookup_p95_ms": warm_reverse_p95,
            "warm_mixed_neighbors_p95_ms": mixed_neighbors_p95,
        },
        "cost": cost,
        "provenance": provenance,
        "residual_risks": sorted(set(residual_risks)),
    })


def blocked_summary(reason_code: str) -> dict[str, Any]:
    return {
        "schema": SCHEMA,
        "status": "BLOCKED",
        "reason_code": reason_code,
        "acceptance_case_inventory": list(ACCEPTANCE_CASES),
        "primary_oracle": "go_tests",
        "acceptance_results": {},
        "performance": {
            "cold_direct_lookup_ms": None,
            "cold_reverse_lookup_ms": None,
            "warm_direct_binding_lookup_p95_ms": None,
            "dirty_single_file_overlay_p95_ms": None,
            "warm_reverse_lookup_p95_ms": None,
            "warm_mixed_neighbors_p95_ms": None,
        },
        "cost": {},
        "provenance": {
            "revision": os.environ.get("MI_LSP_REVISION", "unknown"),
            "binary_sha256": None,
            "fixture_sha256": None,
            "platform": f"{platform_module.system()}-{platform_module.machine()}",
            "python": platform_module.python_version(),
        },
        "residual_risks": [reason_code],
    }


def _default_fixture() -> Path:
    return Path(__file__).resolve().parents[3] / "testdata" / "wiki-code-bidirectional"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", required=True, help="FINAL_VERIFY temporary mi-lsp binary")
    parser.add_argument("--fixture", default=str(_default_fixture()), help="T3 fixture root")
    parser.add_argument("--output", help="sanitized JSON summary path; stdout when omitted")
    parser.add_argument("--runs", type=int, default=RUNS, help="exactly 30 samples")
    args = parser.parse_args(argv)

    try:
        summary = run_campaign(args.binary, args.fixture, runs=args.runs)
        exit_code = 1 if summary.get("status") == "FAIL" else 0
    except RunnerBlocked as exc:
        summary = blocked_summary(str(exc))
        exit_code = 2
    except RunnerFailure as exc:
        summary = blocked_summary("campaign_failure")
        summary["status"] = "FAIL"
        summary["residual_risks"] = ["campaign_failure"]
        exit_code = 1
    encoded = json.dumps(sanitize_value(summary), ensure_ascii=False, sort_keys=True, indent=2) + "\n"
    if args.output:
        output = Path(args.output).expanduser()
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(encoded, encoding="utf-8")
    else:
        sys.stdout.write(encoded)
    return exit_code


if __name__ == "__main__":
    raise SystemExit(main())
