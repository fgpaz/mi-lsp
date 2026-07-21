"""Aggregate and verify complete Victory Lab v2 evidence."""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import statistics
from pathlib import Path
import re
from typing import Any, Iterable, Mapping

try:
    from .canonical_v2 import payload_digest, token_count, validate_terminal_state
    from .manifest_v2 import load_manifest, sha256_bytes, sha256_file
    from .schema_v2 import RunRecord
    from .durable_v2 import validate_group_key, validate_identifier
    from .validate_manifest import validate_strict_manifest
    from .security_gate import runtime_evidence_digest
except ImportError:  # pragma: no cover
    from canonical_v2 import payload_digest, token_count, validate_terminal_state
    from manifest_v2 import load_manifest, sha256_bytes, sha256_file
    from schema_v2 import RunRecord
    from durable_v2 import validate_group_key, validate_identifier
    from validate_manifest import validate_strict_manifest
    from security_gate import runtime_evidence_digest

AUTHORITATIVE_REPETITIONS = 30
_FORBIDDEN_KEYS = frozenset({"best", "best_of", "minimum", "fastest", "selected_sample", "summary"})
_SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
_DURABLE_DIGEST_RE = re.compile(r"^[a-z][0-9a-f]{63}$")
_FRESHNESS_SCHEMA = "victory-sample-freshness/v1"
_FRESHNESS_KEYS = frozenset({"schema", "run_id", "preflight_digest", "group_id", "repetition", "nonce"})


def _valid_pid_list(value: Any, *, nonempty: bool) -> bool:
    return (
        isinstance(value, list)
        and (bool(value) or not nonempty)
        and all(isinstance(pid, int) and not isinstance(pid, bool) and pid > 0 for pid in value)
        and len(set(value)) == len(value)
    )


def _manifest_identity(path: Path) -> str:
    canonical_path = path.resolve().as_posix().casefold()
    return "m" + sha256_bytes(canonical_path.encode("utf-8"))[:63]


def _pid_coverage(runtime: Mapping[str, Any]) -> bool:
    observed = runtime.get("observed_pids")
    metadata = runtime.get("metadata_observed_pids")
    return _valid_pid_list(observed, nonempty=True) and _valid_pid_list(metadata, nonempty=False) and set(observed) <= set(metadata)


def _runtime_digest_matches(runtime: Mapping[str, Any]) -> bool:
    try:
        return runtime_evidence_digest(runtime) == runtime.get("evidence_digest")
    except (TypeError, ValueError):
        return False


def _is_finite_number(value: Any) -> bool:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        return False
    try:
        return math.isfinite(float(value))
    except (OverflowError, ValueError):
        return False


def _sample_nonce(run_id: str, preflight_digest: str, group_key: str, repetition: int) -> str:
    material = "\\0".join((run_id, preflight_digest, group_key, str(repetition)))
    return hashlib.sha256(("victory-sample/v1\\0" + material).encode("utf-8")).hexdigest()


def _freshness_issue(record: Mapping[str, Any], *, expected_preflight: str | None = None, expected_run_id: str | None = None) -> str | None:
    metrics = record.get("metrics")
    freshness = metrics.get("freshness") if isinstance(metrics, Mapping) else None
    if not isinstance(freshness, Mapping) or set(freshness) != _FRESHNESS_KEYS:
        return "freshness_missing"
    if freshness.get("schema") != _FRESHNESS_SCHEMA:
        return "freshness_schema"
    run_id = freshness.get("run_id")
    preflight = freshness.get("preflight_digest")
    group_id = freshness.get("group_id")
    repetition = freshness.get("repetition")
    nonce = freshness.get("nonce")
    if not isinstance(run_id, str) or not _DURABLE_DIGEST_RE.fullmatch(run_id):
        return "freshness_run_id"
    if not isinstance(preflight, str) or not _SHA256_RE.fullmatch(preflight):
        return "freshness_preflight"
    if expected_run_id is not None and run_id != expected_run_id:
        return "freshness_run_mismatch"
    if expected_preflight is not None and preflight != expected_preflight:
        return "freshness_preflight_mismatch"
    expected_group = f"{record.get('adapter_id')}:{record.get('case_id')}:{record.get('operation')}"
    expected_group_id = "g" + hashlib.sha256(expected_group.encode("utf-8")).hexdigest()[:63]
    if not isinstance(group_id, str) or not _DURABLE_DIGEST_RE.fullmatch(group_id) or group_id != expected_group_id:
        return "freshness_group_mismatch"
    if not isinstance(repetition, int) or isinstance(repetition, bool) or repetition != record.get("repetition"):
        return "freshness_repetition_mismatch"
    if not isinstance(nonce, str) or not _SHA256_RE.fullmatch(nonce):
        return "freshness_nonce"
    if nonce != _sample_nonce(run_id, preflight, expected_group, repetition):
        return "freshness_nonce_mismatch"
    return None


def percentile(values: Iterable[float], p: float) -> float | None:
    values = sorted(float(value) for value in values)
    if not values:
        return None
    rank = (len(values) - 1) * p / 100
    low, high = int(rank), min(int(rank) + 1, len(values) - 1)
    return values[low] + (values[high] - values[low]) * (rank - low)


def _stats(values: Iterable[float]) -> dict[str, Any]:
    values = [float(value) for value in values]
    return {"n": len(values), "p50": percentile(values, 50), "p95": percentile(values, 95), "max": max(values) if values else None, "mean": statistics.fmean(values) if values else None}


def latency_stats(records: list[dict[str, Any]]) -> dict[str, Any]:
    values = [float(record["elapsed_ms"]) for record in records if _is_finite_number(record.get("elapsed_ms"))]
    stats = _stats(values)
    return {"n": stats["n"], "p50_ms": stats["p50"], "p95_ms": stats["p95"], "max_ms": stats["max"], "mean_ms": stats["mean"]}


def _metric_stats(records: list[dict[str, Any]], key: str) -> dict[str, Any]:
    values: list[float] = []
    for record in records:
        metrics = record.get("metrics")
        child = metrics.get("child") if isinstance(metrics, Mapping) else None
        value = child.get(key) if isinstance(child, Mapping) else None
        if _is_finite_number(value):
            values.append(float(value))
    stats = _stats(values)
    return {"n": stats["n"], "p50": stats["p50"], "p95": stats["p95"], "max": stats["max"]}


def _contains_forbidden_key(value: Any) -> bool:
    if isinstance(value, dict):
        return any(str(key).lower() in _FORBIDDEN_KEYS or _contains_forbidden_key(child) for key, child in value.items())
    if isinstance(value, list):
        return any(_contains_forbidden_key(child) for child in value)
    return False


def _validate_sample_metrics(record: dict[str, Any]) -> str | None:
    """Return a conservative reason; malformed evidence never becomes PASS."""
    metrics = record.get("metrics")
    if not isinstance(metrics, Mapping):
        return "metrics_malformed"
    child = metrics.get("child")
    if not isinstance(child, Mapping) or child.get("status") not in {"PASS", "NOT_COMPARABLE"}:
        return "child_metrics_missing"
    if child["status"] == "PASS":
        value = child.get("peak_rss_bytes")
        tree_value = child.get("tree_peak_rss_bytes")
        if not _is_finite_number(value) or value < 0:
            return "working_set_unavailable"
        if not _is_finite_number(tree_value) or tree_value < 0:
            return "tree_rss_missing"
        # PASS is reserved for a measured, successful child tree.  In
        # particular, not_required cleanup and a single-process observation are
        # not equivalent to native process-tree proof.
        if child.get("tree_supported") is not True:
            return "tree_not_observed"
        if not isinstance(child.get("samples"), int) or isinstance(child.get("samples"), bool) or child.get("samples", 0) <= 0:
            return "child_samples_missing"
        if child.get("timed_out") is not False:
            return "child_timed_out"
        if child.get("crashed") is not False:
            return "child_crashed"
        if child.get("failure_class") != "none":
            return "child_failure_class"
        if not isinstance(child.get("exit_code"), int) or isinstance(child.get("exit_code"), bool) or child.get("exit_code") != 0:
            return "child_exit_code"
        if child.get("cleanup_status") not in {"not_required", "clean", "forced"}:
            return "cleanup_missing"
        if (
            child.get("timed_out") is not False
            or child.get("crashed") is not False
            or child.get("failure_class") != "none"
            or not isinstance(child.get("exit_code"), int)
            or isinstance(child.get("exit_code"), bool)
            or child.get("exit_code") != 0
        ):
            return "child_terminal_unsuccessful"
    if record.get("status") == "PASS" and not _is_finite_number(record.get("elapsed_ms")):
        return "latency_missing"
    security = metrics.get("security")
    if not isinstance(security, Mapping):
        return "security_missing"
    runtime = security.get("runtime")
    integrity = security.get("integrity")
    source_integrity = security.get("source_integrity")
    required = (
        security.get("status") == "PASS",
        security.get("runtime_proof") is True,
        isinstance(runtime, Mapping) and runtime.get("status") == "PASS",
        isinstance(runtime, Mapping) and runtime.get("runtime_proof") is True,
        isinstance(runtime, Mapping) and runtime.get("provenance") == "child_metrics_executor",
        isinstance(runtime, Mapping) and isinstance(runtime.get("probe_mode"), str) and bool(runtime.get("probe_mode")),
        isinstance(runtime, Mapping) and runtime.get("network_count") == 0,
        isinstance(runtime, Mapping) and runtime.get("mcp_count") == 0,
        isinstance(runtime, Mapping) and runtime.get("reason") is None,
        isinstance(runtime, Mapping) and isinstance(runtime.get("sample_count"), int) and not isinstance(runtime.get("sample_count"), bool) and runtime.get("sample_count", 0) > 0,
        isinstance(runtime, Mapping) and _valid_pid_list(runtime.get("observed_pids"), nonempty=True),
        isinstance(runtime, Mapping) and _valid_pid_list(runtime.get("metadata_observed_pids"), nonempty=False),
        isinstance(runtime, Mapping) and _pid_coverage(runtime),
        isinstance(runtime, Mapping) and isinstance(runtime.get("evidence_digest"), str) and _SHA256_RE.fullmatch(runtime["evidence_digest"]),
        isinstance(runtime, Mapping) and _runtime_digest_matches(runtime),
        isinstance(integrity, Mapping) and integrity.get("status") == "PASS",
        isinstance(source_integrity, Mapping) and source_integrity.get("status") == "PASS",
    )
    if not all(required):
        if security.get("status") in {"FAIL", "BLOCKED"} or (isinstance(runtime, Mapping) and runtime.get("status") in {"FAIL", "BLOCKED"}):
            return "security_failed"
        return "security_incomplete"
    return None


def child_metric_stats(records: list[dict[str, Any]]) -> dict[str, Any]:
    statuses: dict[str, int] = {}
    reasons: dict[str, int] = {}
    for record in records:
        child = record.get("metrics", {}).get("child", {})
        if not isinstance(child, dict):
            continue
        status = str(child.get("status", "NOT_COMPARABLE"))
        statuses[status] = statuses.get(status, 0) + 1
        reason = child.get("reason_code")
        if reason:
            reasons[str(reason)] = reasons.get(str(reason), 0) + 1
    return {"status_counts": dict(sorted(statuses.items())), "reason_counts": dict(sorted(reasons.items())), "peak_rss_bytes": _metric_stats(records, "peak_rss_bytes"), "tree_peak_rss_bytes": _metric_stats(records, "tree_peak_rss_bytes")}


def _validate_pass_canonical(record: Mapping[str, Any]) -> None:
    if record.get("status") != "PASS":
        return
    canonical = record.get("canonical")
    if not isinstance(canonical, Mapping) or not isinstance(canonical.get("payload"), Mapping):
        raise ValueError("PASS sample canonical payload is malformed")
    payload = canonical["payload"]
    validate_terminal_state(payload)
    if canonical.get("digest") != payload_digest(payload):
        raise ValueError("canonical digest does not match payload")
    token_units = canonical.get("token_units")
    expected = token_count(payload)
    if (
        isinstance(token_units, bool)
        or not _is_finite_number(token_units)
        or token_units <= 0
        or token_units != expected
    ):
        raise ValueError("canonical token_units must equal positive token_count(payload)")


def _read_samples(path: Path) -> list[dict[str, Any]]:
    if path.is_file() and path.suffix == ".jsonl":
        lines = path.read_text(encoding="utf-8").splitlines()
    elif path.is_dir() and (path / "samples.jsonl").is_file():
        lines = (path / "samples.jsonl").read_text(encoding="utf-8").splitlines()
    else:
        raise ValueError("input must be samples.jsonl or a runner output directory")
    records = [json.loads(line) for line in lines if line.strip()]
    if not records:
        raise ValueError("no samples")
    for record in records:
        if not isinstance(record, dict):
            raise ValueError("sample must be an object")
        if _contains_forbidden_key(record):
            raise ValueError("best-of summaries are forbidden")
        try:
            RunRecord.from_dict(record)
            validate_identifier(record.get("adapter_id"), "adapter_id")
            validate_identifier(record.get("case_id"), "case_id")
            validate_group_key(f"{record['adapter_id']}:{record['case_id']}:{record['operation']}")
            _validate_pass_canonical(record)
        except (AttributeError, TypeError) as exc:
            raise ValueError("malformed sample evidence") from exc
        freshness_issue = _freshness_issue(record)
        if freshness_issue:
            raise ValueError(f"sample freshness is invalid: {freshness_issue}")
        metric_issue = _validate_sample_metrics(record)
        if metric_issue and record.get("status") == "PASS":
            raise ValueError(f"PASS sample metrics are incomplete: {metric_issue}")
    return records


def _group(records: list[dict[str, Any]], *keys: str) -> dict[tuple[Any, ...], list[dict[str, Any]]]:
    groups: dict[tuple[Any, ...], list[dict[str, Any]]] = {}
    for record in records:
        groups.setdefault(tuple(record.get(key) for key in keys), []).append(record)
    return groups


def _digest_group(manifest: Mapping[str, Any], key: str) -> str:
    return sha256_bytes(json.dumps(manifest.get(key, {}), sort_keys=True, separators=(",", ":")).encode())


def _items(payload: Any) -> list[str]:
    if not isinstance(payload, Mapping):
        return []
    values = payload.get("items", payload.get("nodes", payload.get("results", [])))
    if not isinstance(values, list):
        return []
    result: list[str] = []
    for value in values:
        if isinstance(value, str):
            result.append(value)
        elif isinstance(value, Mapping):
            for key in ("display", "name", "symbol", "qualified_name", "owner_path"):
                if isinstance(value.get(key), str) and value[key]:
                    result.append(value[key])
                    break
    return result


def _expected(case: Mapping[str, Any], manifest: Mapping[str, Any]) -> list[str]:
    oracle = manifest.get("oracles", {}).get(case["id"], {})
    if case.get("operation") == "path":
        return [str(item) for item in oracle.get("expected_shortest_path", [])]
    key = "expected_direct" if case.get("mode", "direct") == "direct" else "expected_transitive"
    return [str(item) for item in oracle.get(key, [])]


def _negative(case: Mapping[str, Any], manifest: Mapping[str, Any]) -> set[str]:
    values = manifest.get("oracles", {}).get(case["id"], {}).get("negative_exclusions", [])
    return {str(item) for item in values} if isinstance(values, list) else set()


def _quality(records: list[dict[str, Any]], case: Mapping[str, Any], manifest: Mapping[str, Any]) -> dict[str, Any]:
    expected = _expected(case, manifest)
    expected_set, negative = set(expected), _negative(case, manifest)
    actuals = [_items(record.get("canonical", {}).get("payload", {})) for record in records if record.get("status") == "PASS"]
    actual = actuals[0] if actuals else []
    actual_set = set(actual)
    tp, fp, fn = len(actual_set & expected_set), len(actual_set - expected_set), len(expected_set - actual_set)
    negative_violations = len(actual_set & negative)
    precision = tp / (tp + fp) if tp + fp else 0.0
    recall = tp / (tp + fn) if tp + fn else 0.0
    return {
        "correctness": {"numerator": int(actual == expected), "denominator": 1, "value": 1.0 if actual == expected else 0.0},
        "precision": {"tp": tp, "fp": fp, "denominator": tp + fp, "value": precision},
        "recall": {"tp": tp, "fn": fn, "denominator": tp + fn, "value": recall},
        "f1": {"numerator": 2 * tp, "denominator": 2 * tp + fp + fn, "value": 2 * precision * recall / (precision + recall) if precision + recall else 0.0},
        "negative_violations": {"count": negative_violations, "denominator": len(negative), "value": negative_violations},
    }


def _effective_status(group: list[dict[str, Any]]) -> str:
    statuses = {str(record.get("status")) for record in group}
    child_statuses: set[str] = set()
    for record in group:
        metrics = record.get("metrics")
        child = metrics.get("child") if isinstance(metrics, Mapping) else None
        if isinstance(child, Mapping):
            child_statuses.add(str(child.get("status")))
    if "BLOCKED" in statuses:
        return "BLOCKED"
    if "FAIL" in statuses:
        return "FAIL"
    if "NOT_COMPARABLE" in statuses or "NOT_COMPARABLE" in child_statuses:
        return "NOT_COMPARABLE"
    return "PASS" if statuses == {"PASS"} else "NOT_RUN"


def _runtime_preflight_digest(manifest_path: Path, manifest: Mapping[str, Any]) -> str:
    evidence = {
        "manifest_sha256": sha256_file(manifest_path),
        "adapters": [
            {
                "adapter_id": adapter.get("adapter_id"),
                "attestation_sha256": adapter.get("expected_attestation_sha256"),
                "executable_sha256": adapter.get("expected_executable_sha256"),
                "interpreter_sha256": adapter.get("interpreter_sha256"),
                "source_commit": adapter.get("expected_commit"),
                "source_sha256": adapter.get("expected_source_sha256"),
            }
            for adapter in manifest.get("adapters", [])
        ],
    }
    return sha256_bytes(json.dumps(evidence, sort_keys=True, separators=(",", ":")).encode())


def _overall_sample_status(records: list[dict[str, Any]]) -> str:
    statuses = {str(record.get("status")) for record in records}
    child_statuses = {
        str(record.get("metrics", {}).get("child", {}).get("status"))
        for record in records
        if isinstance(record.get("metrics"), Mapping) and isinstance(record["metrics"].get("child"), Mapping)
    }
    if "BLOCKED" in statuses:
        return "BLOCKED"
    if "FAIL" in statuses:
        return "FAIL"
    if "NOT_COMPARABLE" in statuses or "NOT_COMPARABLE" in child_statuses:
        return "NOT_COMPARABLE"
    if statuses == {"PASS"}:
        return "PASS"
    return "NOT_RUN"


def _runner_metadata(samples: Path, records: list[dict[str, Any]]) -> dict[str, Any] | None:
    if not samples.is_dir():
        return None
    metadata_path = samples / "run.json"
    samples_path = samples / "samples.jsonl"
    if not metadata_path.is_file():
        raise ValueError("runner output is missing run.json")
    if not samples_path.is_file():
        raise ValueError("runner output is missing samples.jsonl")
    value = json.loads(metadata_path.read_text(encoding="utf-8"))
    if not isinstance(value, dict):
        raise ValueError("run metadata must be an object")
    runtime = value.get("runtime_preflight")
    if not isinstance(runtime, Mapping) or runtime.get("status") != "PASS" or runtime.get("require_runtime") is not True or runtime.get("fresh_reproduction") is not True:
        raise ValueError("runner runtime preflight metadata is incomplete")
    if not isinstance(value.get("run_id"), str) or not _DURABLE_DIGEST_RE.fullmatch(value["run_id"]):
        raise ValueError("runner run_id is invalid")
    if not isinstance(runtime.get("evidence_digest"), str) or not _SHA256_RE.fullmatch(runtime["evidence_digest"]):
        raise ValueError("runner preflight digest is invalid")
    if not isinstance(value.get("samples_sha256"), str) or not _SHA256_RE.fullmatch(value["samples_sha256"]):
        raise ValueError("runner samples digest is invalid")
    if value["samples_sha256"] != sha256_file(samples_path):
        raise ValueError("runner samples SHA-256 does not match samples.jsonl")
    if value.get("samples") != len(records):
        raise ValueError("runner sample count does not match samples.jsonl")
    expected_counts = {item: sum(record.get("status") == item for record in records) for item in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    if value.get("status_counts") != expected_counts:
        raise ValueError("runner status counts do not match samples.jsonl")
    if value.get("status") != _overall_sample_status(records):
        raise ValueError("runner status does not match samples.jsonl")
    if value.get("repetitions") != AUTHORITATIVE_REPETITIONS:
        raise ValueError("runner repetition metadata is invalid")
    return value


def _validate_manifest_bundle(runner_metadata: Mapping[str, Any], manifest_path: Path) -> dict[str, str]:
    expected_path = str(manifest_path.resolve())
    expected_sha256 = sha256_file(manifest_path)
    expected_id = _manifest_identity(manifest_path)
    if runner_metadata.get("manifest_path") != expected_path or runner_metadata.get("manifest") not in (None, expected_path):
        raise ValueError("runner manifest path identity does not match supplied manifest")
    if runner_metadata.get("manifest_sha256") != expected_sha256:
        raise ValueError("runner manifest SHA-256 does not match supplied manifest")
    if runner_metadata.get("manifest_id") != expected_id:
        raise ValueError("runner manifest id does not match supplied manifest")
    bundle = runner_metadata.get("manifest_bundle")
    if not isinstance(bundle, Mapping) or dict(bundle) != {"path": expected_path, "id": expected_id, "sha256": expected_sha256}:
        raise ValueError("runner manifest bundle does not match supplied manifest")
    return {"path": expected_path, "id": expected_id, "sha256": expected_sha256}


def _load_manifest(manifest: str | Path | Mapping[str, Any] | None) -> tuple[dict[str, Any] | None, Path | None]:
    if manifest is None:
        return None, None
    if isinstance(manifest, Mapping):
        return dict(manifest), None
    path = Path(manifest).resolve()
    value = load_manifest(path)
    validate_strict_manifest(value, path.parent, check_files=True)
    return value, path


def build_report(samples: str | Path, *, manifest: str | Path | Mapping[str, Any] | None = None, expected_repetitions: int = AUTHORITATIVE_REPETITIONS) -> dict[str, Any]:
    if expected_repetitions != AUTHORITATIVE_REPETITIONS:
        raise ValueError("Victory Lab reports require exactly 30 repetitions")
    samples_path = Path(samples)
    loaded_manifest, manifest_path = _load_manifest(manifest)
    if loaded_manifest is not None and not samples_path.is_dir():
        raise ValueError("manifest-backed evidence requires a runner output directory")
    records = _read_samples(samples_path)
    if any(not record.get("case_id") for record in records):
        raise ValueError("sample case_id is required")
    runner_metadata = _runner_metadata(samples_path, records)
    manifest_bundle = None
    expected_preflight = None
    expected_run_id = runner_metadata.get("run_id") if runner_metadata else None
    if runner_metadata:
        expected_preflight = runner_metadata["runtime_preflight"]["evidence_digest"]
    if manifest_path is not None:
        if runner_metadata is None:
            raise ValueError("manifest-backed evidence requires runner metadata")
        manifest_bundle = _validate_manifest_bundle(runner_metadata, manifest_path)
        manifest_preflight = _runtime_preflight_digest(manifest_path, loaded_manifest)
        if runner_metadata["runtime_preflight"]["evidence_digest"] != manifest_preflight:
            raise ValueError("runner preflight digest does not match manifest")
        expected_preflight = manifest_preflight
    expected_keys = None
    case_map: dict[tuple[str, str], Mapping[str, Any]] = {}
    if loaded_manifest is not None:
        fixture_digest, oracle_digest = _digest_group(loaded_manifest, "fixture_hashes"), _digest_group(loaded_manifest, "oracle_hashes")
        for record in records:
            if record.get("fixture_digest") != fixture_digest or record.get("oracle_digest") != oracle_digest:
                raise ValueError("sample fixture/oracle digest does not match manifest")
        expected_keys = {(adapter["adapter_id"], case["id"], case["operation"]) for adapter in loaded_manifest["adapters"] for case in loaded_manifest["cases"]}
        case_map = {(case["id"], case["operation"]): case for case in loaded_manifest["cases"]}
    groups = _group(records, "adapter_id", "case_id", "operation")
    if expected_keys is not None and set(groups) != expected_keys:
        raise ValueError(f"sample universe mismatch; missing={sorted(expected_keys - set(groups))}, extra={sorted(set(groups) - expected_keys)}")
    group_reports: dict[str, Any] = {}
    seen_samples: set[tuple[Any, ...]] = set()
    freshness_run_ids: set[str] = set()
    freshness_preflights: set[str] = set()
    for (adapter_id, case_id, operation), group in sorted(groups.items()):
        if len(group) != 30:
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: expected exactly 30 samples, got {len(group)}")
        sample_keys = [(adapter_id, case_id, operation, record.get("repetition")) for record in group]
        if len(set(sample_keys)) != 30 or seen_samples.intersection(sample_keys):
            raise ValueError("duplicate or replayed sample")
        seen_samples.update(sample_keys)
        if sorted(record.get("repetition") for record in group) != list(range(30)):
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: repetitions must be exactly 0..29")
        freshness_issues = [
            issue for issue in (
                _freshness_issue(record, expected_preflight=expected_preflight, expected_run_id=expected_run_id)
                for record in group
            ) if issue
        ]
        if freshness_issues:
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: invalid freshness evidence: {freshness_issues[0]}")
        group_freshness = [record["metrics"]["freshness"] for record in group]
        group_run_ids = {item["run_id"] for item in group_freshness}
        group_preflights = {item["preflight_digest"] for item in group_freshness}
        if len(group_run_ids) != 1 or len(group_preflights) != 1:
            raise ValueError(f"{adapter_id}/{case_id}/{operation}: freshness identity changed within group")
        freshness_run_ids.update(group_run_ids)
        freshness_preflights.update(group_preflights)
        # Identical outputs across distinct repetitions are the expected signal
        # for determinism, especially for static NOT_COMPARABLE capability
        # slices. Replay protection is provided by the exact 0..29 repetition
        # universe and the globally unique sample keys above.
        digests = []
        metric_issues = [issue for issue in (_validate_sample_metrics(record) for record in group) if issue]
        for record in group:
            if record.get("status") == "PASS":
                canonical = record.get("canonical")
                if not isinstance(canonical, Mapping) or canonical.get("digest") != payload_digest(canonical.get("payload")):
                    raise ValueError("canonical digest does not match payload")
                digests.append(canonical["digest"])
        determinism = {"reruns": 30, "unique_digests": len(set(digests)), "outputs_identical": bool(digests) and len(set(digests)) == 1, "status": "PASS" if digests and len(set(digests)) == 1 else "FAIL"}
        case = case_map.get((case_id, operation), {"id": case_id, "operation": operation})
        quality = _quality(group, case, loaded_manifest) if loaded_manifest else {"status": "NOT_COMPARABLE"}
        tokens = [record["canonical"].get("token_units") for record in group if record.get("status") == "PASS" and isinstance(record.get("canonical"), Mapping) and isinstance(record["canonical"].get("token_units"), (int, float))]
        stale = []
        incremental = []
        for record in group:
            metrics = record.get("metrics")
            if not isinstance(metrics, Mapping):
                continue
            stale_value = metrics.get("stale_rate")
            incremental_value = metrics.get("incrementality_ms")
            if _is_finite_number(stale_value):
                stale.append(stale_value)
            if _is_finite_number(incremental_value):
                incremental.append(incremental_value)
        status = _effective_status(group)
        if metric_issues and "security_failed" in metric_issues:
            status = "BLOCKED"
        elif metric_issues and status == "PASS":
            status = "NOT_COMPARABLE"
        if loaded_manifest is not None and status == "PASS" and (not incremental or not stale):
            status = "NOT_COMPARABLE"
        if status == "PASS" and determinism["status"] != "PASS":
            status = "FAIL"
        latency = latency_stats(group)
        provenances = set()
        for record in group:
            metrics = record.get("metrics")
            security = metrics.get("security") if isinstance(metrics, Mapping) else None
            runtime = security.get("runtime") if isinstance(security, Mapping) else None
            if isinstance(runtime, Mapping):
                provenances.add(runtime.get("provenance"))
        group_reports[f"{adapter_id}:{case_id}:{operation}"] = {
            "adapter_id": adapter_id, "case_id": case_id, "operation": operation, "status": status,
            "status_counts": {item: sum(record["status"] == item for record in group) for item in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")},
            "latency": latency, "warm_p95_ms": latency["p95_ms"], "tokens": _stats(tokens), "child_metrics": child_metric_stats(group),
            "runtime_proof": {"status": "PASS" if provenances == {"child_metrics_executor"} else "NOT_COMPARABLE", "provenance": sorted(str(item) for item in provenances), "samples": len(group)},
            "incrementality": {"status": "PASS" if incremental else "NOT_COMPARABLE", "samples": _stats(incremental), "stale_rate": _stats(stale) if stale else {"status": "NOT_COMPARABLE"}},
            "quality": quality, "determinism": determinism, "all_samples": 30, "canonical_digests": sorted(set(digests)),
        }
    if len(freshness_run_ids) != 1 or len(freshness_preflights) != 1:
        raise ValueError("sample freshness identity changed across run")
    counts = {item: sum(record["status"] == item for record in records) for item in ("PASS", "FAIL", "BLOCKED", "NOT_COMPARABLE", "NOT_RUN")}
    statuses = [item["status"] for item in group_reports.values()]
    status = "BLOCKED" if "BLOCKED" in statuses else "FAIL" if "FAIL" in statuses else "NOT_COMPARABLE" if "NOT_COMPARABLE" in statuses or (loaded_manifest is not None and (loaded_manifest.get("thresholds", {}).get("status") != "PASS")) else "PASS" if statuses and all(item == "PASS" for item in statuses) else "NOT_RUN"
    if loaded_manifest is None and status == "PASS":
        status = "NOT_COMPARABLE"
    return {
        "schema": "victory-report/v2", "status": status, "samples": len(records), "status_counts": counts,
        "manifest": str(manifest_path) if manifest_path else None,
        "manifest_path": manifest_bundle["path"] if manifest_bundle else None,
        "manifest_id": manifest_bundle["id"] if manifest_bundle else None,
        "manifest_sha256": manifest_bundle["sha256"] if manifest_bundle else None,
        "manifest_bundle": manifest_bundle,
        "expected_repetitions": 30, "expected_universe": len(expected_keys) if expected_keys is not None else None,
        "groups": group_reports,
        "thresholds": (loaded_manifest or {}).get("thresholds", {"status": "NOT_COMPARABLE", "reason_code": "thresholds_not_declared"}),
        "anti_gaming": {"all_samples_used": True, "best_of": False, "exactly_30": all(len(group) == 30 for group in groups.values()), "manifest_consumed": loaded_manifest is not None},
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--repetitions", type=int, default=30)
    args = parser.parse_args(argv)
    try:
        report = build_report(args.input, manifest=args.manifest, expected_repetitions=args.repetitions)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"BLOCKED: {exc}")
        return 2
    Path(args.output).write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
